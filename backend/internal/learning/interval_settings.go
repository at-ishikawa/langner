package learning

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// Review-schedule algorithms a user can choose.
const (
	AlgorithmFixed       = "fixed"
	AlgorithmModifiedSM2 = "modified_sm2"
)

// Limits of a fixed interval ladder a user can save.
const (
	maxFixedIntervalSteps = 12
	maxFixedIntervalDays  = 3650
)

// ErrInvalidIntervalSettings wraps every validation failure of
// ValidateIntervalSettings, so callers can tell bad input from a storage error.
var ErrInvalidIntervalSettings = errors.New("invalid review schedule")

// ErrIntervalSettingsUnavailable is returned when a user tries to save a
// schedule on a server without a database: there is nowhere per-user to keep it.
var ErrIntervalSettingsUnavailable = errors.New("review schedules can only be saved when a database is configured")

// IntervalSettings is a review schedule: the algorithm and, for "fixed", the
// interval ladder in days.
type IntervalSettings struct {
	Algorithm      string
	FixedIntervals []int
}

// Calculator returns the interval calculator for these settings.
func (s IntervalSettings) Calculator() notebook.IntervalCalculator {
	return notebook.NewIntervalCalculator(s.Algorithm, s.FixedIntervals)
}

// ValidateIntervalSettings checks a schedule a user wants to save and returns
// it normalized (no ladder for modified_sm2).
func ValidateIntervalSettings(s IntervalSettings) (IntervalSettings, error) {
	switch s.Algorithm {
	case AlgorithmModifiedSM2:
		return IntervalSettings{Algorithm: AlgorithmModifiedSM2}, nil
	case AlgorithmFixed:
	default:
		return IntervalSettings{}, fmt.Errorf("%w: unknown algorithm %q", ErrInvalidIntervalSettings, s.Algorithm)
	}
	if len(s.FixedIntervals) == 0 {
		return IntervalSettings{}, fmt.Errorf("%w: enter at least one interval", ErrInvalidIntervalSettings)
	}
	if len(s.FixedIntervals) > maxFixedIntervalSteps {
		return IntervalSettings{}, fmt.Errorf("%w: use at most %d intervals", ErrInvalidIntervalSettings, maxFixedIntervalSteps)
	}
	for i, days := range s.FixedIntervals {
		if days < 1 || days > maxFixedIntervalDays {
			return IntervalSettings{}, fmt.Errorf("%w: each interval must be between 1 and %d days", ErrInvalidIntervalSettings, maxFixedIntervalDays)
		}
		if i > 0 && days <= s.FixedIntervals[i-1] {
			return IntervalSettings{}, fmt.Errorf("%w: each interval must be longer than the one before it", ErrInvalidIntervalSettings)
		}
	}
	return IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: append([]int(nil), s.FixedIntervals...)}, nil
}

// IntervalSchedules decides which review schedule each user's answers are
// scheduled with: the user's saved choice (user_interval_settings), else the
// server default from quiz.algorithm / quiz.fixed_intervals. Every path that
// computes an interval for a user goes through CalculatorFor, so the rule
// lives here only. It keeps no per-user state in memory: each lookup is one
// primary-key read, so a change is seen at once by every server instance.
type IntervalSchedules struct {
	// db is nil without a database: everyone gets the default.
	db         *sqlx.DB
	defaultSet IntervalSettings
}

// NewIntervalSchedules returns the schedules for quizCfg's default. db may be
// nil (YAML-only / CLI), in which case every user gets the default.
func NewIntervalSchedules(db *sqlx.DB, quizCfg config.QuizConfig) *IntervalSchedules {
	def := IntervalSettings{Algorithm: AlgorithmModifiedSM2}
	if quizCfg.Algorithm == AlgorithmFixed {
		def = IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: quizCfg.FixedIntervals}
		if len(def.FixedIntervals) == 0 {
			def.FixedIntervals = notebook.DefaultFixedIntervals
		}
	}
	return &IntervalSchedules{db: db, defaultSet: def}
}

