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
						assert.Equal(t, 2.5, logs[0].EasinessFactor)
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
					{NoteID: 1, QuizType: "notebook", LearnedAt: baseTime},
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
						assert.Equal(t, 2.3, logs[0].EasinessFactor)
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
			name: "duplicate reverse log is skipped",
			setup: func(learningSource *mock_datasync.MockLearningSource, noteRepo *mock_notebook.MockNoteRepository, learningRepo *mock_learning.MockLearningRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Entry: "break the ice", NotebookNotes: []notebook.NotebookNote{{NotebookID: "test-story"}}},
				}, nil)
				learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{
					{NoteID: 1, QuizType: "reverse", LearnedAt: baseTime},
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

func TestRoundTrip_ImportExport_Notes(t *testing.T) {
	tests := []struct {
		name        string
		sourceNotes []notebook.NoteRecord
	}{
		{
			name: "story notes round-trip",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:            "break the ice",
					Entry:            "start a conversation",
					Meaning:          "to initiate social interaction",
					Level:            "intermediate",
					DictionaryNumber: 1,
					Images:           []notebook.NoteImage{{URL: "https://example.com/ice.png", SortOrder: 0}},
					References:       []notebook.NoteReference{{Link: "https://example.com/ref", Description: "idiom reference", SortOrder: 0}},
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
		},
		{
			name: "flashcard notes round-trip",
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
		},
		{
			name: "multiple notes across notebooks round-trip",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "call it a day",
					Entry:   "stop working",
					Meaning: "to decide to stop working",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 2", Subgroup: "Ending"},
					},
				},
				{
					Usage:   "hit the nail on the head",
					Entry:   "hit the nail on the head",
					Meaning: "to be exactly right",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Accuracy Idioms"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// --- Import phase ---
			noteSource := mock_datasync.NewMockNoteSource(ctrl)
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)

			noteSource.EXPECT().FindAll(gomock.Any()).Return(tt.sourceNotes, nil)
			noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)

			// Capture what import writes to the DB
			var storedNotes []notebook.NoteRecord
			noteRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, notes []*notebook.NoteRecord) error {
					for i, n := range notes {
						n.ID = int64(i + 1)
						for j := range n.NotebookNotes {
							n.NotebookNotes[j].NoteID = n.ID
						}
						for j := range n.Images {
							n.Images[j].NoteID = n.ID
						}
						for j := range n.References {
							n.References[j].NoteID = n.ID
						}
						storedNotes = append(storedNotes, *n)
					}
					return nil
				})

			var buf bytes.Buffer
			importer := NewImporter(noteRepo, learningRepo, noteSource, nil, nil, dictRepo, &buf)
			_, err := importer.ImportNotes(context.Background(), ImportOptions{})
			require.NoError(t, err)
			require.Len(t, storedNotes, len(tt.sourceNotes))

			// --- Export phase ---
			noteRepo2 := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo2 := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo2 := mock_dictionary.NewMockDictionaryRepository(ctrl)
			noteSink := mock_datasync.NewMockNoteSink(ctrl)
			learningSink := mock_datasync.NewMockLearningSink(ctrl)
			dictSink := mock_datasync.NewMockDictionarySink(ctrl)

			noteRepo2.EXPECT().FindAll(gomock.Any()).Return(storedNotes, nil)

			var exportedNotes []notebook.NoteRecord
			noteSink.EXPECT().WriteAll(gomock.Any()).
				DoAndReturn(func(notes []notebook.NoteRecord) error {
					exportedNotes = notes
					return nil
				})

			var buf2 bytes.Buffer
			exporter := NewExporter(noteRepo2, learningRepo2, dictRepo2, noteSink, learningSink, dictSink, &buf2)
			_, err = exporter.ExportNotes(context.Background())
			require.NoError(t, err)

			// --- Validate round-trip ---
			require.Len(t, exportedNotes, len(tt.sourceNotes))
			for i, src := range tt.sourceNotes {
				got := exportedNotes[i]
				assert.Equal(t, src.Usage, got.Usage, "Usage mismatch at index %d", i)
				assert.Equal(t, src.Entry, got.Entry, "Entry mismatch at index %d", i)
				assert.Equal(t, src.Meaning, got.Meaning, "Meaning mismatch at index %d", i)
				assert.Equal(t, src.Level, got.Level, "Level mismatch at index %d", i)
				assert.Equal(t, src.DictionaryNumber, got.DictionaryNumber, "DictionaryNumber mismatch at index %d", i)

				require.Len(t, got.NotebookNotes, len(src.NotebookNotes), "NotebookNotes count mismatch at index %d", i)
				for j, srcNN := range src.NotebookNotes {
					gotNN := got.NotebookNotes[j]
					assert.Equal(t, srcNN.NotebookType, gotNN.NotebookType)
					assert.Equal(t, srcNN.NotebookID, gotNN.NotebookID)
					assert.Equal(t, srcNN.Group, gotNN.Group)
					assert.Equal(t, srcNN.Subgroup, gotNN.Subgroup)
				}

				require.Len(t, got.Images, len(src.Images), "Images count mismatch at index %d", i)
				for j, srcImg := range src.Images {
					assert.Equal(t, srcImg.URL, got.Images[j].URL)
					assert.Equal(t, srcImg.SortOrder, got.Images[j].SortOrder)
				}

				require.Len(t, got.References, len(src.References), "References count mismatch at index %d", i)
				for j, srcRef := range src.References {
					assert.Equal(t, srcRef.Link, got.References[j].Link)
					assert.Equal(t, srcRef.Description, got.References[j].Description)
					assert.Equal(t, srcRef.SortOrder, got.References[j].SortOrder)
				}
			}
		})
	}
}

