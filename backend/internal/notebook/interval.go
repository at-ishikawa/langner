package notebook

import (
	"sort"
	"time"
)

// calendarDaysBetween returns the number of calendar-day boundaries crossed
// between two timestamps, evaluated in each timestamp's own location. The
// early-review guard uses this instead of duration-based math because review
// scheduling is a calendar concept: a review at 9am the day after a 6pm
// review is "1 day later" even though only 15 hours have elapsed. Truncating
// duration.Hours()/24 would round that to 0 and clamp the interval.
//
// Negative results are clamped to 0 — callers only care whether `to` is at
// least one day after `from`.
func calendarDaysBetween(from, to time.Time) int {
	startDay := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	endDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
	days := int(endDay.Sub(startDay).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// DefaultFixedIntervals is the default progression of review intervals in days.
var DefaultFixedIntervals = []int{1, 7, 30, 90, 365, 1095, 1825}

// IntervalCalculator computes review intervals for spaced repetition.
type IntervalCalculator interface {
	// CalculateInterval returns the next interval and new easiness factor
	// given existing logs, the current quality grade, and current EF.
	//
	// Prefer NextIntervalForWrite for live-quiz writes. CalculateInterval
	// does NOT apply the early-review guard, so a chain that's been
	// written via this path and later replayed through RecalculateAll
	// (e.g. by `validate --fix`) may produce different intervals.
	CalculateInterval(logs []LearningRecord, currentQuality int, currentEF float64) (intervalDays int, newEF float64)

	// NextIntervalForWrite is the canonical way to compute the
	// interval_days for a new log that the live quiz is about to write.
	// It appends `tentative` to `existingLogs` and replays the chain
	// through RecalculateAll, so the value returned is identical to
	// what `validate --fix` would compute for the same log later.
	// Returns the tentative log's interval and the chain's final EF
	// (used by SM-2; FixedLevelCalculator returns 0).
	NextIntervalForWrite(existingLogs []LearningRecord, tentative LearningRecord) (intervalDays int, newEF float64)

	// RecalculateAll replays logs oldest-to-newest and recomputes EF and intervals.
	// Returns the final EF and updated logs (sorted newest-first).
	RecalculateAll(logs []LearningRecord) (float64, []LearningRecord)

	// DeriveEF replays logs oldest-to-newest and returns the current easiness factor.
	DeriveEF(logs []LearningRecord) float64
}

// nextIntervalForWrite is the shared implementation: append the tentative
// log, run RecalculateAll, and return the tentative log's resulting
// interval. Used by both SM2Calculator and FixedLevelCalculator so the
// "live write" rule is identical regardless of algorithm.
func nextIntervalForWrite(c IntervalCalculator, existingLogs []LearningRecord, tentative LearningRecord) (int, float64) {
	chain := make([]LearningRecord, 0, len(existingLogs)+1)
	chain = append(chain, tentative)
	chain = append(chain, existingLogs...)
	ef, replayed := c.RecalculateAll(chain)
	for _, log := range replayed {
		if log.LearnedAt.Time.Equal(tentative.LearnedAt.Time) {
			return log.IntervalDays, ef
		}
	}
	// Fallback: RecalculateAll sorts newest-first and the tentative log
	// was just stamped with the current time, so it should be [0].
	if len(replayed) > 0 {
		return replayed[0].IntervalDays, ef
	}
	return 0, ef
}

// SM2Calculator implements the modified SM-2 algorithm.
type SM2Calculator struct{}

// NextIntervalForWrite delegates to the shared helper.
func (c *SM2Calculator) NextIntervalForWrite(existingLogs []LearningRecord, tentative LearningRecord) (int, float64) {
	return nextIntervalForWrite(c, existingLogs, tentative)
}

// CalculateInterval computes the next interval using modified SM-2.
func (c *SM2Calculator) CalculateInterval(logs []LearningRecord, currentQuality int, currentEF float64) (int, float64) {
	correctStreak := GetCorrectStreak(logs)
	lastInterval := GetLastInterval(logs)

	if currentQuality >= 3 {
		correctStreak++
	}

	if currentEF == 0 {
		currentEF = DefaultEasinessFactor
	}

	newEF := UpdateEasinessFactor(currentEF, currentQuality, correctStreak)
	intervalDays := CalculateNextInterval(lastInterval, newEF, currentQuality, correctStreak)
	return intervalDays, newEF
}

// DeriveEF replays logs oldest-to-newest and returns the current SM-2 easiness factor.
func (c *SM2Calculator) DeriveEF(logs []LearningRecord) float64 {
	if len(logs) == 0 {
		return DefaultEasinessFactor
	}

	// Sort ascending (oldest first) for replay
	sorted := make([]LearningRecord, len(logs))
	copy(sorted, logs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LearnedAt.Before(sorted[j].LearnedAt.Time)
	})

	ef := DefaultEasinessFactor
	correctStreak := 0

	for _, log := range sorted {
		quality := log.Quality
		if quality == 0 {
			if log.Status == LearnedStatusMisunderstood {
				quality = int(QualityWrong)
			} else {
				quality = int(QualityCorrect)
			}
		}
		if log.Status == LearnedStatusMisunderstood && quality >= 3 {
			quality = 2
		}
		if quality >= 3 {
			correctStreak++
		} else {
			correctStreak = 0
		}
		ef = UpdateEasinessFactor(ef, quality, correctStreak)
	}

	return ef
}

// RecalculateAll replays logs oldest-to-newest and recomputes SM-2 metrics.
func (c *SM2Calculator) RecalculateAll(logs []LearningRecord) (float64, []LearningRecord) {
	if len(logs) == 0 {
		return DefaultEasinessFactor, logs
	}

	newLogs := make([]LearningRecord, len(logs))
	copy(newLogs, logs)

	// Sort ascending (oldest first) for replay
	sort.Slice(newLogs, func(i, j int) bool {
		return newLogs[i].LearnedAt.Before(newLogs[j].LearnedAt.Time)
	})

	ef := DefaultEasinessFactor
	correctStreak := 0
	lastInterval := 0

	for i := range newLogs {
		log := &newLogs[i]
		quality := log.Quality
		if quality == 0 {
			if log.Status == LearnedStatusMisunderstood {
				quality = int(QualityWrong)
			} else {
				quality = int(QualityCorrect)
			}
			log.Quality = quality
		}

		// Fix inconsistency: misunderstood status must have quality < 3
		if log.Status == LearnedStatusMisunderstood && quality >= 3 {
			quality = 2
			log.Quality = quality
		}

		if quality >= 3 {
			correctStreak++
		} else {
			correctStreak = 0
		}

		ef = UpdateEasinessFactor(ef, quality, correctStreak)

		if log.OverrideInterval > 0 {
			log.IntervalDays = log.OverrideInterval
			lastInterval = log.OverrideInterval
		} else {
			nextInterval := CalculateNextInterval(lastInterval, ef, quality, correctStreak)
			// Early review guard: if a correct answer came before the
			// previous interval elapsed, don't advance the interval.
			// Reviewing early doesn't prove long-term retention.
			if quality >= 3 && lastInterval > 0 && i > 0 {
				elapsed := calendarDaysBetween(newLogs[i-1].LearnedAt.Time, log.LearnedAt.Time)
				if elapsed < lastInterval && nextInterval > lastInterval {
					nextInterval = lastInterval
				}
			}
			log.IntervalDays = nextInterval
			lastInterval = nextInterval
		}
	}

	// Re-sort newest first for storage
	sort.Slice(newLogs, func(i, j int) bool {
		return newLogs[i].LearnedAt.After(newLogs[j].LearnedAt.Time)
	})

	return ef, newLogs
}

// FixedLevelCalculator implements fixed interval levels.
// Fast correct (q=5) advances two levels, other correct (q >= 3) advances one level,
// wrong answers (q < 3) go back one level.
// Levels map to a configurable list of intervals in days.
type FixedLevelCalculator struct {
	Intervals []int
}

// qualityToLevelDelta returns the level change for a given quality grade.
func qualityToLevelDelta(quality int) int {
	if quality >= 5 {
		return 2
	}
	if quality >= 3 {
		return 1
	}
	return -1
}

func (c *FixedLevelCalculator) intervals() []int {
	if len(c.Intervals) == 0 {
		return DefaultFixedIntervals
	}
	return c.Intervals
}

// levelFromInterval finds the level index for a stored interval.
// Returns the highest level where intervals[level] <= lastInterval.
func (c *FixedLevelCalculator) levelFromInterval(lastInterval int) int {
	intervals := c.intervals()
	level := 0
	for i, iv := range intervals {
		if iv <= lastInterval {
			level = i
		}
	}
	return level
}

// DeriveEF returns 0 for fixed level calculator since it does not use EF.
func (c *FixedLevelCalculator) DeriveEF(_ []LearningRecord) float64 {
	return 0
}

// NextIntervalForWrite delegates to the shared helper.
func (c *FixedLevelCalculator) NextIntervalForWrite(existingLogs []LearningRecord, tentative LearningRecord) (int, float64) {
	return nextIntervalForWrite(c, existingLogs, tentative)
}

// CalculateInterval computes the next interval using fixed levels.
// Derives current level from the most recent log's stored interval,
// then advances by the quality delta.
func (c *FixedLevelCalculator) CalculateInterval(logs []LearningRecord, currentQuality int, _ float64) (int, float64) {
	intervals := c.intervals()
	maxLevel := len(intervals) - 1

	// Derive current level from the most recent log's interval
	level := 0
	if len(logs) > 0 {
		level = c.levelFromInterval(logs[0].IntervalDays)
	}

	// Apply current quality
	level += qualityToLevelDelta(currentQuality)

	if level < 0 {
		level = 0
	}
	if level > maxLevel {
		level = maxLevel
	}

	return intervals[level], 0
}

// RecalculateAll replays logs oldest-to-newest using fixed interval levels,
// applying the early-review guard: if a correct answer came before the
// previous interval elapsed, the interval does not advance.
func (c *FixedLevelCalculator) RecalculateAll(logs []LearningRecord) (float64, []LearningRecord) {
	if len(logs) == 0 {
		return 0, logs
	}

	newLogs := make([]LearningRecord, len(logs))
	copy(newLogs, logs)

	// Sort ascending (oldest first) for replay
	sort.Slice(newLogs, func(i, j int) bool {
		return newLogs[i].LearnedAt.Before(newLogs[j].LearnedAt.Time)
	})

	lastInterval := 0
	for i := range newLogs {
		log := &newLogs[i]
		q := log.Quality
		if q == 0 {
			if log.Status == LearnedStatusMisunderstood {
				q = int(QualityWrong)
			} else {
				q = int(QualityCorrect)
			}
			log.Quality = q
		}

		var nextInterval int
		if log.OverrideInterval > 0 {
			nextInterval = log.OverrideInterval
		} else {
			// Build a fake "previous logs" slice with just the last interval
			// so CalculateInterval can derive the current level.
			prevLogs := []LearningRecord{}
			if lastInterval > 0 {
				prevLogs = []LearningRecord{{IntervalDays: lastInterval}}
			}
			nextInterval, _ = c.CalculateInterval(prevLogs, q, 0)
		}

		// Early review guard
		if q >= 3 && lastInterval > 0 && i > 0 {
			elapsed := calendarDaysBetween(newLogs[i-1].LearnedAt.Time, log.LearnedAt.Time)
			if elapsed < lastInterval && nextInterval > lastInterval {
				nextInterval = lastInterval
			}
		}

		log.IntervalDays = nextInterval
		lastInterval = nextInterval
	}

	// Re-sort newest first
	sort.Slice(newLogs, func(i, j int) bool {
		return newLogs[i].LearnedAt.After(newLogs[j].LearnedAt.Time)
	})

	return 0, newLogs
}

// NewIntervalCalculator creates an IntervalCalculator based on the algorithm name.
func NewIntervalCalculator(algorithm string, fixedIntervals []int) IntervalCalculator {
	if algorithm == "fixed" {
		return &FixedLevelCalculator{Intervals: fixedIntervals}
	}
	return &SM2Calculator{}
}