// Default returns the server default schedule.
func (s *IntervalSchedules) Default() IntervalSettings {
	return s.defaultSet
}

// DefaultCalculator returns the calculator of the server default schedule.
func (s *IntervalSchedules) DefaultCalculator() notebook.IntervalCalculator {
	return s.defaultSet.Calculator()
}

// Get returns userID's schedule and whether it is the server default (the
// user saved none).
func (s *IntervalSchedules) Get(ctx context.Context, userID int64) (settings IntervalSettings, isDefault bool, err error) {
	if s.db == nil || userID == 0 {
		return s.defaultSet, true, nil
	}
	var row struct {
		Algorithm      string `db:"algorithm"`
		FixedIntervals string `db:"fixed_intervals"`
	}
	err = s.db.GetContext(ctx, &row,
		`SELECT algorithm, array_to_json(fixed_intervals)::text AS fixed_intervals FROM user_interval_settings WHERE user_id = $1`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return s.defaultSet, true, nil
	}
	if err != nil {
		return IntervalSettings{}, false, fmt.Errorf("load review schedule of user %d: %w", userID, err)
	}
	settings.Algorithm = row.Algorithm
	if err := json.Unmarshal([]byte(row.FixedIntervals), &settings.FixedIntervals); err != nil {
		return IntervalSettings{}, false, fmt.Errorf("parse review schedule of user %d: %w", userID, err)
	}
	return settings, false, nil
}

// CalculatorFor returns the interval calculator for userID's answers. A
// calculator installed on ctx by WithCalculator for the same user is reused,
// so a batch of answers looks the schedule up once.
func (s *IntervalSchedules) CalculatorFor(ctx context.Context, userID int64) (notebook.IntervalCalculator, error) {
	if p, ok := ctx.Value(calculatorKey{}).(preloadedCalculator); ok && p.userID == userID {
		return p.calculator, nil
	}
	settings, _, err := s.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return settings.Calculator(), nil
}

type calculatorKey struct{}

type preloadedCalculator struct {
	userID     int64
	calculator notebook.IntervalCalculator
}

// WithCalculator returns ctx carrying userID's calculator for CalculatorFor
// to reuse within this one request. On a lookup error ctx is returned as is,
// so each CalculatorFor call looks the schedule up (and reports the error)
// itself.
func (s *IntervalSchedules) WithCalculator(ctx context.Context, userID int64) context.Context {
	calc, err := s.CalculatorFor(ctx, userID)
	if err != nil {
		return ctx
	}
	return context.WithValue(ctx, calculatorKey{}, preloadedCalculator{userID: userID, calculator: calc})
}

// Save validates and stores userID's schedule. It changes how future answers
// are scheduled; existing learning logs keep their intervals.
func (s *IntervalSchedules) Save(ctx context.Context, userID int64, settings IntervalSettings) error {
	valid, err := ValidateIntervalSettings(settings)
	if err != nil {
		return err
	}
	if s.db == nil || userID == 0 {
		return ErrIntervalSettingsUnavailable
	}
	days := make([]string, len(valid.FixedIntervals))
	for i, d := range valid.FixedIntervals {
		days[i] = strconv.Itoa(d)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_interval_settings (user_id, algorithm, fixed_intervals)
		VALUES ($1, $2, $3::int[])
		ON CONFLICT (user_id) DO UPDATE SET algorithm = EXCLUDED.algorithm, fixed_intervals = EXCLUDED.fixed_intervals`,
		userID, valid.Algorithm, "{"+strings.Join(days, ",")+"}")
	if err != nil {
		return fmt.Errorf("save review schedule of user %d: %w", userID, err)
	}
	return nil
}

// Reset removes userID's schedule so they follow the server default again.
func (s *IntervalSchedules) Reset(ctx context.Context, userID int64) error {
	if s.db == nil || userID == 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM user_interval_settings WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("reset review schedule of user %d: %w", userID, err)
	}
	return nil
}
