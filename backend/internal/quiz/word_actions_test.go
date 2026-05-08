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

// TestService_SkipWord_DoesNotForgeLearningLog covers the bug the user
// reported: clicking Skip on a vocabulary-book word that hadn't been
// studied before fabricated a "quality 5 / understood / 30-day interval"
// learned_log entry, making the YAML look as if the word had been
// answered correctly. The skip should record only skipped_at, leaving
// learned_logs empty.
//
// This is a real disk-backed integration test: it instantiates a real
// quiz.Service, writes to a temp learning_notes directory via the
// production WriteYamlFile path, and then re-reads the YAML to verify
// the on-disk shape — no mocked YAML, no in-memory shortcut.
func TestService_SkipWord_DoesNotForgeLearningLog(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()
	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{t.TempDir()},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	info := CardInfo{
		NotebookName: "word-power-made-easy",
		StoryTitle:   "Session 3",
		SceneTitle:   "verto (to turn)",
		Expression:   "ambivert",
	}

	require.NoError(t, svc.SkipWord(info, "", notebook.QuizTypeNotebook))
	require.NoError(t, svc.SkipWord(info, "", notebook.QuizTypeReverse))
	require.NoError(t, svc.SkipWord(info, "", notebook.QuizTypeFreeform))

	raw, err := os.ReadFile(filepath.Join(learningDir, "word-power-made-easy.yml"))
	require.NoError(t, err)

	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1, "exactly one history block expected")
	assert.Equal(t, "Session 3", got[0].Metadata.Title)
	require.Len(t, got[0].Scenes, 1)
	assert.Equal(t, "verto (to turn)", got[0].Scenes[0].Metadata.Title)
	require.Len(t, got[0].Scenes[0].Expressions, 1)

	expr := got[0].Scenes[0].Expressions[0]
	assert.Equal(t, "ambivert", expr.Expression)
	assert.Empty(t, expr.LearnedLogs, "skip must not fabricate learned_logs entries")
	assert.Empty(t, expr.ReverseLogs, "skip must not fabricate reverse_logs entries")
	assert.Contains(t, expr.SkippedAt, "notebook")
	assert.Contains(t, expr.SkippedAt, "reverse")
	assert.Contains(t, expr.SkippedAt, "freeform")
}

// TestService_SkipWord_PreservesExistingLogs verifies that when a word
// already has real learning history, SkipWord attaches the skip to the
// existing entry instead of creating a duplicate. The original logs
// must remain untouched.
func TestService_SkipWord_PreservesExistingLogs(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()

	// Pre-seed a history file with a real learned_log entry.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "word-power-made-easy.yml"), []byte(`- metadata:
    id: word-power-made-easy
    title: Session 3
  scenes:
    - metadata:
        title: verto (to turn)
      expressions:
        - expression: ambivert
          learned_logs:
            - status: misunderstood
              learned_at: "2026-04-01T10:00:00Z"
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
		NotebookName: "word-power-made-easy",
		StoryTitle:   "Session 3",
		SceneTitle:   "verto (to turn)",
		Expression:   "ambivert",
	}
	require.NoError(t, svc.SkipWord(info, "", notebook.QuizTypeReverse))

	raw, err := os.ReadFile(filepath.Join(learningDir, "word-power-made-easy.yml"))
	require.NoError(t, err)

	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	require.Len(t, got[0].Scenes, 1)
	require.Len(t, got[0].Scenes[0].Expressions, 1, "must not duplicate the entry")

	expr := got[0].Scenes[0].Expressions[0]
	assert.Equal(t, "ambivert", expr.Expression)
	require.Len(t, expr.LearnedLogs, 1, "original log must be preserved")
	assert.Equal(t, notebook.LearnedStatus("misunderstood"), expr.LearnedLogs[0].Status)
	assert.Equal(t, 1, expr.LearnedLogs[0].Quality)
	assert.Contains(t, expr.SkippedAt, "reverse")
}
