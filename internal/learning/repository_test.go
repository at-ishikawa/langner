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
			name: "returns all logs",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "note_id", "status", "learned_at", "quality", "response_time_ms",
					"quiz_type", "interval_days", "easiness_factor", "created_at", "updated_at",
				}).
					AddRow(1, 10, "understood", now, 4, 1500, "flashcard", 7, 2.5, now, now).
					AddRow(2, 20, "misunderstood", now, 1, 3000, "reverse", 1, 1.3, now, now)
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

			assert.Equal(t, int64(1), got[0].ID)
			assert.Equal(t, int64(10), got[0].NoteID)
			assert.Equal(t, "understood", got[0].Status)
			assert.Equal(t, "flashcard", got[0].QuizType)
			assert.Equal(t, 2.5, got[0].EasinessFactor)

			assert.Equal(t, int64(2), got[1].ID)
			assert.Equal(t, "misunderstood", got[1].Status)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBLearningRepository_FindByNote(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		noteID    int64
		quizType  string
		setupMock func(mock sqlmock.Sqlmock)
		wantLen   int
		wantErr   bool
	}{
		{
			name:     "returns logs for note",
			noteID:   10,
			quizType: "flashcard",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "note_id", "status", "learned_at", "quality", "response_time_ms",
					"quiz_type", "interval_days", "easiness_factor", "created_at", "updated_at",
				}).
					AddRow(1, 10, "understood", now, 4, 1500, "flashcard", 7, 2.5, now, now).
					AddRow(2, 10, "misunderstood", now.Add(24*time.Hour), 2, 2000, "flashcard", 1, 2.0, now, now)
				mock.ExpectQuery("SELECT \\* FROM learning_logs WHERE note_id = \\? AND quiz_type = \\? ORDER BY learned_at").
					WithArgs(int64(10), "flashcard").
					WillReturnRows(rows)
			},
			wantLen: 2,
		},
		{
			name:     "db error",
			noteID:   10,
			quizType: "flashcard",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM learning_logs WHERE note_id = \\? AND quiz_type = \\? ORDER BY learned_at").
					WithArgs(int64(10), "flashcard").
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

			got, err := repo.FindByNote(context.Background(), tt.noteID, tt.quizType)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, got, tt.wantLen)

			assert.Equal(t, int64(10), got[0].NoteID)
			assert.Equal(t, "flashcard", got[0].QuizType)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBLearningRepository_FindLatestByNote(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		noteID    int64
		quizType  string
		setupMock func(mock sqlmock.Sqlmock)
		want      *LearningLog
		wantErr   bool
	}{
		{
			name:     "found",
			noteID:   10,
			quizType: "flashcard",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "note_id", "status", "learned_at", "quality", "response_time_ms",
					"quiz_type", "interval_days", "easiness_factor", "created_at", "updated_at",
				}).AddRow(5, 10, "understood", now, 4, 1200, "flashcard", 7, 2.5, now, now)

				mock.ExpectQuery("SELECT \\* FROM learning_logs WHERE note_id = \\? AND quiz_type = \\? ORDER BY learned_at DESC LIMIT 1").
					WithArgs(int64(10), "flashcard").
					WillReturnRows(rows)
			},
			want: &LearningLog{
				ID:             5,
				NoteID:         10,
				Status:         "understood",
				LearnedAt:      now,
				Quality:        4,
				ResponseTimeMs: 1200,
				QuizType:       "flashcard",
				IntervalDays:   7,
				EasinessFactor: 2.5,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		{
			name:     "not found",
			noteID:   99,
			quizType: "flashcard",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM learning_logs WHERE note_id = \\? AND quiz_type = \\? ORDER BY learned_at DESC LIMIT 1").
					WithArgs(int64(99), "flashcard").
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "note_id", "status", "learned_at", "quality", "response_time_ms",
						"quiz_type", "interval_days", "easiness_factor", "created_at", "updated_at",
					}))
			},
			want: nil,
		},
		{
			name:     "db error",
			noteID:   10,
			quizType: "flashcard",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM learning_logs WHERE note_id = \\? AND quiz_type = \\? ORDER BY learned_at DESC LIMIT 1").
					WithArgs(int64(10), "flashcard").
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

			got, err := repo.FindLatestByNote(context.Background(), tt.noteID, tt.quizType)
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
				assert.Equal(t, tt.want.Status, got.Status)
				assert.Equal(t, tt.want.QuizType, got.QuizType)
				assert.Equal(t, tt.want.EasinessFactor, got.EasinessFactor)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBLearningRepository_FindByNoteQuizTypeAndLearnedAt(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		noteID    int64
		quizType  string
		learnedAt time.Time
		setupMock func(mock sqlmock.Sqlmock)
		want      *LearningLog
		wantErr   bool
	}{
		{
			name:      "found",
			noteID:    10,
			quizType:  "flashcard",
			learnedAt: now,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "note_id", "status", "learned_at", "quality", "response_time_ms",
					"quiz_type", "interval_days", "easiness_factor", "created_at", "updated_at",
				}).AddRow(5, 10, "understood", now, 4, 1200, "flashcard", 7, 2.5, now, now)

				mock.ExpectQuery("SELECT \\* FROM learning_logs WHERE note_id = \\? AND quiz_type = \\? AND learned_at = \\?").
					WithArgs(int64(10), "flashcard", now).
					WillReturnRows(rows)
			},
			want: &LearningLog{
				ID:             5,
				NoteID:         10,
				Status:         "understood",
				LearnedAt:      now,
				Quality:        4,
				ResponseTimeMs: 1200,
				QuizType:       "flashcard",
				IntervalDays:   7,
				EasinessFactor: 2.5,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		{
			name:      "not found",
			noteID:    99,
			quizType:  "flashcard",
			learnedAt: now,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM learning_logs WHERE note_id = \\? AND quiz_type = \\? AND learned_at = \\?").
					WithArgs(int64(99), "flashcard", now).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "note_id", "status", "learned_at", "quality", "response_time_ms",
						"quiz_type", "interval_days", "easiness_factor", "created_at", "updated_at",
					}))
			},
			want: nil,
		},
		{
			name:      "db error",
			noteID:    10,
			quizType:  "flashcard",
			learnedAt: now,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM learning_logs WHERE note_id = \\? AND quiz_type = \\? AND learned_at = \\?").
					WithArgs(int64(10), "flashcard", now).
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

			got, err := repo.FindByNoteQuizTypeAndLearnedAt(context.Background(), tt.noteID, tt.quizType, tt.learnedAt)
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
				assert.Equal(t, tt.want.Status, got.Status)
				assert.Equal(t, tt.want.QuizType, got.QuizType)
				assert.Equal(t, tt.want.LearnedAt, got.LearnedAt)
				assert.Equal(t, tt.want.EasinessFactor, got.EasinessFactor)
			}

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
		wantID    int64
		wantErr   bool
	}{
		{
			name: "inserts a log",
			log: &LearningLog{
				NoteID:         10,
				Status:         "understood",
				LearnedAt:      now,
				Quality:        4,
				ResponseTimeMs: 1500,
				QuizType:       "flashcard",
				IntervalDays:   7,
				EasinessFactor: 2.5,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO learning_logs").
					WithArgs(int64(10), "understood", now, 4, 1500, "flashcard", 7, 2.5).
					WillReturnResult(sqlmock.NewResult(42, 1))
			},
			wantID: 42,
		},
		{
			name: "db error",
			log: &LearningLog{
				NoteID:         10,
				Status:         "understood",
				LearnedAt:      now,
				Quality:        4,
				ResponseTimeMs: 1500,
				QuizType:       "flashcard",
				IntervalDays:   7,
				EasinessFactor: 2.5,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO learning_logs").
					WithArgs(int64(10), "understood", now, 4, 1500, "flashcard", 7, 2.5).
					WillReturnError(fmt.Errorf("duplicate entry"))
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

			err = repo.Create(context.Background(), tt.log)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, tt.log.ID)

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
			name:      "empty slice",
			logs:      []*LearningLog{},
			setupMock: func(mock sqlmock.Sqlmock) {},
		},
		{
			name: "inserts records",
			logs: []*LearningLog{
				{
					NoteID:         10,
					Status:         "understood",
					LearnedAt:      now,
					Quality:        4,
					ResponseTimeMs: 1500,
					QuizType:       "flashcard",
					IntervalDays:   7,
					EasinessFactor: 2.5,
				},
				{
					NoteID:         20,
					Status:         "misunderstood",
					LearnedAt:      now,
					Quality:        1,
					ResponseTimeMs: 3000,
					QuizType:       "reverse",
					IntervalDays:   1,
					EasinessFactor: 1.3,
				},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO learning_logs").
					WithArgs(
						int64(10), "understood", now, 4, 1500, "flashcard", 7, 2.5,
						int64(20), "misunderstood", now, 1, 3000, "reverse", 1, 1.3,
					).
					WillReturnResult(sqlmock.NewResult(1, 2))
			},
		},
		{
			name: "db error",
			logs: []*LearningLog{
				{
					NoteID:         10,
					Status:         "understood",
					LearnedAt:      now,
					Quality:        4,
					ResponseTimeMs: 1500,
					QuizType:       "flashcard",
					IntervalDays:   7,
					EasinessFactor: 2.5,
				},
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO learning_logs").
					WithArgs(int64(10), "understood", now, 4, 1500, "flashcard", 7, 2.5).
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
