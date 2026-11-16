package learning

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// intervalSeries identifies one learning-log series: one user's logs for one
// note / origin / grammar correction in one log slot (notebook.LogSlot).
type intervalSeries struct {
	userID       int64
	noteID       int64
	originID     int64
	correctionID int64
	slot         notebook.QuizType
}

// RepairLatestIntervals replays every learning-log series with calculator and
// rewrites the NEWEST row's interval_days wherever it differs from the replay.
// Only the newest row decides when a word is due next, so older rows are left
// alone. The newest row is recomputed exactly as the quiz computes it when it
// writes an attempt: calculator.NextIntervalForWrite over the series' earlier
// logs. It repairs rows an earlier version scheduled wrongly (a Mark-as-Correct
// that kept the wrong answer's interval, an accepted synonym scheduled as a
// lapse). Running it again changes nothing. With apply false it only counts the
// rows it would rewrite. Returns how many rows it rewrote (or would rewrite).
func RepairLatestIntervals(ctx context.Context, db *sqlx.DB, calculator notebook.IntervalCalculator, apply bool) (int, error) {
	var logs []LearningLog
	if err := db.SelectContext(ctx, &logs, selectScopedLearningLogColumns+" ORDER BY id"); err != nil {
		return 0, fmt.Errorf("load learning logs: %w", err)
	}

	bySeries := make(map[intervalSeries][]LearningLog)
	for _, l := range logs {
		key := intervalSeries{
			userID:       l.UserID,
			noteID:       l.NoteID,
			originID:     l.OriginID,
			correctionID: l.CorrectionID,
			slot:         notebook.LogSlot(notebook.QuizType(l.QuizType)),
		}
		bySeries[key] = append(bySeries[key], l)
	}

	type fix struct {
		id           int64
		intervalDays int
	}
	var fixes []fix
	for _, series := range bySeries {
		newest := 0
		for i, l := range series {
			if l.LearnedAt.After(series[newest].LearnedAt) ||
				(l.LearnedAt.Equal(series[newest].LearnedAt) && l.ID > series[newest].ID) {
				newest = i
			}
		}
		prior := make([]notebook.LearningRecord, 0, len(series)-1)
		for i, l := range series {
			if i != newest {
				prior = append(prior, toLearningRecord(l))
			}
		}
		tentative := toLearningRecord(series[newest])
		tentative.IntervalDays = 0
		want, _ := calculator.NextIntervalForWrite(prior, tentative)
		if want != series[newest].IntervalDays {
			fixes = append(fixes, fix{id: series[newest].ID, intervalDays: want})
		}
	}
	if len(fixes) == 0 || !apply {
		return len(fixes), nil
	}

	err := database.RunInTx(ctx, db, func(ctx context.Context, tx *sqlx.Tx) error {
		for _, f := range fixes {
			if _, err := tx.ExecContext(ctx,
				`UPDATE learning_logs SET interval_days = $1 WHERE id = $2`, f.intervalDays, f.id); err != nil {
				return fmt.Errorf("update interval of learning log %d: %w", f.id, err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(fixes), nil
}
