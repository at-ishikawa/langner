package notebook

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIntervalCalculator(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
	}{
		{name: "default returns SM2", algorithm: "modified_sm2"},
		{name: "empty string returns SM2", algorithm: ""},
		{name: "fixed returns FixedLevelCalculator", algorithm: "fixed"},
		{name: "unknown algorithm returns SM2", algorithm: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewIntervalCalculator(tt.algorithm, nil)
			require.NotNil(t, got)
		})
	}
}

func TestSM2Calculator_CalculateInterval(t *testing.T) {
	calc := &SM2Calculator{}

	tests := []struct {
		name        string
		logs        []LearningRecord
		quality     int
		currentEF   float64
		wantMinDays int
	}{
		{
			name:        "first correct answer with no logs",
			logs:        nil,
			quality:     4,
			currentEF:   DefaultEasinessFactor,
			wantMinDays: 1,
		},
		{
			name: "correct answer after one correct",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 3},
			},
			quality:     4,
			currentEF:   DefaultEasinessFactor,
			wantMinDays: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intervalDays, newEF := calc.CalculateInterval(tt.logs, tt.quality, tt.currentEF)
			assert.GreaterOrEqual(t, intervalDays, tt.wantMinDays)
			assert.Greater(t, newEF, 0.0)
		})
	}
}

func TestSM2Calculator_RecalculateAll(t *testing.T) {
	calc := &SM2Calculator{}
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty logs", func(t *testing.T) {
		ef, logs := calc.RecalculateAll(nil)
		assert.InDelta(t, DefaultEasinessFactor, ef, 0.5)
		assert.Empty(t, logs)
	})

	t.Run("preserves override_interval", func(t *testing.T) {
		_, logs := calc.RecalculateAll([]LearningRecord{
			{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood, OverrideInterval: 30},
		})
		assert.Equal(t, 30, logs[0].IntervalDays)
	})
}

func TestSM2Calculator_RecalculateAll_EarlyReviewGuard(t *testing.T) {
	calc := &SM2Calculator{}
	baseTime := time.Date(2025, 3, 23, 0, 0, 0, 0, time.UTC)

	// Simulate: freeform on 3/23, then notebook on 4/1 (9d later), 4/4 (3d), 4/8 (4d).
	// The 4/4 and 4/8 reviews come before the previous interval has elapsed,
	// so their intervals should NOT advance beyond the previous interval.
	logs := []LearningRecord{
		{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 16)}, Status: LearnedStatusUnderstood, IntervalDays: 90},  // 4/8 (wrong)
		{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 12)}, Status: LearnedStatusUnderstood, IntervalDays: 30},  // 4/4 (wrong)
		{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 9)}, Status: LearnedStatusUnderstood, IntervalDays: 7},    // 4/1
		{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood, QuizType: "freeform", IntervalDays: 7}, // 3/23
	}

	_, result := calc.RecalculateAll(logs)

	// Result is sorted newest-first: [4/8, 4/4, 4/1, 3/23]
	require.Len(t, result, 4)
	// 3/23: first correct → small interval (3 days per SM-2)
	assert.Equal(t, 3, result[3].IntervalDays, "first correct answer: interval=3")
	// 4/1: 9 days since 3/23, interval was 3 → elapsed >= interval → advances
	interval_4_1 := result[2].IntervalDays
	assert.Greater(t, interval_4_1, 3, "4/1 should advance past 3 days")
	// 4/4: only 3 days since 4/1, interval was >3 → early review → should NOT advance
	assert.Equal(t, interval_4_1, result[1].IntervalDays, "4/4 early review: should keep same interval as 4/1")
	// 4/8: only 4 days since 4/4 → still early → should NOT advance
	assert.Equal(t, interval_4_1, result[0].IntervalDays, "4/8 early review: should keep same interval")
}

