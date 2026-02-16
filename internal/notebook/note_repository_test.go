package notebook

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBNoteRepository_FindAll(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "mysql")
	repo := NewDBNoteRepository(sqlxDB)
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

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

	got, err := repo.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)

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
}

func TestDBNoteRepository_FindByUsageAndEntry(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		usage     string
		entry     string
		setupMock func(mock sqlmock.Sqlmock)
		want      *NoteRecord
		wantErr   bool
	}{
		{
			name:  "found",
			usage: "idiom",
			entry: "break the ice",
			setupMock: func(mock sqlmock.Sqlmock) {
				noteRow := sqlmock.NewRows([]string{
					"id", "usage", "entry", "meaning", "level", "dictionary_number", "created_at", "updated_at",
				}).AddRow(1, "idiom", "break the ice", "to initiate conversation", "B2", 1, now, now)
				mock.ExpectQuery("SELECT \\* FROM notes WHERE `usage` = \\? AND entry = \\?").
					WithArgs("idiom", "break the ice").
					WillReturnRows(noteRow)

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
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "note_id", "notebook_type", "notebook_id", "group", "subgroup", "created_at", "updated_at",
					}))
			},
			want: &NoteRecord{
				ID:               1,
				Usage:            "idiom",
				Entry:            "break the ice",
				Meaning:          "to initiate conversation",
				Level:            "B2",
				DictionaryNumber: 1,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
		},
		{
			name:  "not found",
			usage: "idiom",
			entry: "nonexistent",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM notes WHERE `usage` = \\? AND entry = \\?").
					WithArgs("idiom", "nonexistent").
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "usage", "entry", "meaning", "level", "dictionary_number", "created_at", "updated_at",
					}))
			},
			want: nil,
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

			got, err := repo.FindByUsageAndEntry(context.Background(), tt.usage, tt.entry)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.want.ID, got.ID)
				assert.Equal(t, tt.want.Usage, got.Usage)
				assert.Equal(t, tt.want.Entry, got.Entry)
				assert.Equal(t, tt.want.Meaning, got.Meaning)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBNoteRepository_FindByNotebook(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "mysql")
	repo := NewDBNoteRepository(sqlxDB)
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	noteRows := sqlmock.NewRows([]string{
		"id", "usage", "entry", "meaning", "level", "dictionary_number", "created_at", "updated_at",
	}).
		AddRow(1, "idiom", "break the ice", "to initiate conversation", "B2", 1, now, now)

	mock.ExpectQuery("SELECT n\\.\\* FROM notes n").
		WithArgs("story", "book-1").
		WillReturnRows(noteRows)

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
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "note_id", "notebook_type", "notebook_id", "group", "subgroup", "created_at", "updated_at",
		}))

	got, err := repo.FindByNotebook(ctx, "story", "book-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "break the ice", got[0].Entry)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBNoteRepository_Create(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "mysql")
	repo := NewDBNoteRepository(sqlxDB)
	ctx := context.Background()

	note := &NoteRecord{
		Usage:            "idiom",
		Entry:            "lose one's temper",
		Meaning:          "to become angry",
		Level:            "B1",
		DictionaryNumber: 1,
		Images: []NoteImage{
			{URL: "https://example.com/img.png", SortOrder: 0},
		},
		References: []NoteReference{
			{Link: "https://example.com/ref", Description: "reference", SortOrder: 0},
		},
		NotebookNotes: []NotebookNote{
			{NotebookType: "story", NotebookID: "book-1", Group: "ch1", Subgroup: ""},
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO notes").
		WithArgs("idiom", "lose one's temper", "to become angry", "B1", 1).
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectExec("INSERT INTO note_images").
		WithArgs(int64(10), "https://example.com/img.png", 0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO note_references").
		WithArgs(int64(10), "https://example.com/ref", "reference", 0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notebook_notes").
		WithArgs(int64(10), "story", "book-1", "ch1", "").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = repo.Create(ctx, note)
	require.NoError(t, err)
	assert.Equal(t, int64(10), note.ID)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBNoteRepository_FindNotebookNote(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		noteID       int64
		notebookType string
		notebookID   string
		group        string
		setupMock    func(mock sqlmock.Sqlmock)
		want         *NotebookNote
		wantErr      bool
	}{
		{
			name:         "found",
			noteID:       1,
			notebookType: "story",
			notebookID:   "book-1",
			group:        "chapter-1",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "note_id", "notebook_type", "notebook_id", "group", "subgroup", "created_at", "updated_at",
				}).AddRow(10, 1, "story", "book-1", "chapter-1", "", now, now)

				mock.ExpectQuery("SELECT \\* FROM notebook_notes WHERE note_id = \\? AND notebook_type = \\? AND notebook_id = \\? AND `group` = \\?").
					WithArgs(int64(1), "story", "book-1", "chapter-1").
					WillReturnRows(rows)
			},
			want: &NotebookNote{
				ID:           10,
				NoteID:       1,
				NotebookType: "story",
				NotebookID:   "book-1",
				Group:        "chapter-1",
				Subgroup:     "",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		{
			name:         "not found",
			noteID:       99,
			notebookType: "story",
			notebookID:   "book-1",
			group:        "chapter-1",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM notebook_notes WHERE note_id = \\? AND notebook_type = \\? AND notebook_id = \\? AND `group` = \\?").
					WithArgs(int64(99), "story", "book-1", "chapter-1").
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "note_id", "notebook_type", "notebook_id", "group", "subgroup", "created_at", "updated_at",
					}))
			},
			want: nil,
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

			got, err := repo.FindNotebookNote(context.Background(), tt.noteID, tt.notebookType, tt.notebookID, tt.group)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.want.ID, got.ID)
				assert.Equal(t, tt.want.NoteID, got.NoteID)
				assert.Equal(t, tt.want.NotebookType, got.NotebookType)
				assert.Equal(t, tt.want.NotebookID, got.NotebookID)
				assert.Equal(t, tt.want.Group, got.Group)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBNoteRepository_CreateNotebookNote(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "mysql")
	repo := NewDBNoteRepository(sqlxDB)
	ctx := context.Background()

	nn := &NotebookNote{
		NoteID:       1,
		NotebookType: "story",
		NotebookID:   "book-1",
		Group:        "chapter-1",
		Subgroup:     "section-1",
	}

	mock.ExpectExec("INSERT INTO notebook_notes").
		WithArgs(int64(1), "story", "book-1", "chapter-1", "section-1").
		WillReturnResult(sqlmock.NewResult(20, 1))

	err = repo.CreateNotebookNote(ctx, nn)
	require.NoError(t, err)
	assert.Equal(t, int64(20), nn.ID)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBNoteRepository_Update(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "mysql")
	repo := NewDBNoteRepository(sqlxDB)
	ctx := context.Background()

	note := &NoteRecord{
		ID:               1,
		Usage:            "idiom",
		Entry:            "break the ice",
		Meaning:          "to start a conversation",
		Level:            "B2",
		DictionaryNumber: 1,
	}

	mock.ExpectExec("UPDATE notes SET").
		WithArgs("idiom", "break the ice", "to start a conversation", "B2", 1, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Update(ctx, note)
	require.NoError(t, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}
