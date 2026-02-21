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
		name       string
		stories    map[string]notebook.Index
		flashcards map[string]notebook.FlashcardIndex
		opts       ImportOptions
		setup      func(noteRepo *mock_notebook.MockNoteRepository)
		want       *ImportResult
		wantErr    bool
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, n *notebook.NoteRecord) error {
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation", NotebookNotes: []notebook.NotebookNote{
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation"},
				}, nil)
				noteRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, n *notebook.NoteRecord) error {
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, n *notebook.NoteRecord) error {
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, n *notebook.NoteRecord) error {
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
		{
			name: "note with images and references",
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
												Meaning:    "able to recover",
												Images:     []string{"https://example.com/img1.png", "https://example.com/img2.png"},
												References: []notebook.Reference{
													{URL: "https://example.com/ref1", Description: "Reference 1"},
												},
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, n *notebook.NoteRecord) error {
						n.ID = 1
						require.Len(t, n.Images, 2)
						assert.Equal(t, "https://example.com/img1.png", n.Images[0].URL)
						assert.Equal(t, 0, n.Images[0].SortOrder)
						assert.Equal(t, "https://example.com/img2.png", n.Images[1].URL)
						assert.Equal(t, 1, n.Images[1].SortOrder)
						require.Len(t, n.References, 1)
						assert.Equal(t, "https://example.com/ref1", n.References[0].Link)
						assert.Equal(t, "Reference 1", n.References[0].Description)
						assert.Equal(t, 0, n.References[0].SortOrder)
						return nil
					})
			},
			want: &ImportResult{
				NotesNew:    1,
				NotebookNew: 1,
			},
		},
		{
			name:       "FindAll error",
			stories:    map[string]notebook.Index{},
			flashcards: map[string]notebook.FlashcardIndex{},
			opts:       ImportOptions{},
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "Create error propagates",
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
				noteRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(fmt.Errorf("insert failed"))
			},
			wantErr: true,
		},
		{
			name: "Update error propagates",
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
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation"},
				}, nil)
				noteRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fmt.Errorf("update failed"))
			},
			wantErr: true,
		},
		{
			name:    "CreateNotebookNote error propagates",
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
									Expression: "break the ice",
									Definition: "start a conversation",
									Meaning:    "to initiate social interaction",
								},
							},
						},
					},
				},
			},
			opts: ImportOptions{},
			setup: func(noteRepo *mock_notebook.MockNoteRepository) {
				noteRepo.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{
					{ID: 1, Usage: "break the ice", Entry: "start a conversation"},
				}, nil)
				noteRepo.EXPECT().CreateNotebookNote(gomock.Any(), gomock.Any()).Return(fmt.Errorf("insert failed"))
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

			got, err := imp.ImportNotes(context.Background(), tt.stories, tt.flashcards, tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