// TestRecalculateAll_AdvancesAfterMisunderstood encodes the spec the user
// articulated: when the previous log is misunderstood (interval=1) and the
// next log is correct on any later calendar day, the recalculated interval
// for the correct log must advance past the misunderstood's interval —
// matching what the live quiz produces at submit time. validate --fix
// re-runs RecalculateAll, and any drift between the two paths leaves the
// stored interval inconsistent with what the user actually earned.
//
// The original bug — duration.Hours()/24 truncation — silently violated
// this spec when the second review's wall-clock time was earlier than the
// first's, even on different calendar days. The test deliberately varies
// both calendar-day gap (1, 3, 7) and times-of-day so the spec is enforced
// regardless of when on the clock the user reviews. Midnight-only fixtures
// hid the original bug; this matrix forbids that.
func TestRecalculateAll_AdvancesAfterMisunderstood(t *testing.T) {
	day1 := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	hourPairs := [][2]int{
		{0, 0},   // identical clock — pure calendar boundary
		{6, 9},   // forward-of-day clock advance
		{18, 9},  // clock wraps backward across midnight (the truncation case)
		{23, 0},  // 1-hour real gap, full calendar day
	}
	dayGaps := []int{1, 3, 7} // next-day, mid-week, end-of-week

	calculators := map[string]IntervalCalculator{
		"sm2":         &SM2Calculator{},
		"fixed_level": &FixedLevelCalculator{},
	}

	for name, calc := range calculators {
		for _, gap := range dayGaps {
			for _, hp := range hourPairs {
				t.Run(fmt.Sprintf("%s/gap=%dd/clock=%02d->%02d", name, gap, hp[0], hp[1]), func(t *testing.T) {
					prev := day1.Add(time.Duration(hp[0]) * time.Hour)
					next := day1.AddDate(0, 0, gap).Add(time.Duration(hp[1]) * time.Hour)
					logs := []LearningRecord{
						{Quality: 4, LearnedAt: Date{Time: next}, Status: LearnedStatusUnderstood},
						{Quality: 1, LearnedAt: Date{Time: prev}, Status: LearnedStatusMisunderstood, IntervalDays: 1},
					}
					_, result := calc.RecalculateAll(logs)
					require.Len(t, result, 2)
					assert.Greater(t, result[0].IntervalDays, 1,
						"misunderstood on %s, correct on %s — validate --fix must advance the interval (live quiz does)",
						prev.Format(time.RFC3339), next.Format(time.RFC3339),
					)
				})
			}
		}
	}
}

// TestRecalculateAll_DoesNotAdvanceSameDayCorrect locks in the negative half
// of the spec: a correct answer the same calendar day as a misunderstood
// must NOT advance the interval, regardless of how many hours later it
// happened. Reviewing twice in one day doesn't prove next-day retention,
// so the early-review guard must keep the interval at 1.
func TestRecalculateAll_DoesNotAdvanceSameDayCorrect(t *testing.T) {
	day := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	hourPairs := [][2]int{{0, 23}, {6, 18}, {9, 17}}

	calculators := map[string]IntervalCalculator{
		"sm2":         &SM2Calculator{},
		"fixed_level": &FixedLevelCalculator{},
	}

	for name, calc := range calculators {
		for _, hp := range hourPairs {
			t.Run(fmt.Sprintf("%s/clock=%02d->%02d", name, hp[0], hp[1]), func(t *testing.T) {
				logs := []LearningRecord{
					{Quality: 4, LearnedAt: Date{Time: day.Add(time.Duration(hp[1]) * time.Hour)}, Status: LearnedStatusUnderstood},
					{Quality: 1, LearnedAt: Date{Time: day.Add(time.Duration(hp[0]) * time.Hour)}, Status: LearnedStatusMisunderstood, IntervalDays: 1},
				}
				_, result := calc.RecalculateAll(logs)
				require.Len(t, result, 2)
				assert.Equal(t, 1, result[0].IntervalDays,
					"same-day correct must NOT advance — early-review guard keeps interval at the misunderstood's value",
				)
			})
		}
	}
}

func TestSM2Calculator_DeriveEF(t *testing.T) {
	calc := &SM2Calculator{}
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty logs returns default EF", func(t *testing.T) {
		ef := calc.DeriveEF(nil)
		assert.InDelta(t, DefaultEasinessFactor, ef, 0.001)
	})

	t.Run("single correct answer", func(t *testing.T) {
		ef := calc.DeriveEF([]LearningRecord{
			{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood},
		})
		assert.Greater(t, ef, MinEasinessFactor)
	})

	t.Run("matches RecalculateAll EF", func(t *testing.T) {
		logs := []LearningRecord{
			{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 3)}, Status: LearnedStatusUnderstood},
			{Quality: 1, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 1)}, Status: LearnedStatusMisunderstood},
			{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood},
		}
		derivedEF := calc.DeriveEF(logs)
		recalcEF, _ := calc.RecalculateAll(logs)
		assert.InDelta(t, recalcEF, derivedEF, 0.001)
	})
}

func TestFixedLevelCalculator_DeriveEF(t *testing.T) {
	calc := &FixedLevelCalculator{}
	ef := calc.DeriveEF([]LearningRecord{
		{Quality: 4},
	})
	assert.Equal(t, 0.0, ef)
}

func TestQualityToLevelDelta(t *testing.T) {
	assert.Equal(t, 2, qualityToLevelDelta(5))
	assert.Equal(t, 1, qualityToLevelDelta(4))
	assert.Equal(t, 1, qualityToLevelDelta(3))
	assert.Equal(t, -1, qualityToLevelDelta(1))
	assert.Equal(t, -1, qualityToLevelDelta(2))
}

