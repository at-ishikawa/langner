package learning

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validLog returns a structurally valid learning log (note-targeted, correct
// answer, computed interval) that callers mutate one field of to test a single
// rule in isolation.
func validLog() *LearningLog {
	return &LearningLog{
		NoteID:       42,
		Status:       "understood",
		QuizType:     "notebook",
		Quality:      4,
		IntervalDays: 7,
		LearnedAt:    time.Date(2026, 8, 29, 9, 39, 0, 0, time.UTC),
	}
}

// TestValidateLearningLog pins every write-boundary invariant. Each rule is
// exercised with a violating log (rejected, clear field-naming error) and the
// valid shapes that MUST pass (no false positives) — the valid cases mirror the
// shapes the owner's real DB actually contains (verified: 3 statuses, 5
// quiz_types, quality 1..5, exactly-one target, misunderstood may be interval 0,
// NULL easiness_factor).
func TestValidateLearningLog(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*LearningLog)
		wantErr     bool
		errContains string
	}{
		// interval_days rule.
		{name: "understood interval 1 passes", mutate: func(l *LearningLog) { l.IntervalDays = 1 }},
		{name: "understood interval 365 passes", mutate: func(l *LearningLog) { l.IntervalDays = 365 }},
		{name: "usable interval 7 passes", mutate: func(l *LearningLog) { l.Status = "usable" }},
		{name: "understood interval 0 rejected", mutate: func(l *LearningLog) { l.IntervalDays = 0 }, wantErr: true, errContains: "interval was not computed"},
		{name: "usable interval 0 rejected", mutate: func(l *LearningLog) { l.Status = "usable"; l.IntervalDays = 0 }, wantErr: true, errContains: "interval was not computed"},
		{name: "intuitive interval 0 rejected", mutate: func(l *LearningLog) { l.Status = "intuitive"; l.IntervalDays = 0 }, wantErr: true, errContains: "interval was not computed"},
		{name: "negative interval rejected", mutate: func(l *LearningLog) { l.IntervalDays = -1 }, wantErr: true, errContains: "negative"},
		// No false positives: not-yet-learned attempts may carry interval 0.
		{name: "misunderstood interval 0 passes", mutate: func(l *LearningLog) { l.Status = "misunderstood"; l.Quality = 1; l.IntervalDays = 0 }},
		{name: "learning (empty status) interval 0 passes", mutate: func(l *LearningLog) { l.Status = ""; l.Quality = 0; l.IntervalDays = 0 }},

		// exactly-one-target rule.
		{name: "origin-targeted log passes", mutate: func(l *LearningLog) { l.NoteID = 0; l.OriginID = 9; l.QuizType = "etymology_origin" }},
		{name: "correction-targeted grammar log passes", mutate: func(l *LearningLog) { l.NoteID = 0; l.CorrectionID = 5; l.QuizType = "grammar" }},
		{name: "no target rejected", mutate: func(l *LearningLog) { l.NoteID = 0 }, wantErr: true, errContains: "exactly one"},
		{name: "two targets rejected", mutate: func(l *LearningLog) { l.OriginID = 9 }, wantErr: true, errContains: "exactly one"},

		// status / quiz_type enum rules.
		{name: "unknown status rejected", mutate: func(l *LearningLog) { l.Status = "bogus" }, wantErr: true, errContains: "unknown status"},
		{name: "unknown quiz_type rejected", mutate: func(l *LearningLog) { l.QuizType = "etymology_breakdown" }, wantErr: true, errContains: "unknown quiz_type"},
		{name: "reverse quiz_type passes", mutate: func(l *LearningLog) { l.QuizType = "reverse" }},
		{name: "freeform quiz_type passes", mutate: func(l *LearningLog) { l.QuizType = "freeform" }},
		// Real data legitimately pairs etymology_origin with note_id and
		// notebook with origin_id — the mapping is NOT enforced, so these pass.
		{name: "etymology_origin on note_id passes (no mapping guard)", mutate: func(l *LearningLog) { l.QuizType = "etymology_origin" }},
		{name: "notebook on origin_id passes (no mapping guard)", mutate: func(l *LearningLog) { l.NoteID = 0; l.OriginID = 9 }},

		// quality rule.
		{name: "quality 0 passes", mutate: func(l *LearningLog) { l.Status = "misunderstood"; l.Quality = 0; l.IntervalDays = 0 }},
		{name: "quality 5 passes", mutate: func(l *LearningLog) { l.Quality = 5 }},
		{name: "quality 7 rejected", mutate: func(l *LearningLog) { l.Quality = 7 }, wantErr: true, errContains: "out of range"},
		{name: "quality -1 rejected", mutate: func(l *LearningLog) { l.Quality = -1 }, wantErr: true, errContains: "out of range"},

		// learned_at rule.
		{name: "zero learned_at rejected", mutate: func(l *LearningLog) { l.LearnedAt = time.Time{} }, wantErr: true, errContains: "learned_at is zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := validLog()
			tt.mutate(log)
			// Runtime-write semantics (Create): the success-interval rule applies.
			err := validateLearningLog(log, true)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestValidateLearningLog_BatchImportSkipsIntervalRule proves the import/seed
// boundary (enforceComputedInterval=false) applies every STRUCTURAL rule but
// exempts the success-interval rule — so a legacy success row with an
// omitted/0 interval round-trips through import, while a structurally broken row
// (unknown quiz_type) is still rejected.
func TestValidateLearningLog_BatchImportSkipsIntervalRule(t *testing.T) {
	successZero := validLog()
	successZero.IntervalDays = 0 // understood + 0: rejected at runtime, allowed on import
	assert.Error(t, validateLearningLog(successZero, true), "runtime write must reject uncomputed success interval")
	assert.NoError(t, validateLearningLog(successZero, false), "import must accept a legacy success row with interval 0")

	structurallyBroken := validLog()
	structurallyBroken.QuizType = "etymology_breakdown"
	assert.Error(t, validateLearningLog(structurallyBroken, false), "import must still reject an unknown quiz_type")
}

// TestDBLearningRepository_Create_RejectsUncomputedInterval proves the guard
// fires at the actual write path (Create), not just in isolation: a correct
// attempt whose interval was left 0 never reaches the INSERT, while a valid
// correct log and a valid misunderstood-0 log both insert cleanly.
func TestDBLearningRepository_Create_RejectsUncomputedInterval(t *testing.T) {
	base := time.Date(2026, 8, 29, 9, 39, 0, 0, time.UTC)

	t.Run("understood with interval 0 is rejected before insert", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo := NewDBLearningRepository(sqlx.NewDb(db, "pgx"))

		// NoteID pre-set so Create skips note resolution and reaches the guard.
		// No INSERT is expected — the guard must return before it.
		err = repo.Create(context.Background(), &LearningLog{
			NoteID: 1258, Status: "understood", LearnedAt: base,
			Quality: 4, QuizType: "reverse", IntervalDays: 0,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "interval was not computed")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("understood with computed interval inserts", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo := NewDBLearningRepository(sqlx.NewDb(db, "pgx"))

		mock.ExpectExec("INSERT INTO learning_logs").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = repo.Create(context.Background(), &LearningLog{
			NoteID: 1258, Status: "understood", LearnedAt: base,
			Quality: 4, QuizType: "reverse", IntervalDays: 1,
		})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("misunderstood with interval 0 inserts (no false positive)", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo := NewDBLearningRepository(sqlx.NewDb(db, "pgx"))

		mock.ExpectExec("INSERT INTO learning_logs").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = repo.Create(context.Background(), &LearningLog{
			NoteID: 1258, Status: "misunderstood", LearnedAt: base,
			Quality: 1, QuizType: "reverse", IntervalDays: 0,
		})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
