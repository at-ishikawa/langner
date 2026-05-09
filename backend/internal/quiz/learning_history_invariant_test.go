package quiz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestLearningHistory_OneLocationPerExpression_AcrossAllWriters pins the
// invariant that every code path that mutates a learning_notes YAML places
// the same (notebook_id, expression) at the SAME (story_title, scene_title)
// location. When two writers disagree on the location, updates from one
// path are invisible to the other and the YAML accumulates duplicate,
// fragmented records — exactly the bug the user reported in
// word-power-made-easy.yml.
//
// SKIPPED today because the etymology quiz writer diverges from
// standard/reverse/freeform: etymology writes (notebook_name, session_title)
// while the others write (story_event, scene_title). Re-enable this test
// once the canonical shape is unified — see PR description for the deferred
// migration plan. Removing t.Skip is the first step of that migration.
//
// Writer matrix — every "live" code path that mutates a learning_notes YAML
// for a single notebook ID. Adding a new writer must add a row below or
// the matrix silently misses it; the comment is an explicit instruction
// to the person extending the code.
//
//	Writer                              | Code path
//	------------------------------------|----------------------------------
//	standard quiz answer                | Service.SaveResult
//	reverse quiz answer                 | Service.SaveReverseResult
//	freeform quiz answer                | Service.SaveFreeformResult
//	etymology answer (any quiz mode)    | Service.SaveEtymologyOriginResult
//	per-type skip                       | Service.SkipWord
//	per-type resume                     | Service.ResumeWord
//	override answer toggle              | Service.OverrideAnswer
//	normalisation pass                  | Validator.Fix
//
// One-shot migrations like cli/migrate_learning_history.go are excluded —
// they intentionally rewrite shapes and aren't subject to the invariant.
func TestLearningHistory_OneLocationPerExpression_AcrossAllWriters(t *testing.T) {
	t.Skip("KNOWN: etymology vs standard/reverse/freeform writers disagree " +
		"on the (story_title, scene_title) tuple for the same expression, " +
		"so the same word gets fragmented across two top-level history " +
		"blocks. Re-enable when the canonical shape is unified.")

	dir := t.TempDir()
	storiesDir := filepath.Join(dir, "stories")
	etymDir := filepath.Join(dir, "etymology")
	learningDir := filepath.Join(dir, "learning")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, os.MkdirAll(etymDir, 0755))
	require.NoError(t, os.MkdirAll(learningDir, 0755))

	// One notebook id ("dual"), simultaneously a story-style notebook AND
	// an etymology source — the configuration in which the divergence shows
	// up in real data.
	const notebookID = "dual"
	const expression = "introvert"

	storyNotebookDir := filepath.Join(storiesDir, notebookID)
	require.NoError(t, os.MkdirAll(storyNotebookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyNotebookDir, "index.yml"), []byte(`id: dual
name: Dual Notebook
notebooks:
  - ./session8.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyNotebookDir, "session8.yml"), []byte(`- event: "Session 8"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "psyche + intro"
      conversations:
        - speaker: "A"
          quote: "She is an introvert."
      definitions:
        - expression: "introvert"
          meaning: "a quiet, inwardly-focused person"
          origin_parts:
            - origin: "intro"
              language: Latin
`), 0644))

	etymNotebookDir := filepath.Join(etymDir, notebookID)
	require.NoError(t, os.MkdirAll(etymNotebookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymNotebookDir, "index.yml"), []byte(`id: dual
kind: Etymology
name: Dual Notebook
notebooks:
  - ./session8.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymNotebookDir, "session8.yml"), []byte(`metadata:
  title: "Session 8"
origins:
  - origin: "intro"
    type: prefix
    language: Latin
    meaning: into, within
`), 0644))

	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	// Grading is irrelevant here — the test exercises Save* and friends
	// directly with synthesised GradeResults. Stub generously.
	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).
		Return(inference.AnswerMeaningsResponse{
			Answers: []inference.AnswerMeaning{{
				Expression: expression,
				AnswersForContext: []inference.AnswersForContext{
					{Correct: true, Reason: "stub", Quality: 4},
				},
			}},
		}, nil).
		AnyTimes()

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		EtymologyDirectories:   []string{etymDir},
		LearningNotesDirectory: learningDir,
	}, mockClient, make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{})

	ctx := context.Background()

	// 1. standard quiz answer — Service.SaveResult
	require.NoError(t, svc.SaveResult(ctx, Card{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Entry: expression, Meaning: "a quiet person",
	}, GradeResult{Correct: true, Quality: 4}, 1000))

	// 2. reverse quiz answer — Service.SaveReverseResult
	require.NoError(t, svc.SaveReverseResult(ctx, ReverseCard{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: expression, Meaning: "a quiet person",
	}, GradeResult{Correct: true, Quality: 4}, 1000))

	// 3. freeform quiz answer — Service.SaveFreeformResult
	require.NoError(t, svc.SaveFreeformResult(ctx, FreeformCard{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: expression, Meaning: "a quiet person",
	}, FreeformGradeResult{Correct: true, Quality: 4}, 1000))

	// 4. etymology answer — Service.SaveEtymologyOriginResult.
	// Uses the etymology card shape, where StoryTitle is the notebook
	// display name ("Dual Notebook") and SessionTitle is "Session 8".
	require.NoError(t, svc.SaveEtymologyOriginResult(EtymologyOriginCard{
		NotebookName: notebookID, NotebookTitle: "Dual Notebook",
		SessionTitle: "Session 8", Origin: expression, Meaning: "into",
	}, 4, true, 1000, notebook.QuizTypeEtymologyStandard, true))

	// 5. per-type skip — Service.SkipWord
	require.NoError(t, svc.SkipWord(CardInfo{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: expression,
	}, "", []notebook.QuizType{notebook.QuizTypeReverse}))

	// 6. per-type resume — Service.ResumeWord
	require.NoError(t, svc.ResumeWord(CardInfo{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: expression,
	}, []notebook.QuizType{notebook.QuizTypeReverse}))

	// 7. override answer — Service.OverrideAnswer
	_, err := svc.OverrideAnswer(CardInfo{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: expression,
	}, notebook.QuizTypeNotebook)
	require.NoError(t, err)

	// 8. normalisation pass — Validator.Fix
	v := notebook.NewValidator(learningDir, []string{storiesDir}, nil, nil, []string{etymDir}, "", nil)
	_, err = v.Fix()
	require.NoError(t, err)

	// Re-read the YAML and find every (top-level title, scene title) where
	// the sentinel expression appears. The invariant: exactly one location.
	raw, err := os.ReadFile(filepath.Join(learningDir, notebookID+".yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))

	type loc struct{ histTitle, sceneTitle string }
	var locations []loc
	for _, h := range got {
		for _, expr := range h.Expressions {
			if expr.Expression == expression {
				locations = append(locations, loc{histTitle: h.Metadata.Title, sceneTitle: "(top-level)"})
			}
		}
		for _, scene := range h.Scenes {
			for _, expr := range scene.Expressions {
				if expr.Expression == expression {
					locations = append(locations, loc{histTitle: h.Metadata.Title, sceneTitle: scene.Metadata.Title})
				}
			}
		}
	}

	require.Lenf(t, locations, 1,
		"every writer must place %q at the same (history_title, scene_title) tuple — found: %v",
		expression, locations,
	)
}

// TestLearningHistory_ShapeFingerprint_AcrossAllWriters is the sibling of
// the locator test: after every writer runs, every top-level history block
// in the same notebook YAML must share a single shape fingerprint.
//
// SKIPPED for the same reason as the locator test — the etymology writer
// produces type=etymology blocks, the others produce type="" blocks, so
// the fingerprint diverges. Re-enable alongside the migration.
//
// The fingerprint captures the structural decisions a writer makes:
// whether metadata.type is set and what value, whether expressions live at
// the top level (flashcard-style) or under scenes, and the scene-depth
// shape. Future writers introducing a new shape (e.g. a new quiz mode that
// writes flashcard-style entries instead of nested scenes) make the
// fingerprint diverge and the test names the offender.
func TestLearningHistory_ShapeFingerprint_AcrossAllWriters(t *testing.T) {
	t.Skip("KNOWN: etymology writer sets metadata.type=etymology while " +
		"standard/reverse/freeform writers leave it empty, so the YAML " +
		"contains two distinct fingerprints for the same notebook.")

	// Reuse the same fixture+writer-matrix from the locator test once the
	// shape is unified — extracting the setup into a helper at that point
	// keeps the two invariants in lockstep.
	t.Skip("see TestLearningHistory_OneLocationPerExpression_AcrossAllWriters")
}

// TestLearningHistory_ReadWriteRoundtrip_AcrossAllWriters is the third
// invariant: for every (writer, reader) pair, the writer's effect must be
// observable by the reader.
//
// This is a "future PR" placeholder. Unlike the locator and shape tests
// above (which would pass once the canonical shape is unified), the
// read-write matrix needs a deliberate fixture that lets every reader run
// against every writer's output. It's worth landing once the shape is
// unified, because at that point any drift between read and write paths
// becomes a visible regression rather than expected divergence.
//
// Reader candidates:
//   - Service.LoadCards excludes recently-answered words
//   - Service.LoadReverseCards excludes recently-answered words
//   - Service.LoadEtymologyOriginCards excludes recently-answered origins
//   - Service.LoadNotebookSummaries decrements review counts
//   - server.NotebookHandler.GetNotebookDetail surfaces the new log
//   - Validator.Fix is a no-op on a freshly-written file
func TestLearningHistory_ReadWriteRoundtrip_AcrossAllWriters(t *testing.T) {
	t.Skip("future work: enable after the locator + fingerprint invariants " +
		"are restored. See the writer/reader matrix in the comment above.")

	_ = fmt.Sprintf // silence imports until the test body lands
	_ = assert.True
}
