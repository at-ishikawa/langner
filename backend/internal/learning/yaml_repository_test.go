package learning

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

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
			repo := NewYAMLLearningRepository(dir, nil)

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

func TestYAMLLearningRepository_WriteAll(t *testing.T) {
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		notes   []notebook.NoteRecord
		logs    []LearningLog
		verify  func(t *testing.T, outputDir string)
		wantErr bool
	}{
		{
			name: "story learning history with scenes",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 1", Subgroup: "Opening Scene"},
					},
				},
				{
					ID:    2,
					Entry: "lose one's temper",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 1", Subgroup: "Conflict Scene"},
					},
				},
			},
			logs: []LearningLog{
				{NoteID: 1, Status: "understood", LearnedAt: baseTime, Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7, EasinessFactor: 2.5, SourceNotebookID: "test-series"},
				{NoteID: 2, Status: "misunderstood", LearnedAt: baseTime.Add(time.Hour), Quality: 2, ResponseTimeMs: 3000, QuizType: "notebook", IntervalDays: 1, EasinessFactor: 2.1, SourceNotebookID: "test-series"},
			},
			verify: func(t *testing.T, outputDir string) {
				filePath := filepath.Join(outputDir, "learning_notes", "test-series.yml")
				var histories []notebook.LearningHistory
				readYAMLHelper(t, filePath, &histories)
				require.Len(t, histories, 1)

				h := histories[0]
				assert.Equal(t, "test-series", h.Metadata.NotebookID)
				assert.Equal(t, "Episode 1", h.Metadata.Title)
				assert.Equal(t, "", h.Metadata.Type)

				require.Len(t, h.Scenes, 2)
				assert.Equal(t, "Opening Scene", h.Scenes[0].Metadata.Title)
				require.Len(t, h.Scenes[0].Expressions, 1)
				assert.Equal(t, "break the ice", h.Scenes[0].Expressions[0].Expression)
				assert.Equal(t, 2.5, h.Scenes[0].Expressions[0].EasinessFactor)
				require.Len(t, h.Scenes[0].Expressions[0].LearnedLogs, 1)
				assert.Equal(t, notebook.LearnedStatus("understood"), h.Scenes[0].Expressions[0].LearnedLogs[0].Status)
				assert.Equal(t, 4, h.Scenes[0].Expressions[0].LearnedLogs[0].Quality)
				assert.Equal(t, int64(1500), h.Scenes[0].Expressions[0].LearnedLogs[0].ResponseTimeMs)
				assert.Equal(t, "notebook", h.Scenes[0].Expressions[0].LearnedLogs[0].QuizType)
				assert.Equal(t, 7, h.Scenes[0].Expressions[0].LearnedLogs[0].IntervalDays)

				assert.Equal(t, "Conflict Scene", h.Scenes[1].Metadata.Title)
				require.Len(t, h.Scenes[1].Expressions, 1)
				assert.Equal(t, "lose one's temper", h.Scenes[1].Expressions[0].Expression)
			},
		},
		{
			name: "flashcard learning history with flat expressions",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
					},
				},
				{
					ID:    2,
					Entry: "lose one's temper",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
					},
				},
			},
			logs: []LearningLog{
				{NoteID: 1, Status: "understood", LearnedAt: baseTime, Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7, EasinessFactor: 2.5, SourceNotebookID: "vocab-cards"},
				{NoteID: 2, Status: "misunderstood", LearnedAt: baseTime, Quality: 2, ResponseTimeMs: 3000, QuizType: "notebook", IntervalDays: 1, EasinessFactor: 2.1, SourceNotebookID: "vocab-cards"},
			},
			verify: func(t *testing.T, outputDir string) {
				filePath := filepath.Join(outputDir, "learning_notes", "vocab-cards.yml")
				var histories []notebook.LearningHistory
				readYAMLHelper(t, filePath, &histories)
				require.Len(t, histories, 1)

				h := histories[0]
				assert.Equal(t, "vocab-cards", h.Metadata.NotebookID)
				assert.Equal(t, "Common Idioms", h.Metadata.Title)
				assert.Equal(t, "flashcard", h.Metadata.Type)
				assert.Empty(t, h.Scenes)

				require.Len(t, h.Expressions, 2)
				assert.Equal(t, "break the ice", h.Expressions[0].Expression)
				assert.Equal(t, 2.5, h.Expressions[0].EasinessFactor)
				assert.Equal(t, "lose one's temper", h.Expressions[1].Expression)
				assert.Equal(t, 2.1, h.Expressions[1].EasinessFactor)
			},
		},
		{
			name: "reverse logs split correctly",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Idioms"},
					},
				},
			},
			logs: []LearningLog{
				{NoteID: 1, Status: "understood", LearnedAt: baseTime, Quality: 4, QuizType: "notebook", EasinessFactor: 2.5, SourceNotebookID: "vocab-cards"},
				{NoteID: 1, Status: "understood", LearnedAt: baseTime.Add(time.Hour), Quality: 3, QuizType: "reverse", EasinessFactor: 2.3, SourceNotebookID: "vocab-cards"},
			},
			verify: func(t *testing.T, outputDir string) {
				filePath := filepath.Join(outputDir, "learning_notes", "vocab-cards.yml")
				var histories []notebook.LearningHistory
				readYAMLHelper(t, filePath, &histories)
				require.Len(t, histories, 1)
				require.Len(t, histories[0].Expressions, 1)

				expr := histories[0].Expressions[0]
				assert.Equal(t, 2.5, expr.EasinessFactor)
				assert.Equal(t, 2.3, expr.ReverseEasinessFactor)
				require.Len(t, expr.LearnedLogs, 1)
				assert.Equal(t, "notebook", expr.LearnedLogs[0].QuizType)
				require.Len(t, expr.ReverseLogs, 1)
				assert.Equal(t, "reverse", expr.ReverseLogs[0].QuizType)
			},
		},
		{
			name: "easiness factor from latest log per quiz type",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Idioms"},
					},
				},
			},
			logs: []LearningLog{
				{NoteID: 1, Status: "misunderstood", LearnedAt: baseTime, Quality: 2, QuizType: "notebook", EasinessFactor: 2.0, SourceNotebookID: "vocab-cards"},
				{NoteID: 1, Status: "understood", LearnedAt: baseTime.Add(24 * time.Hour), Quality: 4, QuizType: "notebook", EasinessFactor: 2.6, SourceNotebookID: "vocab-cards"},
				{NoteID: 1, Status: "misunderstood", LearnedAt: baseTime, Quality: 1, QuizType: "reverse", EasinessFactor: 1.8, SourceNotebookID: "vocab-cards"},
				{NoteID: 1, Status: "understood", LearnedAt: baseTime.Add(24 * time.Hour), Quality: 5, QuizType: "reverse", EasinessFactor: 2.4, SourceNotebookID: "vocab-cards"},
			},
			verify: func(t *testing.T, outputDir string) {
				filePath := filepath.Join(outputDir, "learning_notes", "vocab-cards.yml")
				var histories []notebook.LearningHistory
				readYAMLHelper(t, filePath, &histories)
				require.Len(t, histories, 1)

				expr := histories[0].Expressions[0]
				// Latest notebook log has EasinessFactor 2.6
				assert.Equal(t, 2.6, expr.EasinessFactor)
				// Latest reverse log has EasinessFactor 2.4
				assert.Equal(t, 2.4, expr.ReverseEasinessFactor)
			},
		},
		{
			name: "logs sorted newest first",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Idioms"},
					},
				},
			},
			logs: []LearningLog{
				{NoteID: 1, Status: "misunderstood", LearnedAt: baseTime, Quality: 2, QuizType: "notebook", EasinessFactor: 2.0, SourceNotebookID: "vocab-cards"},
				{NoteID: 1, Status: "understood", LearnedAt: baseTime.Add(24 * time.Hour), Quality: 4, QuizType: "notebook", EasinessFactor: 2.6, SourceNotebookID: "vocab-cards"},
			},
			verify: func(t *testing.T, outputDir string) {
				filePath := filepath.Join(outputDir, "learning_notes", "vocab-cards.yml")
				var histories []notebook.LearningHistory
				readYAMLHelper(t, filePath, &histories)
				require.Len(t, histories, 1)
				require.Len(t, histories[0].Expressions[0].LearnedLogs, 2)

				// Newest first
				assert.Equal(t, notebook.LearnedStatus("understood"), histories[0].Expressions[0].LearnedLogs[0].Status)
				assert.Equal(t, notebook.LearnedStatus("misunderstood"), histories[0].Expressions[0].LearnedLogs[1].Status)
			},
		},
		{
			name:  "empty logs produce no files",
			notes: []notebook.NoteRecord{},
			logs:  []LearningLog{},
			verify: func(t *testing.T, outputDir string) {
				_, err := os.Stat(filepath.Join(outputDir, "learning_notes"))
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "multiple notebooks produce separate files",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "series-a", Group: "Episode 1", Subgroup: "Scene 1"},
					},
				},
				{
					ID:    2,
					Entry: "lose one's temper",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
					},
				},
			},
			logs: []LearningLog{
				{NoteID: 1, Status: "understood", LearnedAt: baseTime, Quality: 4, QuizType: "notebook", EasinessFactor: 2.5, SourceNotebookID: "series-a"},
				{NoteID: 2, Status: "understood", LearnedAt: baseTime, Quality: 3, QuizType: "notebook", EasinessFactor: 2.3, SourceNotebookID: "vocab-cards"},
			},
			verify: func(t *testing.T, outputDir string) {
				// Verify both files exist
				fileA := filepath.Join(outputDir, "learning_notes", "series-a.yml")
				var historiesA []notebook.LearningHistory
				readYAMLHelper(t, fileA, &historiesA)
				require.Len(t, historiesA, 1)
				assert.Equal(t, "Episode 1", historiesA[0].Metadata.Title)
				assert.Equal(t, "", historiesA[0].Metadata.Type)

				fileB := filepath.Join(outputDir, "learning_notes", "vocab-cards.yml")
				var historiesB []notebook.LearningHistory
				readYAMLHelper(t, fileB, &historiesB)
				require.Len(t, historiesB, 1)
				assert.Equal(t, "flashcard", historiesB[0].Metadata.Type)
			},
		},
		{
			name: "note with no logs still appears as expression",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Idioms"},
					},
				},
			},
			logs: []LearningLog{},
			verify: func(t *testing.T, outputDir string) {
				filePath := filepath.Join(outputDir, "learning_notes", "vocab-cards.yml")
				var histories []notebook.LearningHistory
				readYAMLHelper(t, filePath, &histories)
				require.Len(t, histories, 1)
				require.Len(t, histories[0].Expressions, 1)
				assert.Equal(t, "break the ice", histories[0].Expressions[0].Expression)
				assert.Empty(t, histories[0].Expressions[0].LearnedLogs)
				assert.Empty(t, histories[0].Expressions[0].ReverseLogs)
			},
		},
		{
			name: "note in multiple notebooks has logs only in source notebook",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "series-a", Group: "Episode 1", Subgroup: "Scene 1"},
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Idioms"},
					},
				},
			},
			logs: []LearningLog{
				{NoteID: 1, Status: "understood", LearnedAt: baseTime, Quality: 4, QuizType: "notebook", EasinessFactor: 2.5, SourceNotebookID: "vocab-cards"},
			},
			verify: func(t *testing.T, outputDir string) {
				// series-a gets the expression but no logs (logs came from vocab-cards)
				fileA := filepath.Join(outputDir, "learning_notes", "series-a.yml")
				var historiesA []notebook.LearningHistory
				readYAMLHelper(t, fileA, &historiesA)
				require.Len(t, historiesA, 1)
				require.Len(t, historiesA[0].Scenes, 1)
				assert.Equal(t, "break the ice", historiesA[0].Scenes[0].Expressions[0].Expression)
				assert.Empty(t, historiesA[0].Scenes[0].Expressions[0].LearnedLogs)

				// vocab-cards gets the expression with logs (matching source_notebook_id)
				fileB := filepath.Join(outputDir, "learning_notes", "vocab-cards.yml")
				var historiesB []notebook.LearningHistory
				readYAMLHelper(t, fileB, &historiesB)
				require.Len(t, historiesB, 1)
				require.Len(t, historiesB[0].Expressions, 1)
				assert.Equal(t, "break the ice", historiesB[0].Expressions[0].Expression)
				require.Len(t, historiesB[0].Expressions[0].LearnedLogs, 1)
				assert.Equal(t, notebook.LearnedStatus("understood"), historiesB[0].Expressions[0].LearnedLogs[0].Status)
			},
		},
		{
			name: "multiple events in story produce separate histories",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 1", Subgroup: "Scene 1"},
					},
				},
				{
					ID:    2,
					Entry: "lose one's temper",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 2", Subgroup: "Scene 1"},
					},
				},
			},
			logs: []LearningLog{
				{NoteID: 1, Status: "understood", LearnedAt: baseTime, Quality: 4, QuizType: "notebook", EasinessFactor: 2.5, SourceNotebookID: "test-series"},
				{NoteID: 2, Status: "understood", LearnedAt: baseTime, Quality: 3, QuizType: "notebook", EasinessFactor: 2.3, SourceNotebookID: "test-series"},
			},
			verify: func(t *testing.T, outputDir string) {
				filePath := filepath.Join(outputDir, "learning_notes", "test-series.yml")
				var histories []notebook.LearningHistory
				readYAMLHelper(t, filePath, &histories)
				require.Len(t, histories, 2)
				assert.Equal(t, "Episode 1", histories[0].Metadata.Title)
				assert.Equal(t, "Episode 2", histories[1].Metadata.Title)
			},
		},
		{
			name: "multiple flashcard groups produce separate histories",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Idioms"},
					},
				},
				{
					ID:    2,
					Entry: "resilient",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Adjectives"},
					},
				},
			},
			logs: []LearningLog{
				{NoteID: 1, Status: "understood", LearnedAt: baseTime, Quality: 4, QuizType: "notebook", EasinessFactor: 2.5, SourceNotebookID: "vocab-cards"},
				{NoteID: 2, Status: "understood", LearnedAt: baseTime, Quality: 3, QuizType: "notebook", EasinessFactor: 2.3, SourceNotebookID: "vocab-cards"},
			},
			verify: func(t *testing.T, outputDir string) {
				filePath := filepath.Join(outputDir, "learning_notes", "vocab-cards.yml")
				var histories []notebook.LearningHistory
				readYAMLHelper(t, filePath, &histories)
				require.Len(t, histories, 2)
				assert.Equal(t, "Idioms", histories[0].Metadata.Title)
				assert.Equal(t, "Adjectives", histories[1].Metadata.Title)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputDir := t.TempDir()
			repo := NewYAMLLearningRepositoryWriter(outputDir)

			err := repo.WriteAll(tt.notes, tt.logs)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.verify(t, outputDir)
		})
	}
}

func TestYAMLLearningRepository_WriteAll_errors(t *testing.T) {
	tests := []struct {
		name  string
		notes []notebook.NoteRecord
		logs  []LearningLog
	}{
		{
			name: "write error on invalid path",
			notes: []notebook.NoteRecord{
				{
					ID:    1,
					Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Idioms"},
					},
				},
			},
			logs: []LearningLog{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewYAMLLearningRepositoryWriter("/dev/null/invalid")
			err := repo.WriteAll(tt.notes, tt.logs)
			assert.Error(t, err)
		})
	}
}

// readYAMLHelper is a test helper that reads and unmarshals a YAML file.
func readYAMLHelper(t *testing.T, path string, dest interface{}) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read file %s", path)
	require.NoError(t, yaml.Unmarshal(data, dest), "unmarshal %s", path)
}
