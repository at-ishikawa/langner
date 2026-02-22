package datasync

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mock_notebook "github.com/at-ishikawa/langner/internal/mocks/notebook"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestImporter_ImportNotes(t *testing.T) {
	tests := []struct {
		name        string
		sourceNotes []notebook.NoteRecord
		opts        ImportOptions
		setup       func(noteRepo *mock_notebook.MockNoteRepository)
		want        *ImportResult
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
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
			want: &ImportResult{
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation", NotebookNotes: []notebook.NotebookNote{
						{NoteID: 1, NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
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
			want: &ImportResult{
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
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
			want: &ImportResult{
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
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
			want: &ImportResult{
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				// No BatchCreate or BatchUpdate calls expected
			},
			want: &ImportResult{
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
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
			want: &ImportResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
		{
			name:        "FindAll error",
			sourceNotes: []notebook.NoteRecord{},
			opts:        ImportOptions{},
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
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
			noteRepo := mock_notebook.NewMockNoteRepository(ctrl)

			tt.setup(noteRepo)

			var buf bytes.Buffer
			imp := NewImporter(noteRepo, &buf)

			got, err := imp.ImportNotes(context.Background(), tt.sourceNotes, tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
