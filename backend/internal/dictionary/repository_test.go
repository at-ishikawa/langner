package dictionary

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBDictionaryRepository_FindAll(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantLen   int
		wantErr   bool
	}{
		{
			name: "returns all entries",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"word", "source_type", "source_url", "response", "created_at", "updated_at",
				}).
					AddRow("hello", "rapidapi", "https://api.example.com", json.RawMessage(`{"word":"hello"}`), now, now).
					AddRow("world", "rapidapi", "https://api.example.com", json.RawMessage(`{"word":"world"}`), now, now)
				mock.ExpectQuery("SELECT \\* FROM dictionary_entries ORDER BY word").WillReturnRows(rows)
			},
			wantLen: 2,
		},
		{
			name: "db error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM dictionary_entries ORDER BY word").
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
			repo := NewDBDictionaryRepository(sqlxDB)
			tt.setupMock(mock)

			got, err := repo.FindAll(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, got, tt.wantLen)

			assert.Equal(t, "hello", got[0].Word)
			assert.Equal(t, "rapidapi", got[0].SourceType)
			assert.Equal(t, json.RawMessage(`{"word":"hello"}`), got[0].Response)
			assert.Equal(t, "world", got[1].Word)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBDictionaryRepository_BatchUpsert(t *testing.T) {
	tests := []struct {
		name      string
		entries   []*DictionaryEntry
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   bool
	}{
		{
			name:      "empty slice",
			entries:   []*DictionaryEntry{},
			setupMock: func(mock sqlmock.Sqlmock) {},
		},
		{
			name: "upserts records",
			entries: []*DictionaryEntry{
				{
					Word:       "hello",
					SourceType: "rapidapi",
					Response:   json.RawMessage(`{"word":"hello"}`),
				},
				{
					Word:       "world",
					SourceType: "rapidapi",
					Response:   json.RawMessage(`{"word":"world"}`),
				},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO dictionary_entries").
					WithArgs("hello", "rapidapi", json.RawMessage(`{"word":"hello"}`)).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO dictionary_entries").
					WithArgs("world", "rapidapi", json.RawMessage(`{"word":"world"}`)).
					WillReturnResult(sqlmock.NewResult(2, 1))
			},
		},
		{
			name: "db error",
			entries: []*DictionaryEntry{
				{
					Word:       "hello",
					SourceType: "rapidapi",
					Response:   json.RawMessage(`{"word":"hello"}`),
				},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO dictionary_entries").
					WithArgs("hello", "rapidapi", json.RawMessage(`{"word":"hello"}`)).
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
			repo := NewDBDictionaryRepository(sqlxDB)
			tt.setupMock(mock)

			err = repo.BatchUpsert(context.Background(), tt.entries)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