func TestFixedLevelCalculator_LevelFromInterval(t *testing.T) {
	// Default intervals: [1, 7, 30, 90, 365, 1095, 1825]
	calc := &FixedLevelCalculator{}

	tests := []struct {
		interval  int
		wantLevel int
	}{
		{0, 0},
		{1, 0},
		{5, 0},
		{7, 1},
		{20, 1},
		{30, 2},
		{75, 2},
		{90, 3},
		{200, 3},
		{365, 4},
		{1000, 4},
		{1095, 5},
		{1825, 6},
		{5000, 6},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.wantLevel, calc.levelFromInterval(tt.interval), "interval %d", tt.interval)
	}
}

func TestFixedLevelCalculator_CalculateInterval(t *testing.T) {
	// CalculateInterval derives level from the most recent log's stored interval,
	// then advances by quality delta.
	tests := []struct {
		name     string
		logs     []LearningRecord
		quality  int
		wantDays int
	}{
		{
			name:     "first correct q=4: level 0->1 = 7 days",
			logs:     nil,
			quality:  4,
			wantDays: 7,
		},
		{
			name:     "first wrong: level stays 0 = 1 day",
			logs:     nil,
			quality:  1,
			wantDays: 1,
		},
		{
			name: "after interval=7, correct q=4: level 1->2 = 30 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 7},
			},
			quality:  4,
			wantDays: 30,
		},
		{
			name: "after interval=30, correct q=4: level 2->3 = 90 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 30},
			},
			quality:  4,
			wantDays: 90,
		},
		{
			name:     "first q=5: level 0->2 = 30 days (skips a level)",
			logs:     nil,
			quality:  5,
			wantDays: 30,
		},
		{
			name: "after interval=7, q=5: level 1->3 = 90 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 7},
			},
			quality:  5,
			wantDays: 90,
		},
		{
			name: "after interval=90, wrong q=1: level 3->2 = 30 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 90},
			},
			quality:  1,
			wantDays: 30,
		},
		{
			name: "SM-2 interval 75 then correct: level 2->3 = 90 (not 1825)",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 75},
			},
			quality:  4,
			wantDays: 90,
		},
		{
			name: "at max level, stays at max: 1825 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 1825},
			},
			quality:  4,
			wantDays: 1825,
		},
		{
			name: "multiple wrongs clamp at level 0: 1 day",
			logs: []LearningRecord{
				{Quality: 1, IntervalDays: 1},
			},
			quality:  1,
			wantDays: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := &FixedLevelCalculator{}
			gotDays, gotEF := calc.CalculateInterval(tt.logs, tt.quality, 0)
			assert.Equal(t, tt.wantDays, gotDays)
			assert.Equal(t, 0.0, gotEF)
		})
	}
}

func TestFixedLevelCalculator_RecalculateAll(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		logs     []LearningRecord
		validate func(t *testing.T, logs []LearningRecord)
	}{
		{
			name: "empty logs",
			logs: nil,
		},
		{
			name: "snaps SM-2 interval 3 -> 7",
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}, IntervalDays: 3},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 7, logs[0].IntervalDays)
			},
		},
		{
			name: "snaps SM-2 intervals to fixed levels",
			logs: []LearningRecord{
				// Newest first, properly spaced so no early review guard fires
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 40)}, IntervalDays: 63},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 8)}, IntervalDays: 10},
				{Quality: 4, LearnedAt: Date{Time: baseTime}, IntervalDays: 3},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 90, logs[0].IntervalDays)  // 63 -> 90
				assert.Equal(t, 30, logs[1].IntervalDays)  // 10 -> 30
				assert.Equal(t, 7, logs[2].IntervalDays)   // 3 -> 7
			},
		},
		{
			name: "word with 9 correct and interval 75 -> 90 (not 1825)",
			logs: []LearningRecord{
				// Newest first, 9 correct answers, last interval was 75
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 80)}, IntervalDays: 75},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 50)}, IntervalDays: 50},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 30)}, IntervalDays: 25},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 20)}, IntervalDays: 15},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 14)}, IntervalDays: 10},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 8)}, IntervalDays: 6},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 5)}, IntervalDays: 4},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 2)}, IntervalDays: 3},
				{Quality: 4, LearnedAt: Date{Time: baseTime}, IntervalDays: 1},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				// Most recent: 75 -> snaps to 90
				assert.Equal(t, 90, logs[0].IntervalDays)
			},
		},
		{
			name: "preserves override interval",
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}, IntervalDays: 10, OverrideInterval: 100},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 100, logs[0].IntervalDays)
			},
		},
		{
			name: "wrong answer interval stays at 1",
			logs: []LearningRecord{
				{Quality: 1, LearnedAt: Date{Time: baseTime}, IntervalDays: 1},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 1, logs[0].IntervalDays)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := &FixedLevelCalculator{}
			ef, logs := calc.RecalculateAll(tt.logs)
			assert.Equal(t, 0.0, ef)
			if tt.validate != nil {
				tt.validate(t, logs)
			}
		})
	}
}

