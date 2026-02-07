package notebook

import "math"

const (
	DefaultEasinessFactor = 2.5
	MinEasinessFactor     = 1.3
)

// UpdateEasinessFactor calculates new EF based on quality grade
// Uses scaled penalty for wrong answers based on previous correct streak
func UpdateEasinessFactor(ef float64, quality int, previousCorrectStreak int) float64 {
	if ef == 0 {
		ef = DefaultEasinessFactor
	}

	q := float64(quality)

	// Standard SM-2 delta for correct answers
	delta := 0.1 - (5-q)*(0.08+(5-q)*0.02)

	// Scale down penalty for wrong answers on well-learned words
	if quality < 3 && previousCorrectStreak > 2 {
		var scaleFactor float64
		switch {
		case previousCorrectStreak >= 10:
			scaleFactor = 0.37  // -0.54 * 0.37 ≈ -0.20
		case previousCorrectStreak >= 6:
			scaleFactor = 0.56  // -0.54 * 0.56 ≈ -0.30
		case previousCorrectStreak >= 3:
			scaleFactor = 0.74  // -0.54 * 0.74 ≈ -0.40
		default:
			scaleFactor = 1.0
		}
		delta = delta * scaleFactor
	}

	newEF := ef + delta
	return math.Max(newEF, MinEasinessFactor)
}

// CalculateNextInterval calculates the next review interval
// On correct: interval = lastInterval * EF (or 1/6 for first reviews)
// On wrong: interval = lastInterval * reduction factor (proportional)
func CalculateNextInterval(lastInterval int, ef float64, quality int, correctStreak int) int {
	if ef == 0 {
		ef = DefaultEasinessFactor
	}

	// Wrong answer: proportional reduction
	if quality < 3 {
		return calculateLapseInterval(lastInterval, correctStreak)
	}

	// Correct answer: grow interval
	switch correctStreak {
	case 1:
		return 1
	case 2:
		return 6
	default:
		// Use last interval * EF
		if lastInterval == 0 {
			lastInterval = 6 // fallback for migration
		}
		return int(math.Ceil(float64(lastInterval) * ef))
	}
}

// calculateLapseInterval returns interval after wrong answer
// Proportional reduction based on previous progress
func calculateLapseInterval(lastInterval int, previousCorrectStreak int) int {
	if previousCorrectStreak <= 2 {
		return 1 // Still learning: full reset
	}

	var multiplier float64
	switch {
	case previousCorrectStreak >= 10:
		multiplier = 0.7
	case previousCorrectStreak >= 6:
		multiplier = 0.6
	case previousCorrectStreak >= 3:
		multiplier = 0.5
	default:
		multiplier = 0.5
	}

	newInterval := int(math.Ceil(float64(lastInterval) * multiplier))
	if newInterval < 1 {
		return 1
	}
	return newInterval
}

// GetCorrectStreak returns consecutive correct answers (quality >= 3)
// Counts from most recent, stops at first wrong answer
// If quality field is 0 (old data), counts non-misunderstood statuses as correct
func GetCorrectStreak(logs []LearningRecord) int {
	count := 0
	for _, log := range logs {
		// For old data without quality field, infer from status
		if log.Quality == 0 {
			if log.Status == LearnedStatusMisunderstood {
				break // Hit a wrong answer, stop counting
			}
			if log.Status != "" && log.Status != learnedStatusLearning {
				count++
			}
			continue
		}

		// For new data with quality field
		if log.Quality < 3 {
			break // Hit a wrong answer, stop counting
		}
		count++
	}
	return count
}

// GetLastInterval returns the interval from the most recent log
// Returns 0 if no logs exist
func GetLastInterval(logs []LearningRecord) int {
	if len(logs) == 0 {
		return 0
	}
	return logs[0].IntervalDays
}
