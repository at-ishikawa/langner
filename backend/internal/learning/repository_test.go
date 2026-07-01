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
					"id", "note_id", "status", "learned_at", "quality", "response_time_ms", "quiz_type", "interval_days", "source_notebook_id", "created_at", "updated_at",
				}).
					AddRow(1, 10, "understood", now, 4, 1500, "notebook", 7, "", now, now).
					AddRow(2, 11, "misunderstood", now, 1, 3000, "freeform", 1, "", now, now)
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

			sqlxDB := sqlx.NewDb(db, "pgx")
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

func TestDBLearningRepository_Create(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		log       *LearningLog
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   bool
	}{
		{
			name: "inserts a single learning log",
			log:  &LearningLog{NoteID: 10, Status: "understood", LearnedAt: now, Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7, SourceNotebookID: "nb-1"},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO learning_logs").
					WithArgs(int64(10), "understood", now, 4, 1500, "notebook", 7, "nb-1", "").
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
		},
		{
			name: "db error propagates",
			log:  &LearningLog{NoteID: 10, Status: "understood", LearnedAt: now, Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7, SourceNotebookID: "nb-1"},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO learning_logs").
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

			sqlxDB := sqlx.NewDb(db, "pgx")
			repo := NewDBLearningRepository(sqlxDB)
			tt.setupMock(mock)

			err = repo.Create(context.Background(), tt.log)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

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
				{NoteID: 10, Status: "understood", LearnedAt: now, Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7, SourceNotebookID: "nb-1"},
				{NoteID: 11, Status: "misunderstood", LearnedAt: now, Quality: 1, ResponseTimeMs: 3000, QuizType: "freeform", IntervalDays: 1, SourceNotebookID: "nb-2"},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO learning_logs \\(note_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id, concept_key\\) VALUES \\(\\$1, \\$2, \\$3, \\$4, \\$5, \\$6, \\$7, \\$8, \\$9\\), \\(\\$10, \\$11, \\$12, \\$13, \\$14, \\$15, \\$16, \\$17, \\$18\\)").
					WithArgs(
						int64(10), "understood", now, 4, 1500, "notebook", 7, "nb-1", "",
						int64(11), "misunderstood", now, 1, 3000, "freeform", 1, "nb-2", "",
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
				{NoteID: 10, Status: "understood", LearnedAt: now, Quality: 4, ResponseTimeMs: 1500, QuizType: "notebook", IntervalDays: 7, SourceNotebookID: "nb-1"},
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

			sqlxDB := sqlx.NewDb(db, "pgx")
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

// TestDBLearningRepository_UpdateLog_MarkCorrect locks in the DB-side
// shape of OverrideAnswer: SELECT the matching row by (note_id,
// quiz_type, learned_at), UPDATE status/quality/interval, hand back
// the pre-update values for the frontend Undo flow.
func TestDBLearningRepository_UpdateLog_MarkCorrect(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	sqlxDB := sqlx.NewDb(db, "pgx")
	repo := NewDBLearningRepository(sqlxDB)

	learnedAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT id, status, quality, interval_days, learned_at FROM learning_logs`).
		WithArgs(int64(42), "notebook", "2026-06-29").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "quality", "interval_days", "learned_at"}).
			AddRow(int64(7), "misunderstood", 1, 1, learnedAt))
	mock.ExpectExec(`UPDATE learning_logs SET status = \$1, quality = \$2, interval_days = \$3 WHERE id = \$4`).
		WithArgs("understood", 4, 1, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	markCorrect := true
	res, err := repo.UpdateLog(context.Background(), UpdateLogInput{
		NoteID:      42,
		QuizType:    "notebook",
		LearnedAt:   learnedAt,
		MarkCorrect: &markCorrect,
	})
	require.NoError(t, err)
	assert.True(t, res.Found)
	assert.Equal(t, "misunderstood", res.OriginalStatus, "OriginalStatus surfaces the pre-override value for Undo")
	assert.Equal(t, 1, res.OriginalQuality)
	assert.Equal(t, "understood", res.NewStatus)
	assert.Equal(t, 4, res.NewQuality)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestDBLearningRepository_UpdateLog_Mirror covers the path
// MultiLearningRepository takes after the primary (YAML) has computed
// the new status/quality/interval — the secondary store applies the
// values verbatim, no markCorrect derivation.
func TestDBLearningRepository_UpdateLog_Mirror(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	sqlxDB := sqlx.NewDb(db, "pgx")
	repo := NewDBLearningRepository(sqlxDB)

	learnedAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT id, status, quality, interval_days, learned_at FROM learning_logs`).
		WithArgs(int64(42), "notebook", "2026-06-29").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "quality", "interval_days", "learned_at"}).
			AddRow(int64(7), "misunderstood", 1, 1, learnedAt))
	mock.ExpectExec(`UPDATE learning_logs SET status = \$1, quality = \$2, interval_days = \$3 WHERE id = \$4`).
		WithArgs("understood", 4, 3, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	res, err := repo.UpdateLog(context.Background(), UpdateLogInput{
		NoteID:    42,
		QuizType:  "notebook",
		LearnedAt: learnedAt,
		MirrorValues: &UpdateLogMirror{
			Status:       "understood",
			Quality:      4,
			IntervalDays: 3,
		},
	})
	require.NoError(t, err)
	assert.True(t, res.Found)
	assert.Equal(t, 3, res.NewIntervalDays, "MirrorValues overrides any markCorrect-derived interval")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestDBLearningRepository_UpdateLog_NoMatch confirms the soft-no-op
// when no row matches: Found=false, no error. This is what
// MultiLearningRepository relies on so a YAML-only entry doesn't
// fail the whole override when the DB side hasn't seen it yet.
func TestDBLearningRepository_UpdateLog_NoMatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	sqlxDB := sqlx.NewDb(db, "pgx")
	repo := NewDBLearningRepository(sqlxDB)

	mock.ExpectQuery(`SELECT id, status, quality, interval_days, learned_at FROM learning_logs`).
		WithArgs(int64(42), "notebook", "2026-06-29").
		WillReturnError(fmt.Errorf("sql: no rows in result set"))

	markCorrect := true
	res, err := repo.UpdateLog(context.Background(), UpdateLogInput{
		NoteID:      42,
		QuizType:    "notebook",
		LearnedAt:   time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC),
		MarkCorrect: &markCorrect,
	})
	require.NoError(t, err, "missing row must be a soft no-op, not an error")
	assert.False(t, res.Found)
	assert.NoError(t, mock.ExpectationsWereMet())
}