func TestRoundTrip_ImportExport_LearningLogs(t *testing.T) {
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		existingNotes []notebook.NoteRecord
		sourceExprs   []notebook.LearningHistoryExpression
		wantLogCount  int
		wantNoteIDs   []int64
		wantQuizTypes []string
		wantStatuses  []string
		wantQualities []int
	}{
		{
			name: "notebook learning logs round-trip",
			existingNotes: []notebook.NoteRecord{
				{
					ID: 1, Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NoteID: 1, NotebookType: "story", NotebookID: "test-story", Group: "Episode 1"},
					},
				},
			},
			sourceExprs: []notebook.LearningHistoryExpression{
				{
					Expression:     "break the ice",
					EasinessFactor: 2.5,
					LearnedLogs: []notebook.LearningRecord{
						{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, QuizType: "notebook", IntervalDays: 7},
					},
				},
			},
			wantLogCount:  1,
			wantNoteIDs:   []int64{1},
			wantQuizTypes: []string{"notebook"},
			wantStatuses:  []string{"understood"},
			wantQualities: []int{4},
		},
		{
			name: "notebook and reverse logs round-trip",
			existingNotes: []notebook.NoteRecord{
				{
					ID: 1, Usage: "call it a day", Entry: "call it a day",
					NotebookNotes: []notebook.NotebookNote{
						{NoteID: 1, NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Work Idioms"},
					},
				},
			},
			sourceExprs: []notebook.LearningHistoryExpression{
				{
					Expression:            "call it a day",
					EasinessFactor:        2.6,
					ReverseEasinessFactor: 2.4,
					LearnedLogs: []notebook.LearningRecord{
						{Status: "understood", LearnedAt: notebook.NewDate(baseTime), Quality: 4, QuizType: "notebook", IntervalDays: 7},
					},
					ReverseLogs: []notebook.LearningRecord{
						{Status: "misunderstood", LearnedAt: notebook.NewDate(baseTime.Add(24 * time.Hour)), Quality: 2, IntervalDays: 1},
					},
				},
			},
			wantLogCount:  2,
			wantNoteIDs:   []int64{1, 1},
			wantQuizTypes: []string{"notebook", "reverse"},
			wantStatuses:  []string{"understood", "misunderstood"},
			wantQualities: []int{4, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// --- Import phase ---
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			learningSource := mock_datasync.NewMockLearningSource(ctrl)

			noteRepo.EXPECT().FindAll(gomock.Any()).Return(tt.existingNotes, nil)
			learningRepo.EXPECT().FindAll(gomock.Any()).Return([]learning.LearningLog{}, nil)

			// Return expressions for each notebook
			notebookIDs := make(map[string]bool)
			for _, note := range tt.existingNotes {
				for _, nn := range note.NotebookNotes {
					notebookIDs[nn.NotebookID] = true
				}
			}
			for nbID := range notebookIDs {
				learningSource.EXPECT().FindByNotebookID(nbID).Return(tt.sourceExprs, nil)
			}

			// Capture stored logs
			var storedLogs []learning.LearningLog
			learningRepo.EXPECT().BatchCreate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, logs []*learning.LearningLog) error {
					for i, l := range logs {
						l.ID = int64(i + 1)
						storedLogs = append(storedLogs, *l)
					}
					return nil
				})

			var buf bytes.Buffer
			importer := NewImporter(noteRepo, learningRepo, nil, learningSource, nil, nil, &buf)
			_, err := importer.ImportLearningLogs(context.Background(), ImportOptions{})
			require.NoError(t, err)
			require.Len(t, storedLogs, tt.wantLogCount)

			// --- Export phase ---
			noteRepo2 := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo2 := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo2 := mock_dictionary.NewMockDictionaryRepository(ctrl)
			noteSink := mock_datasync.NewMockNoteSink(ctrl)
			learningSink := mock_datasync.NewMockLearningSink(ctrl)
			dictSink := mock_datasync.NewMockDictionarySink(ctrl)

			noteRepo2.EXPECT().FindAll(gomock.Any()).Return(tt.existingNotes, nil)
			learningRepo2.EXPECT().FindAll(gomock.Any()).Return(storedLogs, nil)

			var exportedNotes []notebook.NoteRecord
			var exportedLogs []learning.LearningLog
			learningSink.EXPECT().WriteAll(gomock.Any(), gomock.Any()).
				DoAndReturn(func(notes []notebook.NoteRecord, logs []learning.LearningLog) error {
					exportedNotes = notes
					exportedLogs = logs
					return nil
				})

			var buf2 bytes.Buffer
			exporter := NewExporter(noteRepo2, learningRepo2, dictRepo2, noteSink, learningSink, dictSink, &buf2)
			_, err = exporter.ExportLearningLogs(context.Background())
			require.NoError(t, err)

			// --- Validate round-trip ---
			require.Len(t, exportedNotes, len(tt.existingNotes))
			require.Len(t, exportedLogs, tt.wantLogCount)

			for i, log := range exportedLogs {
				assert.Equal(t, tt.wantNoteIDs[i], log.NoteID, "NoteID mismatch at index %d", i)
				assert.Equal(t, tt.wantQuizTypes[i], log.QuizType, "QuizType mismatch at index %d", i)
				assert.Equal(t, tt.wantStatuses[i], log.Status, "Status mismatch at index %d", i)
				assert.Equal(t, tt.wantQualities[i], log.Quality, "Quality mismatch at index %d", i)
			}
		})
	}
}

