package learning

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestYAMLLearningRepository_FindByNotebookID(t *testing.T) {
	tests := []struct {
		name       string
		setupDir   func(t *testing.T) string
		notebookID string
		want       []notebook.LearningHistoryExpression
		wantErr    bool
	}{
		{
			name: "flashcard type returns top-level expressions",
			setupDir: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    id: "notebook-1"
    title: "Common Idioms"
    type: "flashcard"
  expressions:
    - expression: "break the ice"
      learned_logs: []
    - expression: "lose one's temper"
      learned_logs: []
`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "idioms.yml"), []byte(content), 0644))
				return dir
			},
			notebookID: "notebook-1",
			want: []notebook.LearningHistoryExpression{
				{Expression: "break the ice", LearnedLogs: []notebook.LearningRecord{}},
				{Expression: "lose one's temper", LearnedLogs: []notebook.LearningRecord{}},
			},
		},
		{
			name: "story type returns expressions from all scenes",
			setupDir: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    id: "notebook-2"
    title: "Story Notebook"
  scenes:
    - metadata:
        title: "Scene 1"
      expressions:
        - expression: "take a break"
          learned_logs: []
    - metadata:
        title: "Scene 2"
      expressions:
        - expression: "hit the road"
          learned_logs: []
`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "stories.yml"), []byte(content), 0644))
				return dir
			},
			notebookID: "notebook-2",
			want: []notebook.LearningHistoryExpression{
				{Expression: "take a break", LearnedLogs: []notebook.LearningRecord{}},
				{Expression: "hit the road", LearnedLogs: []notebook.LearningRecord{}},
			},
		},
		{
			name: "no matching notebook returns nil",
			setupDir: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    id: "notebook-1"
    title: "Common Idioms"
    type: "flashcard"
  expressions:
    - expression: "break the ice"
      learned_logs: []
`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "idioms.yml"), []byte(content), 0644))
				return dir
			},
			notebookID: "nonexistent-id",
		},
		{
			name: "invalid directory returns error",
			setupDir: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			notebookID: "notebook-1",
			wantErr:    true,
		},
		{
			name: "multiple entries only matching returned",
			setupDir: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    id: "notebook-1"
    title: "Common Idioms"
    type: "flashcard"
  expressions:
    - expression: "break the ice"
      learned_logs: []
- metadata:
    id: "notebook-2"
    title: "Advanced Phrases"
    type: "flashcard"
  expressions:
    - expression: "hit the nail on the head"
      learned_logs: []
`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "mixed.yml"), []byte(content), 0644))
				return dir
			},
			notebookID: "notebook-1",
			want: []notebook.LearningHistoryExpression{
				{Expression: "break the ice", LearnedLogs: []notebook.LearningRecord{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setupDir(t)
			repo := NewYAMLLearningRepository(dir)

			got, err := repo.FindByNotebookID(tt.notebookID)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
