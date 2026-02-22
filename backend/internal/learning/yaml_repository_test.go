package learning

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYAMLLearningRepository_FindByNotebookID(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles map[string]string
		notebookID string
		wantLen    int
		wantExprs  []string
		wantErr    bool
	}{
		{
			name: "story type returns expressions from scenes",
			setupFiles: map[string]string{
				"vocab.yml": `- metadata:
    id: vocab-101
    title: Vocabulary Unit 1
  scenes:
    - metadata:
        title: Greetings
      expressions:
        - expression: break the ice
          learned_logs: []
        - expression: lose one's temper
          learned_logs: []`,
			},
			notebookID: "vocab-101",
			wantLen:    2,
			wantExprs:  []string{"break the ice", "lose one's temper"},
		},
		{
			name: "flashcard type returns top-level expressions",
			setupFiles: map[string]string{
				"flashcards.yml": `- metadata:
    id: flashcard-set-1
    title: Common Idioms
    type: flashcard
  expressions:
    - expression: hit the nail on the head
      learned_logs: []
    - expression: a piece of cake
      learned_logs: []`,
			},
			notebookID: "flashcard-set-1",
			wantLen:    2,
			wantExprs:  []string{"hit the nail on the head", "a piece of cake"},
		},
		{
			name: "non-matching notebook ID returns empty",
			setupFiles: map[string]string{
				"vocab.yml": `- metadata:
    id: vocab-101
    title: Vocabulary Unit 1
  scenes:
    - metadata:
        title: Greetings
      expressions:
        - expression: break the ice
          learned_logs: []`,
			},
			notebookID: "nonexistent-id",
			wantLen:    0,
		},
		{
			name: "multiple scenes in story type",
			setupFiles: map[string]string{
				"vocab.yml": `- metadata:
    id: vocab-201
    title: Vocabulary Unit 2
  scenes:
    - metadata:
        title: At the store
      expressions:
        - expression: window shopping
          learned_logs: []
    - metadata:
        title: At the park
      expressions:
        - expression: take a stroll
          learned_logs: []`,
			},
			notebookID: "vocab-201",
			wantLen:    2,
			wantExprs:  []string{"window shopping", "take a stroll"},
		},
		{
			name: "multiple files only returns matching notebook",
			setupFiles: map[string]string{
				"unit1.yml": `- metadata:
    id: unit-1
    title: Unit 1
  scenes:
    - metadata:
        title: Lesson 1
      expressions:
        - expression: once in a blue moon
          learned_logs: []`,
				"unit2.yml": `- metadata:
    id: unit-2
    title: Unit 2
  scenes:
    - metadata:
        title: Lesson 2
      expressions:
        - expression: under the weather
          learned_logs: []`,
			},
			notebookID: "unit-1",
			wantLen:    1,
			wantExprs:  []string{"once in a blue moon"},
		},
		{
			name:       "nonexistent directory returns error",
			setupFiles: nil,
			notebookID: "any-id",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dir string
			if tt.setupFiles == nil {
				dir = "/nonexistent/directory/for/test"
			} else {
				tempDir := t.TempDir()
				for filename, content := range tt.setupFiles {
					filePath := filepath.Join(tempDir, filename)
					err := os.WriteFile(filePath, []byte(content), 0644)
					require.NoError(t, err)
				}
				dir = tempDir
			}

			repo := NewYAMLLearningRepository(dir)
			got, err := repo.FindByNotebookID(tt.notebookID)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)

			for i, wantExpr := range tt.wantExprs {
				assert.Equal(t, wantExpr, got[i].Expression)
			}
		})
	}
}
