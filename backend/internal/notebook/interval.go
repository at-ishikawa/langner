package notebook

import (
	"sort"
)

// DefaultFixedIntervals is the default progression of review intervals in days.
var DefaultFixedIntervals = []int{1, 7, 30, 90, 365, 1095, 1825}

// IntervalCalculator computes review intervals for spaced repetition.
type IntervalCalculator interface {
	// CalculateInterval returns the next interval and new easiness factor
	// given existing logs, the current quality grade, and current EF.
	CalculateInterval(logs []LearningRecord, currentQuality int, currentEF float64) (intervalDays int, newEF float64)

	// RecalculateAll replays logs oldest-to-newest and recomputes EF and intervals.
	// Returns the final EF and updated logs (sorted newest-first).
	RecalculateAll(logs []LearningRecord) (float64, []LearningRecord)

	// DeriveEF replays logs oldest-to-newest and returns the current easiness factor.
	DeriveEF(logs []LearningRecord) float64
}

// SM2Calculator implements the modified SM-2 algorithm.
type SM2Calculator struct{}

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
				elapsed := int(log.LearnedAt.Sub(newLogs[i-1].LearnedAt.Time).Hours() / 24)
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

// snapToNextLevel returns the smallest fixed interval >= the given interval.
func (c *FixedLevelCalculator) snapToNextLevel(interval int) int {
	for _, iv := range c.intervals() {
		if iv >= interval {
			return iv
		}
	}
	// If interval exceeds all levels, return max
	intervals := c.intervals()
	return intervals[len(intervals)-1]
}

// DeriveEF returns 0 for fixed level calculator since it does not use EF.
func (c *FixedLevelCalculator) DeriveEF(_ []LearningRecord) float64 {
	return 0
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
			elapsed := int(log.LearnedAt.Sub(newLogs[i-1].LearnedAt.Time).Hours() / 24)
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
