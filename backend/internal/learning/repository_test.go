package learning

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

func TestDBLearningRepository_FindAll(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantLen   int
		wantErr   bool
	}{
		{
			name: "returns all learning logs",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "note_id", "status", "learned_at", "quality", "response_time_ms", "quiz_type", "interval_days", "easiness_factor", "created_at", "updated_at",
				}).
					AddRow(1, 10, "understood", now, 4, 1500, "notebook", 7, 2.5, now, now).
					AddRow(2, 11, "misunderstood", now, 1, 3000, "freeform", 1, 2.1, now, now)
				mock.ExpectQuery("SELECT \\* FROM learning_logs ORDER BY id").WillReturnRows(rows)
			},
			wantLen: 2,
		},
		{
			name: "db error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM learning_logs ORDER BY id").
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
			repo := NewDBLearningRepository(sqlxDB)
			tt.setupMock(mock)

			got, err := repo.FindAll(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, got, tt.wantLen)

			assert.Equal(t, int64(10), got[0].NoteID)
			assert.Equal(t, "understood", got[0].Status)
			assert.Equal(t, int64(11), got[1].NoteID)
			assert.Equal(t, "misunderstood", got[1].Status)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBLearningRepository_BatchCreate(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		logs      []*LearningLog
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   bool
	}{
		{
			name: "creates multiple logs with multi-row insert",
			logs: []*LearningLog{
				{NoteID: 10, Status: "understood", LearnedAt: now, Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7, EasinessFactor: 2.5},
				{NoteID: 11, Status: "misunderstood", LearnedAt: now, Quality: 1, ResponseTimeMs: 3000, QuizType: "freeform", IntervalDays: 1, EasinessFactor: 2.1},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO learning_logs \\(note_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, easiness_factor\\) VALUES \\(\\?, \\?, \\?, \\?, \\?, \\?, \\?, \\?\\), \\(\\?, \\?, \\?, \\?, \\?, \\?, \\?, \\?\\)").
					WithArgs(
						int64(10), "understood", now, 4, 1500, "notebook", 7, 2.5,
						int64(11), "misunderstood", now, 1, 3000, "freeform", 1, 2.1,
					).
					WillReturnResult(sqlmock.NewResult(1, 2))
				mock.ExpectCommit()
			},
		},
		{
			name:  "empty slice returns nil",
			logs:  []*LearningLog{},
			setupMock: func(mock sqlmock.Sqlmock) {
				// No expectations
			},
		},
		{
			name: "db error propagates",
			logs: []*LearningLog{
				{NoteID: 10, Status: "understood", LearnedAt: now, Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7, EasinessFactor: 2.5},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO learning_logs").
					WillReturnError(fmt.Errorf("duplicate entry"))
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
			repo := NewDBLearningRepository(sqlxDB)
			tt.setupMock(mock)

			err = repo.BatchCreate(context.Background(), tt.logs)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
