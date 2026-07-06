package datasync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_datasync "github.com/at-ishikawa/langner/internal/mocks/datasync"
	mock_dictionary "github.com/at-ishikawa/langner/internal/mocks/dictionary"
	mock_learning "github.com/at-ishikawa/langner/internal/mocks/learning"
	mock_notebook "github.com/at-ishikawa/langner/internal/mocks/notebook"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestImporter_ImportNotes(t *testing.T) {
	tests := []struct {
		name        string
		sourceNotes []notebook.NoteRecord
		opts        ImportOptions
		setup       func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository)
		want        *ImportNotesResult
		wantErr     bool
	}{
		{
			name: "new note is created",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "break the ice",
					Entry:   "start a conversation",
					Meaning: "to initiate social interaction",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage:   "break the ice",
						Entry:   "start a conversation",
						Meaning: "to initiate social interaction",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, notes []*notebook.NoteRecord) error {
						require.Len(t, notes, 1)
						assert.Equal(t, "break the ice", notes[0].Usage)
						assert.Equal(t, "start a conversation", notes[0].Entry)
						assert.Equal(t, "to initiate social interaction", notes[0].Meaning)
						require.Len(t, notes[0].NotebookNotes, 1)
						assert.Equal(t, "story", notes[0].NotebookNotes[0].NotebookType)
						assert.Equal(t, "test-story", notes[0].NotebookNotes[0].NotebookID)
						assert.Equal(t, "Episode 1", notes[0].NotebookNotes[0].Group)
						assert.Equal(t, "Opening", notes[0].NotebookNotes[0].Subgroup)
						return nil
					})
			},
			want: &ImportNotesResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
		{
			name: "existing note is skipped when UpdateExisting is false",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice",
					Entry: "start a conversation",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage: "break the ice",
						Entry: "start a conversation",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation", NotebookNotes: []notebook.NotebookNote{
						{NoteID: 1, NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					}},
				}, nil)
			},
			want: &ImportNotesResult{
				NotesSkipped:    1,
				NotebookSkipped: 1,
			},
		},
		{
			name: "existing note is updated when UpdateExisting is true",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "break the ice",
					Entry:   "start a conversation",
					Meaning: "updated meaning",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			opts: ImportOptions{UpdateExisting: true},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage:   "break the ice",
						Entry:   "start a conversation",
						Meaning: "updated meaning",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation"},
				}, nil)
				noteRepo.EXPECT().BatchUpdate(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, notes []*notebook.NoteRecord, newNNs []notebook.NotebookNote) error {
						require.Len(t, notes, 1)
						assert.Equal(t, "updated meaning", notes[0].Meaning)
						require.Len(t, newNNs, 1)
						assert.Equal(t, "story", newNNs[0].NotebookType)
						assert.Equal(t, "test-story", newNNs[0].NotebookID)
						return nil
					})
			},
			want: &ImportNotesResult{
				NotesUpdated: 1,
				NotebookNew:  1,
			},
		},
		{
			name: "existing note's concept_key is backfilled when UpdateExisting is true",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:      "brighten",
					Entry:      "brighten",
					Meaning:    "to make or become bright",
					ConceptKey: "bright",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "book", NotebookID: "demo", Group: "Brightness", Subgroup: ""},
					},
				},
			},
			opts: ImportOptions{UpdateExisting: true},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage:      "brighten",
						Entry:      "brighten",
						Meaning:    "to make or become bright",
						ConceptKey: "bright",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "book", NotebookID: "demo", Group: "Brightness", Subgroup: ""},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 42, Usage: "brighten", Entry: "brighten", ConceptKey: ""}, // pre-existing row with no concept_key
				}, nil)
				noteRepo.EXPECT().BatchUpdate(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, notes []*notebook.NoteRecord, newNNs []notebook.NotebookNote) error {
						require.Len(t, notes, 1)
						assert.Equal(t, "bright", notes[0].ConceptKey,
							"UpdateExisting must propagate ConceptKey from source onto existing row")
						return nil
					})
			},
			want: &ImportNotesResult{
				NotesUpdated: 1,
				NotebookNew:  1,
			},
		},
		{
			name: "skipped note with new notebook_note only inserts notebook_note",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "break the ice",
					Entry:   "start a conversation",
					Meaning: "to initiate social interaction",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage:   "break the ice",
						Entry:   "start a conversation",
						Meaning: "to initiate social interaction",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation"},
				}, nil)
				noteRepo.EXPECT().BatchUpdate(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, notes []*notebook.NoteRecord, newNNs []notebook.NotebookNote) error {
						assert.Empty(t, notes)
						require.Len(t, newNNs, 1)
						assert.Equal(t, "flashcard", newNNs[0].NotebookType)
						assert.Equal(t, "vocab-cards", newNNs[0].NotebookID)
						assert.Equal(t, "Common Idioms", newNNs[0].Group)
						return nil
					})
			},
			want: &ImportNotesResult{
				NotesSkipped: 1,
				NotebookNew:  1,
			},
		},
		{
			name: "flashcard note is created with correct notebook type",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "lose one's temper",
					Entry:   "lose one's temper",
					Meaning: "to become angry",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage:   "lose one's temper",
						Entry:   "lose one's temper",
						Meaning: "to become angry",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, notes []*notebook.NoteRecord) error {
						require.Len(t, notes, 1)
						assert.Equal(t, "flashcard", notes[0].NotebookNotes[0].NotebookType)
						assert.Equal(t, "vocab-cards", notes[0].NotebookNotes[0].NotebookID)
						assert.Equal(t, "Common Idioms", notes[0].NotebookNotes[0].Group)
						assert.Equal(t, "", notes[0].NotebookNotes[0].Subgroup)
						return nil
					})
			},
			want: &ImportNotesResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
		{
			name: "dry run does not create or update",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice",
					Entry: "start a conversation",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			opts: ImportOptions{DryRun: true},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage: "break the ice",
						Entry: "start a conversation",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				// No BatchCreate or BatchUpdate calls expected
			},
			want: &ImportNotesResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
		{
			name: "note with images and references",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "resilient",
					Entry:   "resilient",
					Meaning: "able to recover",
					Images: []notebook.NoteImage{
						{URL: "https://example.com/img1.png", SortOrder: 0},
						{URL: "https://example.com/img2.png", SortOrder: 1},
					},
					References: []notebook.NoteReference{
						{Link: "https://example.com/ref1", Description: "Reference 1", SortOrder: 0},
					},
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage:   "resilient",
						Entry:   "resilient",
						Meaning: "able to recover",
						Images: []notebook.NoteImage{
							{URL: "https://example.com/img1.png", SortOrder: 0},
							{URL: "https://example.com/img2.png", SortOrder: 1},
						},
						References: []notebook.NoteReference{
							{Link: "https://example.com/ref1", Description: "Reference 1", SortOrder: 0},
						},
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, notes []*notebook.NoteRecord) error {
						require.Len(t, notes, 1)
						require.Len(t, notes[0].Images, 2)
						assert.Equal(t, "https://example.com/img1.png", notes[0].Images[0].URL)
						assert.Equal(t, 0, notes[0].Images[0].SortOrder)
						assert.Equal(t, "https://example.com/img2.png", notes[0].Images[1].URL)
						assert.Equal(t, 1, notes[0].Images[1].SortOrder)
						require.Len(t, notes[0].References, 1)
						assert.Equal(t, "https://example.com/ref1", notes[0].References[0].Link)
						assert.Equal(t, "Reference 1", notes[0].References[0].Description)
						assert.Equal(t, 0, notes[0].References[0].SortOrder)
						return nil
					})
			},
			want: &ImportNotesResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
		{
			name: "NoteSource FindAll error",
			opts: ImportOptions{},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("source read failed"))
			},
			wantErr: true,
		},
		{
			name:        "FindAll error",
			sourceNotes: []notebook.NoteRecord{},
			opts:        ImportOptions{},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "BatchCreate error propagates",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "resilient",
					Entry:   "resilient",
					Meaning: "able to recover quickly",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage:   "resilient",
						Entry:   "resilient",
						Meaning: "able to recover quickly",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).Return(fmt.Errorf("insert failed"))
			},
			wantErr: true,
		},
		{
			name: "BatchUpdate error propagates",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "break the ice",
					Entry:   "start a conversation",
					Meaning: "updated meaning",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			opts: ImportOptions{UpdateExisting: true},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage:   "break the ice",
						Entry:   "start a conversation",
						Meaning: "updated meaning",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation"},
				}, nil)
				noteRepo.EXPECT().BatchUpdate(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("update failed"))
			},
			wantErr: true,
		},
		{
			name: "BatchUpdate error propagates for new notebook_note on existing note",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "break the ice",
					Entry:   "start a conversation",
					Meaning: "to initiate social interaction",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteSource *mock_datasync.MockNoteSource, noteRepo *mock_notebook.MockNoteRepository) {
				noteSource.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{
						Usage:   "break the ice",
						Entry:   "start a conversation",
						Meaning: "to initiate social interaction",
						NotebookNotes: []notebook.NotebookNote{
							{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
						},
					},
				}, nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation"},
				}, nil)
				noteRepo.EXPECT().BatchUpdate(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("insert failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteSource := mock_datasync.NewMockNoteSource(ctrl)
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictSource := mock_datasync.NewMockDictionarySource(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)

			tt.setup(noteSource, noteRepo)

			var buf bytes.Buffer
			imp := NewImporter(noteRepo, learningRepo, noteSource, nil, dictSource, dictRepo, &buf)

			got, err := imp.ImportNotes(context.Background(), tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestImporter_ImportLearningLogs(t *testing.T) {
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		opts    ImportOptions
		setup   func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository)
		want    *ImportLearningLogsResult
		wantErr bool
	}{
		{
			name: "new learning log is created",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression:     "break the ice",
						EasinessFactor: 2.5,
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7},
						},
					},
				}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, int64(1), logs[0].NoteID)
						assert.Equal(t, "understood", logs[0].Status)
						assert.Equal(t, baseTime, logs[0].LearnedAt)
						assert.Equal(t, 4, logs[0].Quality)
						assert.Equal(t, 1500, logs[0].ResponseTimeMs)
						assert.Equal(t, "notebook", logs[0].QuizType)
						assert.Equal(t, 7, logs[0].IntervalDays)
						return nil
					})
			},
			want: &ImportLearningLogsResult{
				LearningNew: 1,
			},
		},
		{
			name: "duplicate learning log is skipped",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{
					{NoteID: 1, QuizType: "notebook", LearnedAt: baseTime, SourceNotebookID: "test-story", Status: "understood"},
				}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression:     "break the ice",
						EasinessFactor: 2.5,
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7},
						},
					},
				}, nil)
			},
			want: &ImportLearningLogsResult{
				LearningSkipped: 1,
			},
		},
		{
			name: "missing note is auto-created",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 99, Entry: "other-word", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression:     "lose one's temper",
						EasinessFactor: 2.5,
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7},
						},
					},
				}, nil)
				noteRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, notes []*notebook.NoteRecord) error {
						require.Len(t, notes, 1)
						assert.Equal(t, "lose one's temper", notes[0].Usage)
						assert.Equal(t, "lose one's temper", notes[0].Entry)
						return nil
					})
				// Re-fetch after batch create
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 99, Entry: "other-word"},
					{ID: 42, Entry: "lose one's temper"},
				}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, int64(42), logs[0].NoteID)
						return nil
					})
			},
			want: &ImportLearningLogsResult{
				NotesNew:    1,
				LearningNew: 1,
			},
		},
		{
			name: "flashcard type reads expressions directly",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice", NotebookNotes: []notebook.NotebookNote{{NotebookID: "vocab-cards"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("vocab-cards").Return([]notebook.LearningHistoryExpression{
					{
						Expression:     "break the ice",
						EasinessFactor: 2.5,
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7},
						},
					},
				}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, int64(1), logs[0].NoteID)
						assert.Equal(t, "notebook", logs[0].QuizType)
						return nil
					})
			},
			want: &ImportLearningLogsResult{
				LearningNew: 1,
			},
		},
		{
			name: "reverse logs use forced quiz type",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression:            "break the ice",
						ReverseEasinessFactor: 2.3,
						ReverseLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 3, ResponseTimeMs: 2000, IntervalDays: 5},
						},
					},
				}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, "reverse", logs[0].QuizType)
						return nil
					})
			},
			want: &ImportLearningLogsResult{
				LearningNew: 1,
			},
		},
		{
			name: "empty quiz type defaults to notebook",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression:     "break the ice",
						EasinessFactor: 2.5,
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, IntervalDays: 7},
						},
					},
				}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1)
						assert.Equal(t, "notebook", logs[0].QuizType)
						return nil
					})
			},
			want: &ImportLearningLogsResult{
				LearningNew: 1,
			},
		},
		{
			name: "noteRepo.FindAll error",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "learningRepo.FindAll error",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "learningSource.FindByNotebookID error",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return(nil, fmt.Errorf("read failed"))
			},
			wantErr: true,
		},
		{
			name: "noteRepo.BatchCreate error",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 99, Entry: "other-word", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression: "lose one's temper",
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7},
						},
					},
				}, nil)
				noteRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).Return(fmt.Errorf("insert failed"))
			},
			wantErr: true,
		},
		{
			name: "learningRepo.BatchCreate error",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression:     "break the ice",
						EasinessFactor: 2.5,
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7},
						},
					},
				}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).Return(fmt.Errorf("insert failed"))
			},
			wantErr: true,
		},
		{
			name: "re-fetch FindAll error after BatchCreate",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 99, Entry: "other-word", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression: "lose one's temper",
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7},
						},
					},
				}, nil)
				noteRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).Return(nil)
				noteRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "dry-run skips notes not in noteMap",
			opts: ImportOptions{DryRun: true},
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 99, Entry: "other-word", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression: "lose one's temper",
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7},
						},
					},
				}, nil)
				// No BatchCreate calls expected in dry-run
			},
			want: &ImportLearningLogsResult{
				NotesNew: 1,
			},
		},
		{
			name: "multiple notes with same entry uses notebook-specific note",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "took off", Entry: "take off", NotebookNotes: []notebook.NotebookNote{{NotebookID: "series-a"}}},
					{ID: 2, Usage: "take off", Entry: "take off", NotebookNotes: []notebook.NotebookNote{{NotebookID: "series-b"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("series-a").Return([]notebook.LearningHistoryExpression{
					{
						Expression:     "take off",
						EasinessFactor: 2.5,
						LearnedLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7},
						},
					},
				}, nil)
				learningSource.EXPECT().FindByNotebookID("series-b").Return([]notebook.LearningHistoryExpression{
					{
						Expression:     "take off",
						EasinessFactor: 2.3,
						LearnedLogs: []notebook.LearningRecord{
							{Status: "misunderstood", LearnedAt: notebook.NewDate(baseTime.Add(time.Hour)), Quality: 2, ResponseTimeMs: 3000, QuizType: "notebook", IntervalDays: 1},
						},
					},
				}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 2)
						// series-a log should use note ID 1 (the note linked to series-a)
						assert.Equal(t, int64(1), logs[0].NoteID)
						assert.Equal(t, "series-a", logs[0].SourceNotebookID)
						// series-b log should use note ID 2 (the note linked to series-b)
						assert.Equal(t, int64(2), logs[1].NoteID)
						assert.Equal(t, "series-b", logs[1].SourceNotebookID)
						return nil
					})
			},
			want: &ImportLearningLogsResult{
				LearningNew: 2,
			},
		},
		{
			// An etymology_freeform answer is written to both
			// EtymologyBreakdownLogs AND EtymologyAssemblyLogs on the
			// YAML side (AddRecordWithQualityForEtymology mirrors it).
			// The importer must recognise the assembly copy as a
			// duplicate of the breakdown copy and insert only one DB
			// row per event. Without this, sync-db doubles every
			// freeform log and the round-trip validate-db reports +2
			// per short polysemous root like "alter" — which is the
			// exact failure the user hit in production.
			name: "etymology_freeform log is inserted once even when mirrored to both slots",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "alter", NotebookNotes: []notebook.NotebookNote{{NotebookID: "wpme"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("wpme").Return([]notebook.LearningHistoryExpression{
					{
						Expression: "alter",
						EtymologyBreakdownLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: string(notebook.QuizTypeEtymologyFreeform), IntervalDays: 7},
						},
						EtymologyAssemblyLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: string(notebook.QuizTypeEtymologyFreeform), IntervalDays: 7},
						},
					},
				}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 1, "one freeform event must not be inserted twice")
						assert.Equal(t, string(notebook.QuizTypeEtymologyFreeform), logs[0].QuizType)
						return nil
					})
			},
			want: &ImportLearningLogsResult{
				LearningNew: 1,
			},
		},
		{
			// A pure etymology_reverse log in the assembly slot is
			// unrelated to freeform and must still get imported —
			// confirming the dedup is scoped to freeform.
			name: "etymology_reverse assembly log is not filtered out by freeform dedup",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "alter", NotebookNotes: []notebook.NotebookNote{{NotebookID: "wpme"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSource.EXPECT().FindByNotebookID("wpme").Return([]notebook.LearningHistoryExpression{
					{
						Expression: "alter",
						EtymologyBreakdownLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: string(notebook.QuizTypeEtymologyFreeform), IntervalDays: 7},
						},
						EtymologyAssemblyLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, ResponseTimeMs: 1500, QuizType: string(notebook.QuizTypeEtymologyFreeform), IntervalDays: 7},
							{Status: "misunderstood", LearnedAt: notebook.NewDate(baseTime.Add(24 * time.Hour)), Quality: 1, ResponseTimeMs: 3000, QuizType: string(notebook.QuizTypeEtymologyReverse), IntervalDays: 1},
						},
					},
				}, nil)
				learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
						require.Len(t, logs, 2, "freeform deduped, pure reverse kept")
						types := []string{logs[0].QuizType, logs[1].QuizType}
						assert.Contains(t, types, string(notebook.QuizTypeEtymologyFreeform))
						assert.Contains(t, types, string(notebook.QuizTypeEtymologyReverse))
						return nil
					})
			},
			want: &ImportLearningLogsResult{
				LearningNew: 2,
			},
		},
		{
			name: "duplicate reverse log is skipped",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{
					{NoteID: 1, QuizType: "reverse", LearnedAt: baseTime, SourceNotebookID: "test-story", Status: "understood"},
				}, nil)
				learningSource.EXPECT().FindByNotebookID("test-story").Return([]notebook.LearningHistoryExpression{
					{
						Expression:            "break the ice",
						ReverseEasinessFactor: 2.3,
						ReverseLogs: []notebook.LearningRecord{
							{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 3, ResponseTimeMs: 2000, IntervalDays: 5},
						},
					},
				}, nil)
			},
			want: &ImportLearningLogsResult{
				LearningSkipped: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteSource := mock_datasync.NewMockNoteSource(ctrl)
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			learningSource := mock_datasync.NewMockLearningSource(ctrl)
			dictSource := mock_datasync.NewMockDictionarySource(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)

			tt.setup(learningSource, noteRepo, learningRepo)

			var buf bytes.Buffer
			imp := NewImporter(noteRepo, learningRepo, noteSource, learningSource, dictSource, dictRepo, &buf)

			got, err := imp.ImportLearningLogs(context.Background(), tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestImporter_ImportDictionary(t *testing.T) {
	tests := []struct {
		name    string
		opts    ImportOptions
		setup   func(dictSource *mock_datasync.MockDictionarySource, dictRepo *mock_dictionary.MockDictionaryRepository)
		want    *ImportDictionaryResult
		wantErr bool
	}{
		{
			name: "new dictionary entry is created",
			opts: ImportOptions{},
			setup: func(dictSource *mock_datasync.MockDictionarySource, dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictSource.EXPECT().ReadAll().Return([]rapidapi.Response{
					{Word: "resilient", Results: []rapidapi.Result{{Definition: "able to recover"}}},
				}, nil)
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
			want: &ImportDictionaryResult{
				DictionaryNew: 1,
			},
		},
		{
			name: "existing entry is skipped when UpdateExisting is false",
			opts: ImportOptions{},
			setup: func(dictSource *mock_datasync.MockDictionarySource, dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictSource.EXPECT().ReadAll().Return([]rapidapi.Response{
					{Word: "resilient"},
				}, nil)
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{
					{Word: "resilient"},
				}, nil)
				dictRepo.EXPECT().BatchUpsert(gomock.Any(), gomock.Any()).Return(nil)
			},
			want: &ImportDictionaryResult{
				DictionarySkipped: 1,
			},
		},
		{
			name: "existing entry is updated when UpdateExisting is true",
			opts: ImportOptions{UpdateExisting: true},
			setup: func(dictSource *mock_datasync.MockDictionarySource, dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictSource.EXPECT().ReadAll().Return([]rapidapi.Response{
					{Word: "resilient", Results: []rapidapi.Result{{Definition: "updated"}}},
				}, nil)
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{
					{Word: "resilient", Response: json.RawMessage(`{}`)},
				}, nil)
				dictRepo.EXPECT().BatchUpsert(gomock.Any(), gomock.Any()).Return(nil)
			},
			want: &ImportDictionaryResult{
				DictionaryUpdated: 1,
			},
		},
		{
			name: "dry run does not upsert",
			opts: ImportOptions{DryRun: true},
			setup: func(dictSource *mock_datasync.MockDictionarySource, dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictSource.EXPECT().ReadAll().Return([]rapidapi.Response{
					{Word: "resilient"},
				}, nil)
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{}, nil)
			},
			want: &ImportDictionaryResult{
				DictionaryNew: 1,
			},
		},
		{
			name: "DictionarySource ReadAll error",
			opts: ImportOptions{},
			setup: func(dictSource *mock_datasync.MockDictionarySource, dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictSource.EXPECT().ReadAll().Return(nil, fmt.Errorf("read failed"))
			},
			wantErr: true,
		},
		{
			name: "FindAll error",
			opts: ImportOptions{},
			setup: func(dictSource *mock_datasync.MockDictionarySource, dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictSource.EXPECT().ReadAll().Return([]rapidapi.Response{}, nil)
				dictRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "BatchUpsert error",
			opts: ImportOptions{},
			setup: func(dictSource *mock_datasync.MockDictionarySource, dictRepo *mock_dictionary.MockDictionaryRepository) {
				dictSource.EXPECT().ReadAll().Return([]rapidapi.Response{
					{Word: "resilient"},
				}, nil)
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{}, nil)
				dictRepo.EXPECT().BatchUpsert(gomock.Any(), gomock.Any()).Return(fmt.Errorf("insert failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteSource := mock_datasync.NewMockNoteSource(ctrl)
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			learningSource := mock_datasync.NewMockLearningSource(ctrl)
			dictSource := mock_datasync.NewMockDictionarySource(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)

			tt.setup(dictSource, dictRepo)

			var buf bytes.Buffer
			imp := NewImporter(noteRepo, learningRepo, noteSource, learningSource, dictSource, dictRepo, &buf)

			got, err := imp.ImportDictionary(context.Background(), tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExporter_ExportNotes(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(noteRepo *mock_notebook.MockNoteRepository, noteSink *mock_datasync.MockNoteSink)
		want    *ExportNotesResult
		wantErr bool
	}{
		{
			name: "successful export",
			setup: func(noteRepo *mock_notebook.MockNoteRepository, noteSink *mock_datasync.MockNoteSink) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation", Meaning: "to initiate social interaction"},
					{ID: 2, Usage: "lose one's temper", Entry: "lose one's temper", Meaning: "to become angry"},
				}, nil)
				noteSink.EXPECT().WriteAll(gomock.Any()).
					DoAndReturn(func(notes []notebook.NoteRecord) error {
						require.Len(t, notes, 2)
						assert.Equal(t, "break the ice", notes[0].Usage)
						assert.Equal(t, "lose one's temper", notes[1].Usage)
						return nil
					})
			},
			want: &ExportNotesResult{
				NotesExported: 2,
			},
		},
		{
			name: "empty notes",
			setup: func(noteRepo *mock_notebook.MockNoteRepository, noteSink *mock_datasync.MockNoteSink) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteSink.EXPECT().WriteAll(gomock.Any()).Return(nil)
			},
			want: &ExportNotesResult{},
		},
		{
			name: "FindAll error propagates",
			setup: func(noteRepo *mock_notebook.MockNoteRepository, noteSink *mock_datasync.MockNoteSink) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "WriteAll error propagates",
			setup: func(noteRepo *mock_notebook.MockNoteRepository, noteSink *mock_datasync.MockNoteSink) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation"},
				}, nil)
				noteSink.EXPECT().WriteAll(gomock.Any()).Return(fmt.Errorf("write failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)
			noteSink := mock_datasync.NewMockNoteSink(ctrl)
			learningSink := mock_datasync.NewMockLearningSink(ctrl)
			dictSink := mock_datasync.NewMockDictionarySink(ctrl)

			tt.setup(noteRepo, noteSink)

			var buf bytes.Buffer
			exp := NewExporter(noteRepo, learningRepo, dictRepo, noteSink, learningSink, dictSink, &buf)

			got, err := exp.ExportNotes(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExporter_ExportLearningLogs(t *testing.T) {
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository, learningSink *mock_datasync.MockLearningSink)
		want    *ExportLearningLogsResult
		wantErr bool
	}{
		{
			name: "successful export",
			setup: func(noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository, learningSink *mock_datasync.MockLearningSink) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice"},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{
					{ID: 1, NoteID: 1, Status: "understood", LearnedAt: baseTime, Quality: 4, QuizType: "notebook"},
					{ID: 2, NoteID: 1, Status: "understood", LearnedAt: baseTime.Add(24 * time.Hour), Quality: 5, QuizType: "reverse"},
				}, nil)
				learningSink.EXPECT().WriteAll(gomock.Any(), gomock.Any()).
					DoAndReturn(func(notes []notebook.NoteRecord, logs []learning.LearningLog) error {
						require.Len(t, notes, 1)
						require.Len(t, logs, 2)
						return nil
					})
			},
			want: &ExportLearningLogsResult{LogsExported: 2},
		},
		{
			name: "empty logs",
			setup: func(noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository, learningSink *mock_datasync.MockLearningSink) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSink.EXPECT().WriteAll(gomock.Any(), gomock.Any()).Return(nil)
			},
			want: &ExportLearningLogsResult{},
		},
		{
			name: "noteRepo.FindAll error propagates",
			setup: func(noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository, learningSink *mock_datasync.MockLearningSink) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "learningRepo.FindAll error propagates",
			setup: func(noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository, learningSink *mock_datasync.MockLearningSink) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "learningSink.WriteAll error propagates",
			setup: func(noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository, learningSink *mock_datasync.MockLearningSink) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)
				learningSink.EXPECT().WriteAll(gomock.Any(), gomock.Any()).Return(fmt.Errorf("write failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)
			noteSink := mock_datasync.NewMockNoteSink(ctrl)
			learningSink := mock_datasync.NewMockLearningSink(ctrl)
			dictSink := mock_datasync.NewMockDictionarySink(ctrl)

			tt.setup(noteRepo, learningRepo, learningSink)

			var buf bytes.Buffer
			exp := NewExporter(noteRepo, learningRepo, dictRepo, noteSink, learningSink, dictSink, &buf)

			got, err := exp.ExportLearningLogs(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExporter_ExportDictionary(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(dictRepo *mock_dictionary.MockDictionaryRepository, dictSink *mock_datasync.MockDictionarySink)
		want    *ExportDictionaryResult
		wantErr bool
	}{
		{
			name: "successful export",
			setup: func(dictRepo *mock_dictionary.MockDictionaryRepository, dictSink *mock_datasync.MockDictionarySink) {
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{
					{Word: "hello", SourceType: "rapidapi", Response: json.RawMessage(`{"word":"hello"}`)},
					{Word: "world", SourceType: "rapidapi", Response: json.RawMessage(`{"word":"world"}`)},
				}, nil)
				dictSink.EXPECT().WriteAll(gomock.Any()).
					DoAndReturn(func(entries []rapidapi.DictionaryExportEntry) error {
						require.Len(t, entries, 2)
						assert.Equal(t, "hello", entries[0].Word)
						assert.Equal(t, json.RawMessage(`{"word":"hello"}`), entries[0].Response)
						assert.Equal(t, "world", entries[1].Word)
						assert.Equal(t, json.RawMessage(`{"word":"world"}`), entries[1].Response)
						return nil
					})
			},
			want: &ExportDictionaryResult{EntriesExported: 2},
		},
		{
			name: "empty entries",
			setup: func(dictRepo *mock_dictionary.MockDictionaryRepository, dictSink *mock_datasync.MockDictionarySink) {
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{}, nil)
				dictSink.EXPECT().WriteAll(gomock.Any()).Return(nil)
			},
			want: &ExportDictionaryResult{},
		},
		{
			name: "FindAll error propagates",
			setup: func(dictRepo *mock_dictionary.MockDictionaryRepository, dictSink *mock_datasync.MockDictionarySink) {
				dictRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "WriteAll error propagates",
			setup: func(dictRepo *mock_dictionary.MockDictionaryRepository, dictSink *mock_datasync.MockDictionarySink) {
				dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{
					{Word: "hello", SourceType: "rapidapi", Response: json.RawMessage(`{"word":"hello"}`)},
				}, nil)
				dictSink.EXPECT().WriteAll(gomock.Any()).Return(fmt.Errorf("write failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)
			noteSink := mock_datasync.NewMockNoteSink(ctrl)
			learningSink := mock_datasync.NewMockLearningSink(ctrl)
			dictSink := mock_datasync.NewMockDictionarySink(ctrl)

			tt.setup(dictRepo, dictSink)

			var buf bytes.Buffer
			exp := NewExporter(noteRepo, learningRepo, dictRepo, noteSink, learningSink, dictSink, &buf)

			got, err := exp.ExportDictionary(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// fakeDefinitionConceptRepo is an in-memory DefinitionConceptRepository used
// by the datasync tests. It tracks IDs assigned during BatchCreateConcepts
// so the member-side reconcile can find them, mirroring the DB behavior.
type fakeDefinitionConceptRepo struct {
	concepts     []notebook.DefinitionConceptRecord
	members      []notebook.DefinitionConceptMemberRecord
	nextConceptID int64
	nextMemberID  int64
}

func newFakeDefinitionConceptRepo() *fakeDefinitionConceptRepo {
	return &fakeDefinitionConceptRepo{nextConceptID: 1, nextMemberID: 1}
}

func (f *fakeDefinitionConceptRepo) ListDefinitionConceptsByNotebook(_ context.Context, notebookID string) ([]notebook.DefinitionConceptRecord, error) {
	var out []notebook.DefinitionConceptRecord
	for _, c := range f.concepts {
		if c.NotebookID == notebookID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeDefinitionConceptRepo) ListAllDefinitionConcepts(_ context.Context) ([]notebook.DefinitionConceptRecord, error) {
	return append([]notebook.DefinitionConceptRecord(nil), f.concepts...), nil
}

func (f *fakeDefinitionConceptRepo) ListDefinitionConceptMembersByConceptIDs(_ context.Context, ids []int64) ([]notebook.DefinitionConceptMemberRecord, error) {
	idSet := make(map[int64]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var out []notebook.DefinitionConceptMemberRecord
	for _, m := range f.members {
		if idSet[m.ConceptID] {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeDefinitionConceptRepo) BatchCreateConcepts(_ context.Context, records []*notebook.DefinitionConceptRecord) error {
	for _, r := range records {
		r.ID = f.nextConceptID
		f.nextConceptID++
		f.concepts = append(f.concepts, *r)
	}
	return nil
}

func (f *fakeDefinitionConceptRepo) BatchUpdateConcepts(_ context.Context, records []*notebook.DefinitionConceptRecord) error {
	for _, r := range records {
		for i := range f.concepts {
			if f.concepts[i].ID == r.ID {
				f.concepts[i].Meaning = r.Meaning
			}
		}
	}
	return nil
}

func (f *fakeDefinitionConceptRepo) BatchDeleteConcepts(_ context.Context, ids []int64) error {
	idSet := make(map[int64]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var keptConcepts []notebook.DefinitionConceptRecord
	for _, c := range f.concepts {
		if !idSet[c.ID] {
			keptConcepts = append(keptConcepts, c)
		}
	}
	f.concepts = keptConcepts
	// Cascade-delete members of removed concepts to mirror the FK.
	var keptMembers []notebook.DefinitionConceptMemberRecord
	for _, m := range f.members {
		if !idSet[m.ConceptID] {
			keptMembers = append(keptMembers, m)
		}
	}
	f.members = keptMembers
	return nil
}

func (f *fakeDefinitionConceptRepo) BatchCreateMembers(_ context.Context, records []*notebook.DefinitionConceptMemberRecord) error {
	for _, r := range records {
		r.ID = f.nextMemberID
		f.nextMemberID++
		f.members = append(f.members, *r)
	}
	return nil
}

func (f *fakeDefinitionConceptRepo) BatchDeleteMembers(_ context.Context, ids []int64) error {
	idSet := make(map[int64]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var kept []notebook.DefinitionConceptMemberRecord
	for _, m := range f.members {
		if !idSet[m.ID] {
			kept = append(kept, m)
		}
	}
	f.members = kept
	return nil
}

func TestImporter_ImportDefinitionConcepts_NewConceptsAndMembers(t *testing.T) {
	ctrl := gomock.NewController(t)
	src := mock_datasync.NewMockDefinitionConceptSource(ctrl)
	src.EXPECT().FindAll(gomock.Any()).Return([]notebook.DefinitionConceptForImport{
		{
			NotebookID:   "wpme",
			SessionTitle: "Chapter 1",
			Head:         "bright",
			Meaning:      "having or emitting much light",
			Members:      []string{"bright", "brighten", "brightness"},
		},
	}, nil)

	fake := newFakeDefinitionConceptRepo()
	var buf bytes.Buffer
	imp := NewImporter(nil, nil, nil, nil, nil, nil, &buf)
	imp = imp.WithDefinitionConcepts(fake, src)

	result := &ImportEtymologyResult{}
	require.NoError(t, imp.ImportDefinitionConcepts(context.Background(), ImportOptions{}, result))

	assert.Equal(t, 1, result.DefinitionConceptsNew)
	assert.Equal(t, 3, result.DefinitionConceptMembersNew)
	require.Len(t, fake.concepts, 1)
	assert.Equal(t, "bright", fake.concepts[0].Head)
	assert.Equal(t, "having or emitting much light", fake.concepts[0].Meaning)
	require.Len(t, fake.members, 3)
}

func TestImporter_ImportDefinitionConcepts_ReconcileDropsStale(t *testing.T) {
	ctrl := gomock.NewController(t)
	src := mock_datasync.NewMockDefinitionConceptSource(ctrl)
	src.EXPECT().FindAll(gomock.Any()).Return([]notebook.DefinitionConceptForImport{
		{NotebookID: "book-a", SessionTitle: "S1", Head: "alpha", Meaning: "first", Members: []string{"alpha", "alphas"}},
	}, nil)

	fake := newFakeDefinitionConceptRepo()
	// Pre-populate: one stale concept (gamma) and one surviving concept
	// (alpha) where one of its members ("alphae") will be removed.
	fake.concepts = []notebook.DefinitionConceptRecord{
		{ID: 1, NotebookID: "book-a", Head: "alpha", Meaning: "first"},
		{ID: 2, NotebookID: "book-a", Head: "gamma", Meaning: "third"},
	}
	fake.nextConceptID = 3
	fake.members = []notebook.DefinitionConceptMemberRecord{
		{ID: 1, ConceptID: 1, Expression: "alpha"},
		{ID: 2, ConceptID: 1, Expression: "alphae"}, // stale
		{ID: 3, ConceptID: 2, Expression: "gamma"},  // belongs to deleted concept; cascades
	}
	fake.nextMemberID = 4

	var buf bytes.Buffer
	imp := NewImporter(nil, nil, nil, nil, nil, nil, &buf)
	imp = imp.WithDefinitionConcepts(fake, src)

	result := &ImportEtymologyResult{}
	require.NoError(t, imp.ImportDefinitionConcepts(context.Background(), ImportOptions{}, result))

	assert.Equal(t, 0, result.DefinitionConceptsNew)
	assert.Equal(t, 1, result.DefinitionConceptsDeleted, "gamma is no longer claimed")
	assert.Equal(t, 1, result.DefinitionConceptMembersDeleted, "alphae is no longer claimed on alpha")
	assert.Equal(t, 1, result.DefinitionConceptMembersNew, "alphas is newly claimed on alpha")
	// One concept survives, one was cascade-deleted.
	require.Len(t, fake.concepts, 1)
	assert.Equal(t, "alpha", fake.concepts[0].Head)
}

func TestImporter_ImportDefinitionConcepts_NoRepoIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	imp := NewImporter(nil, nil, nil, nil, nil, nil, &buf)
	result := &ImportEtymologyResult{}
	require.NoError(t, imp.ImportDefinitionConcepts(context.Background(), ImportOptions{}, result))
	assert.Equal(t, 0, result.DefinitionConceptsNew)
}

func TestExporter_ExportDefinitionConcepts(t *testing.T) {
	ctrl := gomock.NewController(t)
	sink := mock_datasync.NewMockDefinitionsBookSink(ctrl)

	fake := newFakeDefinitionConceptRepo()
	fake.concepts = []notebook.DefinitionConceptRecord{
		{ID: 1, NotebookID: "book-a", Head: "bright", Meaning: "having much light"},
	}
	fake.members = []notebook.DefinitionConceptMemberRecord{
		{ID: 1, ConceptID: 1, Expression: "bright", SessionTitle: "Chapter 1"},
		{ID: 2, ConceptID: 1, Expression: "brighten", SessionTitle: "Chapter 1"},
	}

	sink.EXPECT().WriteConcepts(gomock.Any()).DoAndReturn(
		func(perBook map[string]map[string][]notebook.DefinitionConcept) error {
			require.Contains(t, perBook, "book-a")
			require.Contains(t, perBook["book-a"], "Chapter 1")
			require.Len(t, perBook["book-a"]["Chapter 1"], 1)
			c := perBook["book-a"]["Chapter 1"][0]
			assert.Equal(t, "bright", c.Head)
			assert.Equal(t, "having much light", c.Meaning)
			assert.Equal(t, []string{"bright", "brighten"}, c.Expressions)
			return nil
		})

	var buf bytes.Buffer
	exp := NewExporter(nil, nil, nil, nil, nil, nil, &buf)
	exp = exp.WithDefinitionConcepts(fake, sink)

	got, err := exp.ExportDefinitionConcepts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, &ExportDefinitionConceptsResult{ConceptsExported: 1}, got)
}

func TestExporter_ExportDefinitionConcepts_NoSinkIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	exp := NewExporter(nil, nil, nil, nil, nil, nil, &buf)
	got, err := exp.ExportDefinitionConcepts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, &ExportDefinitionConceptsResult{}, got)
}
