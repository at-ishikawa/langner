package dictionary

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBDictionaryRepository_FindAll(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "mysql")
	repo := NewDBDictionaryRepository(sqlxDB)
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"word", "source_type", "source_url", "response", "created_at", "updated_at",
	}).
		AddRow("hello", "rapidapi", "https://api.example.com", json.RawMessage(`{"word":"hello"}`), now, now).
		AddRow("world", "rapidapi", "https://api.example.com", json.RawMessage(`{"word":"world"}`), now, now)

	mock.ExpectQuery("SELECT \\* FROM dictionary_entries ORDER BY word").WillReturnRows(rows)

	got, err := repo.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "hello", got[0].Word)
	assert.Equal(t, "rapidapi", got[0].SourceType)
	assert.Equal(t, json.RawMessage(`{"word":"hello"}`), got[0].Response)

	assert.Equal(t, "world", got[1].Word)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBDictionaryRepository_FindByWord(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		word      string
		setupMock func(mock sqlmock.Sqlmock)
		want      *DictionaryEntry
		wantErr   bool
	}{
		{
			name: "found",
			word: "hello",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"word", "source_type", "source_url", "response", "created_at", "updated_at",
				}).AddRow("hello", "rapidapi", "https://api.example.com", json.RawMessage(`{"word":"hello"}`), now, now)

				mock.ExpectQuery("SELECT \\* FROM dictionary_entries WHERE word = \\?").
					WithArgs("hello").
					WillReturnRows(rows)
			},
			want: &DictionaryEntry{
				Word:       "hello",
				SourceType: "rapidapi",
				SourceURL:  "https://api.example.com",
				Response:   json.RawMessage(`{"word":"hello"}`),
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		},
		{
			name: "not found",
			word: "nonexistent",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM dictionary_entries WHERE word = \\?").
					WithArgs("nonexistent").
					WillReturnRows(sqlmock.NewRows([]string{
						"word", "source_type", "source_url", "response", "created_at", "updated_at",
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
			repo := NewDBDictionaryRepository(sqlxDB)
			tt.setupMock(mock)

			got, err := repo.FindByWord(context.Background(), tt.word)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.want.Word, got.Word)
				assert.Equal(t, tt.want.SourceType, got.SourceType)
				assert.Equal(t, tt.want.Response, got.Response)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBDictionaryRepository_Upsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "mysql")
	repo := NewDBDictionaryRepository(sqlxDB)
	ctx := context.Background()

	entry := &DictionaryEntry{
		Word:       "hello",
		SourceType: "rapidapi",
		SourceURL:  "https://api.example.com",
		Response:   json.RawMessage(`{"word":"hello","definitions":["greeting"]}`),
	}

	mock.ExpectExec("INSERT INTO dictionary_entries").
		WithArgs("hello", "rapidapi", "https://api.example.com", json.RawMessage(`{"word":"hello","definitions":["greeting"]}`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.Upsert(ctx, entry)
	require.NoError(t, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}
