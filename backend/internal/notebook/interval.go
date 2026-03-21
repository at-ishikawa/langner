package notebook

import (
	"math"
	"sort"
)

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

// ExponentialCalculator implements the exponential interval algorithm.
type ExponentialCalculator struct {
	Base float64
}

// CalculateInterval computes the next interval using exponential scoring.
// score = max(sum(q - 3) for all logs + (currentQuality - 3), 1)
// interval = base ^ max(score - 1, 0)
func (c *ExponentialCalculator) CalculateInterval(logs []LearningRecord, currentQuality int, _ float64) (int, float64) {
	base := c.base()

	// Accumulate score from existing logs (oldest to newest = reverse of storage order)
	score := 0
	for i := len(logs) - 1; i >= 0; i-- {
		q := logs[i].Quality
		if q == 0 {
			if logs[i].Status == LearnedStatusMisunderstood {
				q = int(QualityWrong)
			} else {
				q = int(QualityCorrect)
			}
		}
		score += q - 3
	}

	// Add current quality
	score += currentQuality - 3

	if score < 1 {
		score = 1
	}

	exponent := score - 1
	if exponent < 0 {
		exponent = 0
	}

	intervalDays := int(math.Pow(base, float64(exponent)))
	return intervalDays, 0
}

// RecalculateAll replays logs oldest-to-newest and recomputes exponential intervals.
func (c *ExponentialCalculator) RecalculateAll(logs []LearningRecord) (float64, []LearningRecord) {
	if len(logs) == 0 {
		return 0, logs
	}

	base := c.base()

	newLogs := make([]LearningRecord, len(logs))
	copy(newLogs, logs)

	// Sort ascending (oldest first) for replay
	sort.Slice(newLogs, func(i, j int) bool {
		return newLogs[i].LearnedAt.Before(newLogs[j].LearnedAt.Time)
	})

	score := 0
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

		score += q - 3
		clampedScore := score
		if clampedScore < 1 {
			clampedScore = 1
		}

		exponent := clampedScore - 1
		if exponent < 0 {
			exponent = 0
		}

		if log.OverrideInterval > 0 {
			log.IntervalDays = log.OverrideInterval
		} else {
			log.IntervalDays = int(math.Pow(base, float64(exponent)))
		}
	}

	// Re-sort newest first for storage
	sort.Slice(newLogs, func(i, j int) bool {
		return newLogs[i].LearnedAt.After(newLogs[j].LearnedAt.Time)
	})

	return 0, newLogs
}

func (c *ExponentialCalculator) base() float64 {
	if c.Base <= 0 {
		return 4
	}
	return c.Base
}

// NewIntervalCalculator creates an IntervalCalculator based on the algorithm name.
func NewIntervalCalculator(algorithm string, exponentialBase float64) IntervalCalculator {
	if algorithm == "exponential" {
		return &ExponentialCalculator{Base: exponentialBase}
	}
	return &SM2Calculator{}
}
