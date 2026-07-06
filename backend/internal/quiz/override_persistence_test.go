package quiz

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestService_OverrideAnswer_PersistsCorrectionToYAML reproduces the
// reported bug: after a "misunderstood" answer on a standard vocabulary
// quiz, clicking "Mark as Correct" must rewrite the YAML log so the
// status flips to "understood" and quality goes from 1 to 4. The
// service was silently no-op'ing for cases the on-disk reload didn't
// surface, leaving the failure recorded.
//
// Real disk round-trip: seed YAML, call OverrideAnswer, re-read YAML,
// assert the persisted shape — same pattern as the SkipWord tests.
func TestService_OverrideAnswer_PersistsCorrectionToYAML_StandardScene(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()

	// Seed: one notebook-type log marked misunderstood inside a scene.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-story.yml"), []byte(`- metadata:
    notebook_id: test-story
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "preposterous"
          learned_logs:
            - status: misunderstood
              learned_at: "2026-06-29T10:00:00Z"
              quality: 1
              quiz_type: notebook
              interval_days: 1
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{t.TempDir()},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	markCorrect := true
	info := CardInfo{
		NotebookName: "test-story",
		StoryTitle:   "Chapter One",
		SceneTitle:   "Opening",
		Expression:   "preposterous",
		LearnedAt:    "2026-06-29",
		MarkCorrect:  &markCorrect,
	}

	_, err := svc.OverrideAnswer(info, notebook.QuizTypeNotebook)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, "test-story.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	require.Len(t, got[0].Scenes, 1)
	require.Len(t, got[0].Scenes[0].Expressions, 1)
	logs := got[0].Scenes[0].Expressions[0].LearnedLogs
	require.Len(t, logs, 1, "override must not duplicate the log entry")
	assert.Equal(t, notebook.LearnedStatus("understood"), logs[0].Status,
		"Mark-as-Correct must flip misunderstood -> understood on disk")
	assert.Equal(t, 4, logs[0].Quality, "override must bump quality from 1 to 4")
}

// Definitions-style note where Note.Definition is set, so the card's
// Entry is the definition string but the YAML stores the original
// Expression as the lookup key. The bug: toggleLastAnswer matches
// against info.Expression == card.Entry (the definition), never
// realising the YAML entry's expression is the original word, and
// silently no-ops. The persisted log stays misunderstood even though
// the user clicked Mark-as-Correct.
func TestService_OverrideAnswer_PersistsCorrectionToYAML_DefinitionEntryFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()

	// YAML stores under expression "serenity", but the quiz card's Entry
	// would be the longer definition (Note.Definition != Note.Expression).
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "wpme.yml"), []byte(`- metadata:
    notebook_id: wpme
    title: "Session 3"
  scenes:
    - metadata:
        title: "serenitas (calm)"
      expressions:
        - expression: "serenity"
          learned_logs:
            - status: misunderstood
              learned_at: "2026-06-29T10:00:00Z"
              quality: 1
              quiz_type: notebook
              interval_days: 1
`), 0644))

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{t.TempDir()},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	// Mimics CardInfoFromCard for a card built from a definition-bearing
	// note: Expression carries the displayed definition (card.Entry).
	// The matching YAML key is the original word (card.OriginalEntry),
	// which the service must check as a fallback.
	markCorrect := true
	info := CardInfo{
		NotebookName:       "wpme",
		StoryTitle:         "Session 3",
		SceneTitle:         "serenitas (calm)",
		Expression:         "state of being calm and untroubled",
		OriginalExpression: "serenity",
		LearnedAt:          "2026-06-29",
		MarkCorrect:        &markCorrect,
	}

	_, err := svc.OverrideAnswer(info, notebook.QuizTypeNotebook)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, "wpme.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	require.Len(t, got[0].Scenes, 1)
	require.Len(t, got[0].Scenes[0].Expressions, 1)
	logs := got[0].Scenes[0].Expressions[0].LearnedLogs
	require.Len(t, logs, 1)
	assert.Equal(t, notebook.LearnedStatus("understood"), logs[0].Status,
		"override must find the YAML entry by its original Expression key when the card's Entry is the Definition form")
}

// The frontend sends LearnedAt of the specific log the user clicked
// Mark-as-Correct on. The backend must flip THAT log — not blindly
// touch logs[0]. Test: mark a historical (older) misunderstood log
// correct while a more recent understood log exists. The newer log
// must not be touched.
func TestService_OverrideAnswer_TargetsLearnedAt_NotJustLatest(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()

	// Newest log first (the YAML convention). The historical log at
	// 2026-06-27 is the one the user is "marking correct" via Analytics.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-story.yml"), []byte(`- metadata:
    notebook_id: test-story
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "preposterous"
          learned_logs:
            - status: understood
              learned_at: "2026-06-29T10:00:00Z"
              quality: 4
              quiz_type: notebook
              interval_days: 3
            - status: misunderstood
              learned_at: "2026-06-27T10:00:00Z"
              quality: 1
              quiz_type: notebook
              interval_days: 1
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{t.TempDir()},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	info := CardInfo{
		NotebookName: "test-story",
		StoryTitle:   "Chapter One",
		SceneTitle:   "Opening",
		Expression:   "preposterous",
		LearnedAt:    "2026-06-27", // The historical log the user clicked
	}
	markCorrect := true
	info.MarkCorrect = &markCorrect

	_, err := svc.OverrideAnswer(info, notebook.QuizTypeNotebook)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, "test-story.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	logs := got[0].Scenes[0].Expressions[0].LearnedLogs
	require.Len(t, logs, 2)
	assert.Equal(t, notebook.LearnedStatus("understood"), logs[0].Status,
		"newest log was already understood — override of historical log must leave it untouched")
	assert.Equal(t, notebook.LearnedStatus("understood"), logs[1].Status,
		"the historical log at LearnedAt=2026-06-27 should have flipped to understood")
}

