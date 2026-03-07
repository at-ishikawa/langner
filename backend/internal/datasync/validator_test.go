package datasync

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mock_datasync "github.com/at-ishikawa/langner/internal/mocks/datasync"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestValidator_ValidateNotes(t *testing.T) {
	tests := []struct {
		name         string
		sourceNotes  []notebook.NoteRecord
		exportNotes  []notebook.NoteRecord
		wantErrCount int
		wantErrors   []ValidationError
	}{
		{
			name: "matching notes pass validation",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage:   "break the ice",
					Entry:   "start a conversation",
					Meaning: "to initiate social interaction",
					Level:   "intermediate",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			exportNotes: []notebook.NoteRecord{
				{
					Usage:   "break the ice",
					Entry:   "start a conversation",
					Meaning: "to initiate social interaction",
					Level:   "intermediate",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "missing note in export",
			sourceNotes: []notebook.NoteRecord{
				{Usage: "break the ice", Entry: "start a conversation", Meaning: "to initiate social interaction"},
			},
			wantErrCount: 1,
			wantErrors: []ValidationError{
				{NoteKey: "break the ice (start a conversation)", Field: "presence", Source: "exists", Export: "missing"},
			},
		},
		{
			name: "extra note in export",
			exportNotes: []notebook.NoteRecord{
				{Usage: "call it a day", Entry: "stop working", Meaning: "to decide to stop working"},
			},
			wantErrCount: 1,
			wantErrors: []ValidationError{
				{NoteKey: "call it a day (stop working)", Field: "presence", Source: "missing", Export: "exists"},
			},
		},
		{
			name: "meaning mismatch",
			sourceNotes: []notebook.NoteRecord{
				{Usage: "break the ice", Entry: "break the ice", Meaning: "to initiate social interaction"},
			},
			exportNotes: []notebook.NoteRecord{
				{Usage: "break the ice", Entry: "break the ice", Meaning: "to crack frozen water"},
			},
			wantErrCount: 1,
			wantErrors: []ValidationError{
				{NoteKey: "break the ice (break the ice)", Field: "meaning", Source: "to initiate social interaction", Export: "to crack frozen water"},
			},
		},
		{
			name: "level mismatch",
			sourceNotes: []notebook.NoteRecord{
				{Usage: "break the ice", Entry: "break the ice", Level: "intermediate"},
			},
			exportNotes: []notebook.NoteRecord{
				{Usage: "break the ice", Entry: "break the ice", Level: "advanced"},
			},
			wantErrCount: 1,
			wantErrors: []ValidationError{
				{NoteKey: "break the ice (break the ice)", Field: "level", Source: "intermediate", Export: "advanced"},
			},
		},
		{
			name: "dictionary number mismatch",
			sourceNotes: []notebook.NoteRecord{
				{Usage: "break the ice", Entry: "break the ice", DictionaryNumber: 1},
			},
			exportNotes: []notebook.NoteRecord{
				{Usage: "break the ice", Entry: "break the ice", DictionaryNumber: 2},
			},
			wantErrCount: 1,
			wantErrors: []ValidationError{
				{NoteKey: "break the ice (break the ice)", Field: "dictionary_number", Source: "1", Export: "2"},
			},
		},
		{
			name: "missing notebook note in export",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
					},
				},
			},
			exportNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
			wantErrCount: 1,
			wantErrors: []ValidationError{
				{NoteKey: "break the ice (break the ice)", Field: "notebook_note", Source: "flashcard/vocab-cards/Common Idioms/", Export: "missing"},
			},
		},
		{
			name: "multiple notes all matching",
			sourceNotes: []notebook.NoteRecord{
				{Usage: "break the ice", Entry: "break the ice", Meaning: "to initiate social interaction"},
				{Usage: "call it a day", Entry: "call it a day", Meaning: "to stop working"},
			},
			exportNotes: []notebook.NoteRecord{
				{Usage: "call it a day", Entry: "call it a day", Meaning: "to stop working"},
				{Usage: "break the ice", Entry: "break the ice", Meaning: "to initiate social interaction"},
			},
			wantErrCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			source := mock_datasync.NewMockNoteSource(ctrl)
			exportedSource := mock_datasync.NewMockNoteSource(ctrl)

			source.EXPECT().FindAll(gomock.Any()).Return(tt.sourceNotes, nil)
			exportedSource.EXPECT().FindAll(gomock.Any()).Return(tt.exportNotes, nil)

			var buf bytes.Buffer
			validator := NewValidator(&buf)
			result, err := validator.ValidateNotes(context.Background(), source, exportedSource)
			require.NoError(t, err)

			assert.Equal(t, len(tt.sourceNotes), result.SourceNoteCount)
			assert.Equal(t, len(tt.exportNotes), result.ExportedNoteCount)
			assert.Len(t, result.Errors, tt.wantErrCount)

			if tt.wantErrors != nil {
				for i, want := range tt.wantErrors {
					assert.Equal(t, want.NoteKey, result.Errors[i].NoteKey, "NoteKey mismatch at index %d", i)
					assert.Equal(t, want.Field, result.Errors[i].Field, "Field mismatch at index %d", i)
					assert.Equal(t, want.Source, result.Errors[i].Source, "Source mismatch at index %d", i)
					assert.Equal(t, want.Export, result.Errors[i].Export, "Export mismatch at index %d", i)
				}
			}
		})
	}
}

func TestValidator_ValidateNotes_SourceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	source := mock_datasync.NewMockNoteSource(ctrl)
	exportedSource := mock_datasync.NewMockNoteSource(ctrl)

	source.EXPECT().FindAll(gomock.Any()).Return(nil, assert.AnError)

	var buf bytes.Buffer
	validator := NewValidator(&buf)
	_, err := validator.ValidateNotes(context.Background(), source, exportedSource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read source notes")
}

func TestValidator_ValidateNotes_ExportError(t *testing.T) {
	ctrl := gomock.NewController(t)
	source := mock_datasync.NewMockNoteSource(ctrl)
	exportedSource := mock_datasync.NewMockNoteSource(ctrl)

	source.EXPECT().FindAll(gomock.Any()).Return([]notebook.NoteRecord{}, nil)
	exportedSource.EXPECT().FindAll(gomock.Any()).Return(nil, assert.AnError)

	var buf bytes.Buffer
	validator := NewValidator(&buf)
	_, err := validator.ValidateNotes(context.Background(), source, exportedSource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read exported notes")
}