func TestRoundTrip_ImportExport_Dictionary(t *testing.T) {
	tests := []struct {
		name            string
		sourceResponses []rapidapi.Response
	}{
		{
			name: "single dictionary entry round-trip",
			sourceResponses: []rapidapi.Response{
				{Word: "hello"},
			},
		},
		{
			name: "multiple dictionary entries round-trip",
			sourceResponses: []rapidapi.Response{
				{Word: "hello"},
				{Word: "world"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// --- Import phase ---
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo := mock_dictionary.NewMockDictionaryRepository(ctrl)
			dictSource := mock_datasync.NewMockDictionarySource(ctrl)

			dictSource.EXPECT().ReadAll().Return(tt.sourceResponses, nil)
			dictRepo.EXPECT().FindAll(gomock.Any()).Return([]dictionary.DictionaryEntry{}, nil)

			// Capture stored entries
			var storedEntries []dictionary.DictionaryEntry
			dictRepo.EXPECT().BatchUpsert(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, entries []*dictionary.DictionaryEntry) error {
					for _, e := range entries {
						storedEntries = append(storedEntries, *e)
					}
					return nil
				})

			var buf bytes.Buffer
			importer := NewImporter(noteRepo, learningRepo, nil, nil, dictSource, dictRepo, &buf)
			_, err := importer.ImportDictionary(context.Background(), ImportOptions{})
			require.NoError(t, err)
			require.Len(t, storedEntries, len(tt.sourceResponses))

			// --- Export phase ---
			noteRepo2 := mock_notebook.NewMockNoteRepository(ctrl)
			learningRepo2 := mock_learning.NewMockLearningRepository(ctrl)
			dictRepo2 := mock_dictionary.NewMockDictionaryRepository(ctrl)
			noteSink := mock_datasync.NewMockNoteSink(ctrl)
			learningSink := mock_datasync.NewMockLearningSink(ctrl)
			dictSink := mock_datasync.NewMockDictionarySink(ctrl)

			dictRepo2.EXPECT().FindAll(gomock.Any()).Return(storedEntries, nil)

			var exportedEntries []rapidapi.DictionaryExportEntry
			dictSink.EXPECT().WriteAll(gomock.Any()).
				DoAndReturn(func(entries []rapidapi.DictionaryExportEntry) error {
					exportedEntries = entries
					return nil
				})

			var buf2 bytes.Buffer
			exporter := NewExporter(noteRepo2, learningRepo2, dictRepo2, noteSink, learningSink, dictSink, &buf2)
			_, err = exporter.ExportDictionary(context.Background())
			require.NoError(t, err)

			// --- Validate round-trip ---
			require.Len(t, exportedEntries, len(tt.sourceResponses))
			for i, src := range tt.sourceResponses {
				got := exportedEntries[i]
				assert.Equal(t, src.Word, got.Word, "Word mismatch at index %d", i)

				// Verify the JSON response can be unmarshaled back to the original
				var roundTripped rapidapi.Response
				err := json.Unmarshal(got.Response, &roundTripped)
				require.NoError(t, err, "failed to unmarshal exported response at index %d", i)
				assert.Equal(t, src.Word, roundTripped.Word, "round-tripped Word mismatch at index %d", i)
			}
		})
	}
}
