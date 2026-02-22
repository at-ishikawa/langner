package notebook

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBNoteRepository_FindAll(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantLen   int
		wantErr   bool
	}{
		{
			name: "returns all notes with relations",
			setupMock: func(mock sqlmock.Sqlmock) {
				noteRows := sqlmock.NewRows([]string{
					"id", "usage", "entry", "meaning", "level", "dictionary_number", "created_at", "updated_at",
				}).
					AddRow(1, "idiom", "break the ice", "to initiate conversation", "B2", 1, now, now).
					AddRow(2, "phrasal_verb", "give up", "to stop trying", "A2", 2, now, now)
				mock.ExpectQuery("SELECT \\* FROM notes ORDER BY id").WillReturnRows(noteRows)

				imageRows := sqlmock.NewRows([]string{
					"id", "note_id", "url", "sort_order", "created_at", "updated_at",
				}).
					AddRow(1, 1, "https://example.com/img1.png", 0, now, now)
				mock.ExpectQuery("SELECT \\* FROM note_images WHERE note_id IN \\(\\?,\\s*\\?\\) ORDER BY sort_order").
					WithArgs(int64(1), int64(2)).
					WillReturnRows(imageRows)

				refRows := sqlmock.NewRows([]string{
					"id", "note_id", "link", "description", "sort_order", "created_at", "updated_at",
				})
				mock.ExpectQuery("SELECT \\* FROM note_references WHERE note_id IN \\(\\?,\\s*\\?\\) ORDER BY sort_order").
					WithArgs(int64(1), int64(2)).
					WillReturnRows(refRows)

				nnRows := sqlmock.NewRows([]string{
					"id", "note_id", "notebook_type", "notebook_id", "group", "subgroup", "created_at", "updated_at",
				}).
					AddRow(1, 1, "story", "book-1", "chapter-1", "", now, now)
				mock.ExpectQuery("SELECT \\* FROM notebook_notes WHERE note_id IN \\(\\?,\\s*\\?\\) ORDER BY id").
					WithArgs(int64(1), int64(2)).
					WillReturnRows(nnRows)
			},
			wantLen: 2,
		},
		{
			name: "select notes db error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM notes ORDER BY id").
					WillReturnError(fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "load images db error",
			setupMock: func(mock sqlmock.Sqlmock) {
				noteRows := sqlmock.NewRows([]string{
					"id", "usage", "entry", "meaning", "level", "dictionary_number", "created_at", "updated_at",
				}).
					AddRow(1, "idiom", "break the ice", "to initiate conversation", "B2", 1, now, now)
				mock.ExpectQuery("SELECT \\* FROM notes ORDER BY id").WillReturnRows(noteRows)

				mock.ExpectQuery("SELECT \\* FROM note_images WHERE note_id IN \\(\\?\\) ORDER BY sort_order").
					WithArgs(int64(1)).
					WillReturnError(fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "load references db error",
			setupMock: func(mock sqlmock.Sqlmock) {
				noteRows := sqlmock.NewRows([]string{
					"id", "usage", "entry", "meaning", "level", "dictionary_number", "created_at", "updated_at",
				}).
					AddRow(1, "idiom", "break the ice", "to initiate conversation", "B2", 1, now, now)
				mock.ExpectQuery("SELECT \\* FROM notes ORDER BY id").WillReturnRows(noteRows)

				mock.ExpectQuery("SELECT \\* FROM note_images WHERE note_id IN \\(\\?\\) ORDER BY sort_order").
					WithArgs(int64(1)).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "note_id", "url", "sort_order", "created_at", "updated_at",
					}))

				mock.ExpectQuery("SELECT \\* FROM note_references WHERE note_id IN \\(\\?\\) ORDER BY sort_order").
					WithArgs(int64(1)).
					WillReturnError(fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "load notebook_notes db error",
			setupMock: func(mock sqlmock.Sqlmock) {
				noteRows := sqlmock.NewRows([]string{
					"id", "usage", "entry", "meaning", "level", "dictionary_number", "created_at", "updated_at",
				}).
					AddRow(1, "idiom", "break the ice", "to initiate conversation", "B2", 1, now, now)
				mock.ExpectQuery("SELECT \\* FROM notes ORDER BY id").WillReturnRows(noteRows)

				mock.ExpectQuery("SELECT \\* FROM note_images WHERE note_id IN \\(\\?\\) ORDER BY sort_order").
					WithArgs(int64(1)).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "note_id", "url", "sort_order", "created_at", "updated_at",
					}))

				mock.ExpectQuery("SELECT \\* FROM note_references WHERE note_id IN \\(\\?\\) ORDER BY sort_order").
					WithArgs(int64(1)).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "note_id", "link", "description", "sort_order", "created_at", "updated_at",
					}))

				mock.ExpectQuery("SELECT \\* FROM notebook_notes WHERE note_id IN \\(\\?\\) ORDER BY id").
					WithArgs(int64(1)).
					WillReturnError(fmt.Errorf("connection refused"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "mysql")
			repo := NewDBNoteRepository(sqlxDB)
			tt.setupMock(mock)

			got, err := repo.FindAll(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, got, tt.wantLen)

			assert.Equal(t, int64(1), got[0].ID)
			assert.Equal(t, "idiom", got[0].Usage)
			assert.Equal(t, "break the ice", got[0].Entry)
			require.Len(t, got[0].Images, 1)
			assert.Equal(t, "https://example.com/img1.png", got[0].Images[0].URL)
			require.Len(t, got[0].NotebookNotes, 1)
			assert.Equal(t, "story", got[0].NotebookNotes[0].NotebookType)

			assert.Equal(t, int64(2), got[1].ID)
			assert.Equal(t, "give up", got[1].Entry)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBNoteRepository_BatchCreate(t *testing.T) {
	tests := []struct {
		name      string
		notes     []*NoteRecord
		setupMock func(mock sqlmock.Sqlmock)
		wantIDs   []int64
		wantErr   bool
	}{
		{
			name: "creates multiple notes with relations",
			notes: []*NoteRecord{
				{
					Usage:            "idiom",
					Entry:            "break the ice",
					Meaning:          "to initiate conversation",
					Level:            "B2",
					DictionaryNumber: 1,
					Images: []NoteImage{
						{URL: "https://example.com/img.png", SortOrder: 0},
					},
					References: []NoteReference{
						{Link: "https://example.com/ref", Description: "reference", SortOrder: 0},
					},
					NotebookNotes: []NotebookNote{
						{NotebookType: "story", NotebookID: "book-1", Group: "ch1", Subgroup: "scene1"},
					},
				},
				{
					Usage:            "phrasal_verb",
					Entry:            "give up",
					Meaning:          "to stop trying",
					Level:            "A2",
					DictionaryNumber: 2,
					NotebookNotes: []NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-1", Group: "unit1"},
					},
				},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO notes \\(`usage`, entry, meaning, level, dictionary_number\\) VALUES \\(\\?, \\?, \\?, \\?, \\?\\), \\(\\?, \\?, \\?, \\?, \\?\\)").
					WithArgs("idiom", "break the ice", "to initiate conversation", "B2", 1,
						"phrasal_verb", "give up", "to stop trying", "A2", 2).
					WillReturnResult(sqlmock.NewResult(10, 2))
				mock.ExpectExec("INSERT INTO note_images \\(note_id, url, sort_order\\) VALUES \\(\\?, \\?, \\?\\)").
					WithArgs(int64(10), "https://example.com/img.png", 0).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO note_references \\(note_id, link, description, sort_order\\) VALUES \\(\\?, \\?, \\?, \\?\\)").
					WithArgs(int64(10), "https://example.com/ref", "reference", 0).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO notebook_notes \\(note_id, notebook_type, notebook_id, `group`, subgroup\\) VALUES \\(\\?, \\?, \\?, \\?, \\?\\), \\(\\?, \\?, \\?, \\?, \\?\\)").
					WithArgs(int64(10), "story", "book-1", "ch1", "scene1",
						int64(11), "flashcard", "vocab-1", "unit1", "").
					WillReturnResult(sqlmock.NewResult(1, 2))
				mock.ExpectCommit()
			},
			wantIDs: []int64{10, 11},
		},
		{
			name:  "empty slice returns nil",
			notes: []*NoteRecord{},
			setupMock: func(mock sqlmock.Sqlmock) {
				// No expectations
			},
		},
		{
			name: "insert notes db error",
			notes: []*NoteRecord{
				{Usage: "idiom", Entry: "break the ice", Meaning: "to initiate conversation", Level: "B2", DictionaryNumber: 1},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO notes").
					WillReturnError(fmt.Errorf("duplicate entry"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name: "insert images db error",
			notes: []*NoteRecord{
				{
					Usage: "idiom", Entry: "break the ice", Meaning: "to initiate conversation", Level: "B2", DictionaryNumber: 1,
					Images: []NoteImage{{URL: "https://example.com/img.png", SortOrder: 0}},
				},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO notes").
					WillReturnResult(sqlmock.NewResult(10, 1))
				mock.ExpectExec("INSERT INTO note_images").
					WillReturnError(fmt.Errorf("connection refused"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name: "insert references db error",
			notes: []*NoteRecord{
				{
					Usage: "idiom", Entry: "break the ice", Meaning: "to initiate conversation", Level: "B2", DictionaryNumber: 1,
					References: []NoteReference{{Link: "https://example.com/ref", Description: "reference", SortOrder: 0}},
				},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO notes").
					WillReturnResult(sqlmock.NewResult(10, 1))
				mock.ExpectExec("INSERT INTO note_references").
					WillReturnError(fmt.Errorf("connection refused"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name: "insert notebook_notes db error",
			notes: []*NoteRecord{
				{
					Usage: "idiom", Entry: "break the ice", Meaning: "to initiate conversation", Level: "B2", DictionaryNumber: 1,
					NotebookNotes: []NotebookNote{{NotebookType: "story", NotebookID: "book-1", Group: "ch1"}},
				},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO notes").
					WillReturnResult(sqlmock.NewResult(10, 1))
				mock.ExpectExec("INSERT INTO notebook_notes").
					WillReturnError(fmt.Errorf("connection refused"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name: "commit error",
			notes: []*NoteRecord{
				{Usage: "idiom", Entry: "break the ice", Meaning: "to initiate conversation", Level: "B2", DictionaryNumber: 1},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO notes").
					WillReturnResult(sqlmock.NewResult(10, 1))
				mock.ExpectCommit().WillReturnError(fmt.Errorf("commit failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "mysql")
			repo := NewDBNoteRepository(sqlxDB)
			tt.setupMock(mock)

			err = repo.BatchCreate(context.Background(), tt.notes)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			for i, wantID := range tt.wantIDs {
				assert.Equal(t, wantID, tt.notes[i].ID)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBNoteRepository_BatchUpdate(t *testing.T) {
	tests := []struct {
		name             string
		notes            []*NoteRecord
		newNotebookNotes []NotebookNote
		setupMock        func(mock sqlmock.Sqlmock)
		wantErr          bool
	}{
		{
			name: "updates multiple notes with new notebook_notes",
			notes: []*NoteRecord{
				{
					ID: 1, Usage: "idiom", Entry: "break the ice", Meaning: "updated meaning", Level: "B2", DictionaryNumber: 1,
				},
				{
					ID: 2, Usage: "phrasal_verb", Entry: "give up", Meaning: "to stop trying", Level: "A2", DictionaryNumber: 2,
				},
			},
			newNotebookNotes: []NotebookNote{
				{NoteID: 1, NotebookType: "story", NotebookID: "book-2", Group: "ch1"},
				{NoteID: 2, NotebookType: "flashcard", NotebookID: "vocab-1", Group: "unit1"},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("UPDATE notes SET").
					WithArgs("idiom", "break the ice", "updated meaning", "B2", 1, int64(1)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE notes SET").
					WithArgs("phrasal_verb", "give up", "to stop trying", "A2", 2, int64(2)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("INSERT INTO notebook_notes \\(note_id, notebook_type, notebook_id, `group`, subgroup\\) VALUES \\(\\?, \\?, \\?, \\?, \\?\\), \\(\\?, \\?, \\?, \\?, \\?\\)").
					WithArgs(int64(1), "story", "book-2", "ch1", "",
						int64(2), "flashcard", "vocab-1", "unit1", "").
					WillReturnResult(sqlmock.NewResult(1, 2))
				mock.ExpectCommit()
			},
		},
		{
			name: "empty slices returns nil",
			setupMock: func(mock sqlmock.Sqlmock) {
				// No expectations
			},
		},
		{
			name: "updates notes without new notebook_notes",
			notes: []*NoteRecord{
				{ID: 1, Usage: "idiom", Entry: "break the ice", Meaning: "updated", Level: "B2", DictionaryNumber: 1},
				{ID: 2, Usage: "phrasal_verb", Entry: "give up", Meaning: "to stop", Level: "A2", DictionaryNumber: 2},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("UPDATE notes SET").
					WithArgs("idiom", "break the ice", "updated", "B2", 1, int64(1)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE notes SET").
					WithArgs("phrasal_verb", "give up", "to stop", "A2", 2, int64(2)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			},
		},
		{
			name: "inserts only notebook_notes without note updates",
			newNotebookNotes: []NotebookNote{
				{NoteID: 1, NotebookType: "story", NotebookID: "book-2", Group: "ch1"},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO notebook_notes \\(note_id, notebook_type, notebook_id, `group`, subgroup\\) VALUES \\(\\?, \\?, \\?, \\?, \\?\\)").
					WithArgs(int64(1), "story", "book-2", "ch1", "").
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
		},
		{
			name: "update db error",
			notes: []*NoteRecord{
				{ID: 1, Usage: "idiom", Entry: "break the ice", Meaning: "updated", Level: "B2", DictionaryNumber: 1},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("UPDATE notes SET").
					WillReturnError(fmt.Errorf("connection refused"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name: "insert notebook_notes db error",
			newNotebookNotes: []NotebookNote{
				{NoteID: 1, NotebookType: "story", NotebookID: "book-2", Group: "ch1"},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO notebook_notes").
					WillReturnError(fmt.Errorf("connection refused"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "mysql")
			repo := NewDBNoteRepository(sqlxDB)
			tt.setupMock(mock)

			err = repo.BatchUpdate(context.Background(), tt.notes, tt.newNotebookNotes)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