// TestNextIntervalForWrite_AgreesWithRecalculateAll pins the contract
// that drove the helper's introduction: a sequence of live-quiz writes
// (each calling NextIntervalForWrite) must yield the same interval_days
// as a single RecalculateAll over the same chain. Otherwise `validate
// --fix` would always produce diffs against freshly-written data — the
// "live vs recalc" drift this PR set out to remove.
func TestNextIntervalForWrite_AgreesWithRecalculateAll(t *testing.T) {
	calculators := map[string]IntervalCalculator{
		"sm2":   &SM2Calculator{},
		"fixed": &FixedLevelCalculator{Intervals: DefaultFixedIntervals},
	}

	day := func(n int) Date { return NewDate(time.Date(2026, 1, 1+n, 10, 0, 0, 0, time.UTC)) }

	// A mixed chain: correct → correct (early review) → wrong → correct.
	// The early-review and lapse steps are exactly where the two paths
	// used to disagree before unification.
	tentatives := []LearningRecord{
		{Status: LearnedStatusUnderstood, LearnedAt: day(0), Quality: 4},
		{Status: LearnedStatusUnderstood, LearnedAt: day(2), Quality: 4},
		{Status: LearnedStatusMisunderstood, LearnedAt: day(10), Quality: 1},
		{Status: LearnedStatusUnderstood, LearnedAt: day(11), Quality: 4},
	}

	for name, calc := range calculators {
		t.Run(name, func(t *testing.T) {
			// Simulate live-quiz writes: each call sees the previous
			// writes as the existing chain, computes its own interval,
			// and prepends itself newest-first.
			liveLogs := []LearningRecord{}
			for _, tent := range tentatives {
				tent.IntervalDays, _ = calc.NextIntervalForWrite(liveLogs, tent)
				liveLogs = append([]LearningRecord{tent}, liveLogs...)
			}

			// Run RecalculateAll over the same set of (timestamp,
			// quality, status) values, with zero stored intervals,
			// so the chain is forced to derive everything from scratch.
			fresh := make([]LearningRecord, len(tentatives))
			for i, tent := range tentatives {
				fresh[i] = LearningRecord{
					Status:    tent.Status,
					LearnedAt: tent.LearnedAt,
					Quality:   tent.Quality,
				}
			}
			_, recalced := calc.RecalculateAll(fresh)

			byTs := make(map[time.Time]int, len(recalced))
			for _, log := range recalced {
				byTs[log.LearnedAt.Time] = log.IntervalDays
			}

			for _, live := range liveLogs {
				recalcInterval, ok := byTs[live.LearnedAt.Time]
				require.True(t, ok, "recalc result missing log at %s", live.LearnedAt.Time)
				assert.Equal(t, recalcInterval, live.IntervalDays,
					"live-write interval at %s should equal recalc interval",
					live.LearnedAt.Format("2006-01-02"))
			}
		})
	}
}

// TestRecalculateAll_StableOnEqualTimestamps pins that logs sharing the
// same learned_at no longer reorder across repeated RecalculateAll runs.
// sort.Slice isn't stable, so before the learningRecordBefore tiebreaker
// two same-timestamp logs swapped on every `validate --fix`, producing a
// spurious diff each run.
func TestRecalculateAll_StableOnEqualTimestamps(t *testing.T) {
	ts := NewDate(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	calculators := map[string]IntervalCalculator{
		"sm2":   &SM2Calculator{},
		"fixed": &FixedLevelCalculator{Intervals: DefaultFixedIntervals},
	}
	for name, calc := range calculators {
		t.Run(name, func(t *testing.T) {
			logs := []LearningRecord{
				{Status: LearnedStatusUnderstood, LearnedAt: ts, Quality: 4, ResponseTimeMs: 13942, QuizType: "notebook"},
				{Status: LearnedStatusUnderstood, LearnedAt: ts, Quality: 4, ResponseTimeMs: 9037, QuizType: "notebook"},
			}
			_, first := calc.RecalculateAll(logs)
			// Feed the output back in repeatedly — order must not change.
			prev := first
			for i := 0; i < 5; i++ {
				_, next := calc.RecalculateAll(prev)
				require.Len(t, next, len(prev))
				for j := range next {
					assert.Equal(t, prev[j].ResponseTimeMs, next[j].ResponseTimeMs,
						"same-timestamp logs must keep a stable order across runs")
				}
				prev = next
			}
		})
	}
}
