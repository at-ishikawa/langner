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

// levelFromLogs derives the current level by replaying quality history.
func (c *FixedLevelCalculator) levelFromLogs(logs []LearningRecord) int {
	level := 0
	intervals := c.intervals()
	maxLevel := len(intervals) - 1

	// Iterate oldest to newest (logs are stored newest-first)
	for i := len(logs) - 1; i >= 0; i-- {
		q := logs[i].Quality
		if q == 0 {
			if logs[i].Status == LearnedStatusMisunderstood {
				q = int(QualityWrong)
			} else {
				q = int(QualityCorrect)
			}
		}

		level += qualityToLevelDelta(q)

		if level < 0 {
			level = 0
		}
		if level > maxLevel {
			level = maxLevel
		}
	}

	return level
}

// CalculateInterval computes the next interval using fixed levels.
func (c *FixedLevelCalculator) CalculateInterval(logs []LearningRecord, currentQuality int, _ float64) (int, float64) {
	intervals := c.intervals()
	maxLevel := len(intervals) - 1

	level := c.levelFromLogs(logs)

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

// RecalculateAll replays logs oldest-to-newest and recomputes fixed-level intervals.
func (c *FixedLevelCalculator) RecalculateAll(logs []LearningRecord) (float64, []LearningRecord) {
	if len(logs) == 0 {
		return 0, logs
	}

	intervals := c.intervals()
	maxLevel := len(intervals) - 1

	newLogs := make([]LearningRecord, len(logs))
	copy(newLogs, logs)

	// Sort ascending (oldest first) for replay
	sort.Slice(newLogs, func(i, j int) bool {
		return newLogs[i].LearnedAt.Before(newLogs[j].LearnedAt.Time)
	})

	level := 0
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

		level += qualityToLevelDelta(q)

		if level < 0 {
			level = 0
		}
		if level > maxLevel {
			level = maxLevel
		}

		if log.OverrideInterval > 0 {
			log.IntervalDays = log.OverrideInterval
		} else {
			log.IntervalDays = intervals[level]
		}
	}

	// Re-sort newest first for storage
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
