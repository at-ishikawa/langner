package notebook

import (
	"testing"
)

func TestUpdateEasinessFactor(t *testing.T) {
	tests := []struct {
		name                  string
		ef                    float64
		quality               int
		previousCorrectStreak int
		expected              float64
	}{
		{
			name:                  "quality 5 increases EF",
			ef:                    2.5,
			quality:               5,
			previousCorrectStreak: 1,
			expected:              2.6,
		},
		{
			name:                  "quality 4 increases EF (shifted up from 4)",
			ef:                    2.5,
			quality:               4,
			previousCorrectStreak: 1,
			expected:              2.6,
		},
		{
			name:                  "quality 3 maintains EF (shifted up from 3)",
			ef:                    2.5,
			quality:               3,
			previousCorrectStreak: 1,
			expected:              2.5,
		},
		{
			name:                  "quality 2 decreases EF by 0.14 (shifted up from 2)",
			ef:                    2.5,
			quality:               2,
			previousCorrectStreak: 1,
			expected:              2.36, // 2.5 - 0.14
		},
		{
			name:                  "quality 1 full penalty for new word",
			ef:                    2.5,
			quality:               1,
			previousCorrectStreak: 2,
			expected:              2.18, // 2.5 + (-0.32)
		},
		{
			name:                  "quality 1 scaled penalty for streak 3-5",
			ef:                    2.5,
			quality:               1,
			previousCorrectStreak: 5,
			expected:              2.2632, // 2.5 + (-0.32 * 0.74)
		},
		{
			name:                  "quality 1 scaled penalty for streak 6-9",
			ef:                    2.5,
			quality:               1,
			previousCorrectStreak: 8,
			expected:              2.3208, // 2.5 + (-0.32 * 0.56)
		},
		{
			name:                  "quality 1 scaled penalty for streak 10+",
			ef:                    2.5,
			quality:               1,
			previousCorrectStreak: 12,
			expected:              2.3816, // 2.5 + (-0.32 * 0.37)
		},
		{
			name:                  "never goes below MinEasinessFactor",
			ef:                    1.3,
			quality:               1,
			previousCorrectStreak: 1,
			expected:              MinEasinessFactor,
		},
		{
			name:                  "default EF when zero",
			ef:                    0,
			quality:               5,
			previousCorrectStreak: 1,
			expected:              2.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UpdateEasinessFactor(tt.ef, tt.quality, tt.previousCorrectStreak)
			if result < tt.expected-0.01 || result > tt.expected+0.01 {
				t.Errorf("UpdateEasinessFactor(%v, %v, %v) = %v, want %v", tt.ef, tt.quality, tt.previousCorrectStreak, result, tt.expected)
			}
		})
	}
}

func TestCalculateNextInterval(t *testing.T) {
	tests := []struct {
		name          string
		lastInterval  int
		ef            float64
		quality       int
		correctStreak int
		expected      int
	}{
		{
			name:          "first correct answer",
			lastInterval:  0,
			ef:            2.5,
			quality:       4,
			correctStreak: 1,
			expected:      3,
		},
		{
			name:          "second correct answer",
			lastInterval:  3,
			ef:            2.5,
			quality:       4,
			correctStreak: 2,
			expected:      8, // 3 * 2.5 = 7.5 -> 8 (now uses lastInterval * EF instead of hardcoded 10)
		},
		{
			name:          "third correct answer",
			lastInterval:  10,
			ef:            2.5,
			quality:       4,
			correctStreak: 3,
			expected:      25, // 10 * 2.5 = 25
		},
			{
				name:          "first correct answer after lapse with small lastInterval",
				lastInterval:  5, // e.g., after a lapse where previous was 10, then reduced to 5
				ef:            2.6,
				quality:       4,
				correctStreak: 1, // First correct answer after a lapse
				expected:      13, // 5 * 2.6 = 13
			},
			{
				name:          "correct answer after lapse on well-learned card",
				lastInterval:  14, // e.g., after a lapse from a 28-day interval
				ef:            2.6,
				quality:       4,
				correctStreak: 1, // First correct answer after a lapse
				expected:      37, // 14 * 2.6 = 36.4 -> 37
			},			{
				name:          "wrong answer proportional reduction for new word",
				lastInterval:  10,
				ef:            2.5,
				quality:       1,
				correctStreak: 2,
				expected:      5, // 10 * 0.5
			},
		{
			name:          "wrong answer proportional for streak 3-5",
			lastInterval:  30,
			ef:            2.5,
			quality:       1,
			correctStreak: 5,
			expected:      15, // 30 * 0.5
		},
		{
			name:          "wrong answer proportional for streak 6-9",
			lastInterval:  90,
			ef:            2.5,
			quality:       1,
			correctStreak: 8,
			expected:      54, // 90 * 0.6
		},
		{
			name:          "wrong answer proportional for streak 10+",
			lastInterval:  180,
			ef:            2.5,
			quality:       1,
			correctStreak: 12,
			expected:      126, // 180 * 0.7
		},
		{
			name:          "wrong answer with zero last interval returns 1",
			lastInterval:  0,
			ef:            2.5,
			quality:       1,
			correctStreak: 5,
			expected:      1, // 0 * 0.5 = 0, clamped to 1
		},
		{
			name:          "default EF when zero",
			lastInterval:  10,
			ef:            0,
			quality:       4,
			correctStreak: 3,
			expected:      25, // 10 * 2.5 (default EF)
		},
		{
			name:          "fallback lastInterval when zero on high streak",
			lastInterval:  0,
			ef:            2.5,
			quality:       4,
			correctStreak: 3,
			expected:      25, // fallback to 10 * 2.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateNextInterval(tt.lastInterval, tt.ef, tt.quality, tt.correctStreak)
			if result != tt.expected {
				t.Errorf("CalculateNextInterval(%v, %v, %v, %v) = %v, want %v", tt.lastInterval, tt.ef, tt.quality, tt.correctStreak, result, tt.expected)
			}
		})
	}
}

func TestGetCorrectStreak(t *testing.T) {
	tests := []struct {
		name     string
		logs     []LearningRecord
		expected int
	}{
		{
			name:     "empty logs",
			logs:     []LearningRecord{},
			expected: 0,
		},
		{
			name: "all correct with quality",
			logs: []LearningRecord{
				{Quality: 5},
				{Quality: 4},
				{Quality: 5},
			},
			expected: 3,
		},
		{
			name: "wrong answer breaks streak",
			logs: []LearningRecord{
				{Quality: 5},
				{Quality: 1},
				{Quality: 5},
			},
			expected: 1,
		},
		{
			name: "old data without quality field",
			logs: []LearningRecord{
				{Status: learnedStatusCanBeUsed},
				{Status: LearnedStatusUnderstood},
				{Status: LearnedStatusMisunderstood},
				{Status: learnedStatusCanBeUsed},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCorrectStreak(tt.logs)
			if result != tt.expected {
				t.Errorf("GetCorrectStreak() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetLastInterval(t *testing.T) {
	tests := []struct {
		name     string
		logs     []LearningRecord
		expected int
	}{
		{
			name:     "empty logs",
			logs:     []LearningRecord{},
			expected: 0,
		},
		{
			name: "returns first log interval",
			logs: []LearningRecord{
				{IntervalDays: 30},
				{IntervalDays: 6},
				{IntervalDays: 1},
			},
			expected: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLastInterval(tt.logs)
			if result != tt.expected {
				t.Errorf("GetLastInterval() = %v, want %v", result, tt.expected)
			}
		})
	}
}
