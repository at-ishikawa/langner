package datasync

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_dictionary "github.com/at-ishikawa/langner/internal/mocks/dictionary"
	mock_learning "github.com/at-ishikawa/langner/internal/mocks/learning"
	mock_note "github.com/at-ishikawa/langner/internal/mocks/note"
	"github.com/at-ishikawa/langner/internal/note"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestImporter_ImportNotes(t *testing.T) {
	tests := []struct {
		name       string
		stories    map[string]notebook.Index
		flashcards map[string]notebook.FlashcardIndex
		opts       ImportOptions
		setup      func(noteRepo *mock_note.MockNoteRepository)
		want       *ImportResult
	}{
		{
			name: "new story note is created",
			stories: map[string]notebook.Index{
				"test-story": {
					ID: "test-story",
					Notebooks: [][]notebook.StoryNotebook{
						{
							{
								Event: "Episode 1",
								Scenes: []notebook.StoryScene{
									{
										Title: "Opening",
										Definitions: []notebook.Note{
											{
												Expression: "break the ice",
												Definition: "start a conversation",
												Meaning:    "to initiate social interaction",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			flashcards: map[string]notebook.FlashcardIndex{},
			opts:       ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{}, nil)
				noteRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, n *note.Note) error {
						n.ID = 1
						assert.Equal(t, "break the ice", n.Usage)
						assert.Equal(t, "start a conversation", n.Entry)
						assert.Equal(t, "to initiate social interaction", n.Meaning)
						assert.Len(t, n.NotebookNotes, 1)
						assert.Equal(t, "story", n.NotebookNotes[0].NotebookType)
						assert.Equal(t, "test-story", n.NotebookNotes[0].NotebookID)
						assert.Equal(t, "Episode 1", n.NotebookNotes[0].Group)
						assert.Equal(t, "Opening", n.NotebookNotes[0].Subgroup)
						return nil
					})
			},
			want: &ImportResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
		{
			name: "existing note is skipped when UpdateExisting is false",
			stories: map[string]notebook.Index{
				"test-story": {
					ID: "test-story",
					Notebooks: [][]notebook.StoryNotebook{
						{
							{
								Event: "Episode 1",
								Scenes: []notebook.StoryScene{
									{
										Title: "Opening",
										Definitions: []notebook.Note{
											{
												Expression: "break the ice",
												Definition: "start a conversation",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			flashcards: map[string]notebook.FlashcardIndex{},
			opts:       ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation", NotebookNotes: []note.NotebookNote{
						{NoteID: 1, NotebookType: "story", NotebookID: "test-story", Group: "Episode 1"},
					}},
				}, nil)
			},
			want: &ImportResult{
				NotesSkipped:    1,
				NotebookSkipped: 1,
			},
		},
		{
			name: "existing note is updated when UpdateExisting is true",
			stories: map[string]notebook.Index{
				"test-story": {
					ID: "test-story",
					Notebooks: [][]notebook.StoryNotebook{
						{
							{
								Event: "Episode 1",
								Scenes: []notebook.StoryScene{
									{
										Title: "Opening",
										Definitions: []notebook.Note{
											{
												Expression: "break the ice",
												Definition: "start a conversation",
												Meaning:    "updated meaning",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			flashcards: map[string]notebook.FlashcardIndex{},
			opts:       ImportOptions{UpdateExisting: true},
			setup: func(noteRepo *mock_note.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation"},
				}, nil)
				noteRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, n *note.Note) error {
						assert.Equal(t, "updated meaning", n.Meaning)
						return nil
					})
				noteRepo.EXPECT().CreateNotebookNote(gomock.Any(), gomock.Any()).Return(nil)
			},
			want: &ImportResult{
				NotesUpdated: 1,
				NotebookNew:  1,
			},
		},
		{
			name:    "flashcard note is created with correct notebook type",
			stories: map[string]notebook.Index{},
			flashcards: map[string]notebook.FlashcardIndex{
				"vocab-cards": {
					ID:   "vocab-cards",
					Name: "Vocabulary Cards",
					Notebooks: []notebook.FlashcardNotebook{
						{
							Title: "Common Idioms",
							Cards: []notebook.Note{
								{
									Expression: "lose one's temper",
									Meaning:    "to become angry",
								},
							},
						},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{}, nil)
				noteRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, n *note.Note) error {
						n.ID = 2
						assert.Equal(t, "flashcard", n.NotebookNotes[0].NotebookType)
						assert.Equal(t, "vocab-cards", n.NotebookNotes[0].NotebookID)
						assert.Equal(t, "Common Idioms", n.NotebookNotes[0].Group)
						assert.Equal(t, "", n.NotebookNotes[0].Subgroup)
						return nil
					})
			},
			want: &ImportResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
		{
			name: "dry run does not create or update",
			stories: map[string]notebook.Index{
				"test-story": {
					ID: "test-story",
					Notebooks: [][]notebook.StoryNotebook{
						{
							{
								Event: "Episode 1",
								Scenes: []notebook.StoryScene{
									{
										Title: "Opening",
										Definitions: []notebook.Note{
											{
												Expression: "break the ice",
												Definition: "start a conversation",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			flashcards: map[string]notebook.FlashcardIndex{},
			opts:       ImportOptions{DryRun: true},
			setup: func(noteRepo *mock_note.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{}, nil)
				// No Create or CreateNotebookNote calls expected
			},
			want: &ImportResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
		{
			name: "note with empty definition uses expression as entry",
			stories: map[string]notebook.Index{
				"test-story": {
					ID: "test-story",
					Notebooks: [][]notebook.StoryNotebook{
						{
							{
								Event: "Episode 1",
								Scenes: []notebook.StoryScene{
									{
										Title: "Opening",
										Definitions: []notebook.Note{
											{
												Expression: "resilient",
												Meaning:    "able to recover quickly",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			flashcards: map[string]notebook.FlashcardIndex{},
			opts:       ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{}, nil)
				noteRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, n *note.Note) error {
						n.ID = 3
						assert.Equal(t, "resilient", n.Usage)
						assert.Equal(t, "resilient", n.Entry)
						return nil
					})
			},
			want: &ImportResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteRepo := mock_note.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)

			tt.setup(noteRepo)

			var buf bytes.Buffer
			imp := NewImporter(noteRepo, learningRepo, dictRepo, &buf)

			got, err := imp.ImportNotes(context.Background(), tt.stories, tt.flashcards, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestImporter_ImportLearningLogs(t *testing.T) {
	baseTime := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		histories map[string][]notebook.LearningHistory
		opts      ImportOptions
		setup     func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository)
		want      *ImportResult
	}{
		{
			name: "new learning log is created",
			histories: map[string][]notebook.LearningHistory{
				"test-story": {
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "test-story",
							Title:      "Episode 1",
							Type:       "story",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{Title: "Opening"},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression:     "break the ice",
										EasinessFactor: 2.5,
										LearnedLogs: []notebook.LearningRecord{
											{
												Status:    "understood",
												LearnedAt: notebook.NewDate(baseTime),
												Quality:   4,
												QuizType:  "notebook",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{
					{ID: 1, Entry: "break the ice"},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, int64(1), logs[0].NoteID)
						assert.Equal(t, "understood", logs[0].Status)
						assert.Equal(t, "notebook", logs[0].QuizType)
						assert.Equal(t, 4, logs[0].Quality)
						assert.Equal(t, 2.5, logs[0].EasinessFactor)
						return nil
					})
			},
			want: &ImportResult{
				LearningNew: 1,
			},
		},
		{
			name: "duplicate learning log is skipped",
			histories: map[string][]notebook.LearningHistory{
				"test-story": {
					{
						Metadata: notebook.LearningHistoryMetadata{Type: "story"},
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "break the ice",
										LearnedLogs: []notebook.LearningRecord{
											{
												Status:    "understood",
												LearnedAt: notebook.NewDate(baseTime),
												QuizType:  "notebook",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{
					{ID: 1, Entry: "break the ice"},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{
					{ID: 1, NoteID: 1, QuizType: "notebook", LearnedAt: baseTime},
				}, nil)
			},
			want: &ImportResult{
				LearningSkipped: 1,
			},
		},
		{
			name: "missing note is auto-created",
			histories: map[string][]notebook.LearningHistory{
				"test-story": {
					{
						Metadata: notebook.LearningHistoryMetadata{Type: "story"},
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "unknown phrase",
										LearnedLogs: []notebook.LearningRecord{
											{
												Status:    "understood",
												LearnedAt: notebook.NewDate(baseTime),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				first := noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{}, nil)
				noteRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, notes []*note.Note) error {
						require.Len(t, notes, 1)
						assert.Equal(t, "unknown phrase", notes[0].Usage)
						assert.Equal(t, "unknown phrase", notes[0].Entry)
						return nil
					}).After(first)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{
					{ID: 10, Entry: "unknown phrase", Usage: "unknown phrase"},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, int64(10), logs[0].NoteID)
						return nil
					})
			},
			want: &ImportResult{
				NotesNew:    1,
				LearningNew: 1,
			},
		},
		{
			name: "flashcard type reads expressions directly",
			histories: map[string][]notebook.LearningHistory{
				"vocab": {
					{
						Metadata: notebook.LearningHistoryMetadata{Type: "flashcard"},
						Expressions: []notebook.LearningHistoryExpression{
							{
								Expression:     "resilient",
								EasinessFactor: 2.3,
								LearnedLogs: []notebook.LearningRecord{
									{
										Status:    "understood",
										LearnedAt: notebook.NewDate(baseTime),
										QuizType:  "freeform",
									},
								},
							},
						},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{
					{ID: 5, Entry: "resilient"},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, "freeform", logs[0].QuizType)
						assert.Equal(t, 2.3, logs[0].EasinessFactor)
						return nil
					})
			},
			want: &ImportResult{
				LearningNew: 1,
			},
		},
		{
			name: "reverse logs use forced quiz type",
			histories: map[string][]notebook.LearningHistory{
				"test": {
					{
						Metadata: notebook.LearningHistoryMetadata{Type: "flashcard"},
						Expressions: []notebook.LearningHistoryExpression{
							{
								Expression:            "resilient",
								ReverseEasinessFactor: 2.1,
								ReverseLogs: []notebook.LearningRecord{
									{
										Status:    "misunderstood",
										LearnedAt: notebook.NewDate(baseTime),
										QuizType:  "notebook",
									},
								},
							},
						},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{
					{ID: 5, Entry: "resilient"},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, "reverse", logs[0].QuizType)
						assert.Equal(t, 2.1, logs[0].EasinessFactor)
						return nil
					})
			},
			want: &ImportResult{
				LearningNew: 1,
			},
		},
		{
			name: "empty quiz type defaults to notebook",
			histories: map[string][]notebook.LearningHistory{
				"test": {
					{
						Metadata: notebook.LearningHistoryMetadata{Type: "story"},
						Scenes: []notebook.LearningScene{
							{
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "break the ice",
										LearnedLogs: []notebook.LearningRecord{
											{
												Status:    "understood",
												LearnedAt: notebook.NewDate(baseTime),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{
					{ID: 1, Entry: "break the ice"},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, "notebook", logs[0].QuizType)
						return nil
					})
			},
			want: &ImportResult{
				LearningNew: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteRepo := mock_note.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)

			tt.setup(noteRepo, learningRepo)

			var buf bytes.Buffer
			imp := NewImporter(noteRepo, learningRepo, dictRepo, &buf)

			got, err := imp.ImportLearningLogs(context.Background(), tt.histories, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestImporter_ImportDictionary(t *testing.T) {
	tests := []struct {
		name      string
		responses []rapidapi.Response
		opts      ImportOptions
		setup     func(dictRepo *mock_dictionary.MockDictionaryRepository)
		want      *ImportResult
	}{
		{
			name: "new dictionary entry is created",
			responses: []rapidapi.Response{
				{Word: "resilient", Results: []rapidapi.Result{{Definition: "able to recover"}}},
			},
			opts: ImportOptions{},
			setup: func(dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{}, nil)
				dictRepo.EXPECT().BatchUpsert(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, entries []*dictionary.DictionaryEntry) error {
						require.Len(t, entries, 1)
						assert.Equal(t, "resilient", entries[0].Word)
						assert.Equal(t, "rapidapi", entries[0].SourceType)
						assert.NotEmpty(t, entries[0].Response)
						return nil
					})
			},
			want: &ImportResult{
				DictionaryNew: 1,
			},
		},
		{
			name: "existing entry is skipped when UpdateExisting is false",
			responses: []rapidapi.Response{
				{Word: "resilient"},
			},
			opts: ImportOptions{},
			setup: func(dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{
					{Word: "resilient"},
				}, nil)
				dictRepo.EXPECT().BatchUpsert(gomock.Any(), gomock.Any()).Return(nil)
			},
			want: &ImportResult{
				DictionarySkipped: 1,
			},
		},
		{
			name: "existing entry is updated when UpdateExisting is true",
			responses: []rapidapi.Response{
				{Word: "resilient", Results: []rapidapi.Result{{Definition: "updated"}}},
			},
			opts: ImportOptions{UpdateExisting: true},
			setup: func(dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{
					{Word: "resilient", Response: json.RawMessage(`{}`)},
				}, nil)
				dictRepo.EXPECT().BatchUpsert(gomock.Any(), gomock.Any()).Return(nil)
			},
			want: &ImportResult{
				DictionaryUpdated: 1,
			},
		},
		{
			name: "dry run does not upsert",
			responses: []rapidapi.Response{
				{Word: "resilient"},
			},
			opts: ImportOptions{DryRun: true},
			setup: func(dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{}, nil)
			},
			want: &ImportResult{
				DictionaryNew: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteRepo := mock_note.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)

			tt.setup(dictRepo)

			var buf bytes.Buffer
			imp := NewImporter(noteRepo, learningRepo, dictRepo, &buf)

			got, err := imp.ImportDictionary(context.Background(), tt.responses, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExporter_Export(t *testing.T) {
	tests := []struct {
		name  string
		setup func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository, dictRepo *mock_dictionary.MockDictionaryRepository)
		want  *ExportData
	}{
		{
			name: "exports all data from repos",
			setup: func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository, dictRepo *mock_dictionary.MockDictionaryRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{
					{ID: 1, Usage: "break the ice"},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{
					{ID: 1, NoteID: 1, Status: "understood"},
				}, nil)
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{
					{Word: "resilient"},
				}, nil)
			},
			want: &ExportData{
				Notes:             []note.Note{{ID: 1, Usage: "break the ice"}},
				LearningLogs:      []learning.LearningLog{{ID: 1, NoteID: 1, Status: "understood"}},
				DictionaryEntries: []dictionary.DictionaryEntry{{Word: "resilient"}},
			},
		},
		{
			name: "returns empty data when repos are empty",
			setup: func(noteRepo *mock_note.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository, dictRepo *mock_dictionary.MockDictionaryRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]note.Note{}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{}, nil)
			},
			want: &ExportData{
				Notes:             []note.Note{},
				LearningLogs:      []learning.LearningLog{},
				DictionaryEntries: []dictionary.DictionaryEntry{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteRepo := mock_note.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)

			tt.setup(noteRepo, learningRepo, dictRepo)

			exporter := NewExporter(noteRepo, learningRepo, dictRepo)
			got, err := exporter.Export(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