// Freeform quizzes write each answer into both LearnedLogs AND
// ReverseLogs (see YAMLLearningRepository.Create freeform branch). An
// override that only touches one half leaves the two halves of the
// same logical answer disagreeing. Both must flip together.
func TestService_OverrideAnswer_Freeform_KeepsLearnedAndReverseLogsInSync(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-story.yml"), []byte(`- metadata:
    notebook_id: test-story
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "preposterous"
          learned_logs:
            - status: misunderstood
              learned_at: "2026-06-29T10:00:00Z"
              quality: 1
              quiz_type: freeform
              interval_days: 1
          reverse_logs:
            - status: misunderstood
              learned_at: "2026-06-29T10:00:00Z"
              quality: 1
              quiz_type: freeform
              interval_days: 1
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{t.TempDir()},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	markCorrect := true
	info := CardInfo{
		NotebookName: "test-story",
		StoryTitle:   "Chapter One",
		SceneTitle:   "Opening",
		Expression:   "preposterous",
		LearnedAt:    "2026-06-29",
		MarkCorrect:  &markCorrect,
	}

	_, err := svc.OverrideAnswer(info, notebook.QuizTypeFreeform)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, "test-story.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	expr := got[0].Scenes[0].Expressions[0]
	require.Len(t, expr.LearnedLogs, 1)
	require.Len(t, expr.ReverseLogs, 1)
	assert.Equal(t, expr.LearnedLogs[0].Status, expr.ReverseLogs[0].Status,
		"freeform override must keep learned_logs and reverse_logs in sync")
	assert.NotEqual(t, notebook.LearnedStatus("misunderstood"), expr.LearnedLogs[0].Status,
		"override should have flipped misunderstood to a correct status")
}

// UndoOverrideAnswer restores the log to the caller-supplied
// pre-override snapshot on disk — this is the path the frontend's
// Analytics "Undo" button takes after a Mark-as-Correct. Without
// this, YAML.UpdateLog silently no-op'd when MirrorValues was set
// (Undo passes MirrorValues, not MarkCorrect), leaving the log stuck
// in the overridden state.
func TestService_UndoOverrideAnswer_RestoresOriginalStateOnDisk(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()

	// Seed with the OVERRIDDEN state: user previously marked this correct.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-story.yml"), []byte(`- metadata:
    notebook_id: test-story
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "preposterous"
          learned_logs:
            - status: understood
              learned_at: "2026-06-29T10:00:00Z"
              quality: 4
              quiz_type: notebook
              interval_days: 3
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{t.TempDir()},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	info := CardInfo{
		NotebookName:         "test-story",
		StoryTitle:           "Chapter One",
		SceneTitle:           "Opening",
		Expression:           "preposterous",
		LearnedAt:            "2026-06-29",
		OriginalQuality:      1,
		OriginalStatus:       "misunderstood",
		OriginalIntervalDays: 1,
	}

	correct, _, err := svc.UndoOverrideAnswer(info, notebook.QuizTypeNotebook)
	require.NoError(t, err)
	assert.False(t, correct, "restored log has quality 1 < 3, so undo reports correct=false")

	raw, err := os.ReadFile(filepath.Join(learningDir, "test-story.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	logs := got[0].Scenes[0].Expressions[0].LearnedLogs
	require.Len(t, logs, 1)
	assert.Equal(t, notebook.LearnedStatus("misunderstood"), logs[0].Status,
		"Undo must restore status to the pre-override snapshot on disk")
	assert.Equal(t, 1, logs[0].Quality)
	assert.Equal(t, 1, logs[0].IntervalDays)
}

// Flashcard variant: top-level Expressions (no Scenes). This is the
// shape the user's vocabulary-book notebooks use.
func TestService_OverrideAnswer_PersistsCorrectionToYAML_StandardFlashcard(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(`- metadata:
    notebook_id: test-vocab
    title: "Basic Words"
    type: flashcard
  expressions:
    - expression: "serendipity"
      learned_logs:
        - status: misunderstood
          learned_at: "2026-06-29T10:00:00Z"
          quality: 1
          quiz_type: notebook
          interval_days: 1
`), 0644))

	svc := NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{t.TempDir()},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	markCorrect := true
	info := CardInfo{
		NotebookName: "test-vocab",
		StoryTitle:   "Basic Words",
		Expression:   "serendipity",
		LearnedAt:    "2026-06-29",
		MarkCorrect:  &markCorrect,
	}

	_, err := svc.OverrideAnswer(info, notebook.QuizTypeNotebook)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, "test-vocab.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	require.Len(t, got[0].Expressions, 1)
	logs := got[0].Expressions[0].LearnedLogs
	require.Len(t, logs, 1)
	assert.Equal(t, notebook.LearnedStatus("understood"), logs[0].Status)
	assert.Equal(t, 4, logs[0].Quality)
}
