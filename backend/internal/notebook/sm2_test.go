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
			name:                  "quality 4 maintains EF",
			ef:                    2.5,
			quality:               4,
			previousCorrectStreak: 1,
			expected:              2.5,
		},
		{
			name:                  "quality 3 decreases EF slightly",
			ef:                    2.5,
			quality:               3,
			previousCorrectStreak: 1,
			expected:              2.36,
		},
		{
			name:                  "quality 1 full penalty for new word",
			ef:                    2.5,
			quality:               1,
			previousCorrectStreak: 2,
			expected:              1.96, // 2.5 - 0.54
		},
		{
			name:                  "quality 1 scaled penalty for streak 3-5",
			ef:                    2.5,
			quality:               1,
			previousCorrectStreak: 5,
			expected:              2.1004, // 2.5 + (-0.54 * 0.74)
		},
		{
			name:                  "quality 1 scaled penalty for streak 6-9",
			ef:                    2.5,
			quality:               1,
			previousCorrectStreak: 8,
			expected:              2.1976, // 2.5 + (-0.54 * 0.56)
		},
		{
			name:                  "quality 1 scaled penalty for streak 10+",
			ef:                    2.5,
			quality:               1,
			previousCorrectStreak: 12,
			expected:              2.3002, // 2.5 + (-0.54 * 0.37)
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
			expected:      1,
		},
		{
			name:          "second correct answer",
			lastInterval:  1,
			ef:            2.5,
			quality:       4,
			correctStreak: 2,
			expected:      6,
		},
		{
			name:          "third correct answer",
			lastInterval:  6,
			ef:            2.5,
			quality:       4,
			correctStreak: 3,
			expected:      15, // 6 * 2.5 = 15
		},
		{
			name:          "wrong answer resets for new word",
			lastInterval:  6,
			ef:            2.5,
			quality:       1,
			correctStreak: 2,
			expected:      1,
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
			lastInterval:  6,
			ef:            0,
			quality:       4,
			correctStreak: 3,
			expected:      15, // 6 * 2.5 (default EF)
		},
		{
			name:          "fallback lastInterval when zero on high streak",
			lastInterval:  0,
			ef:            2.5,
			quality:       4,
			correctStreak: 3,
			expected:      15, // fallback to 6 * 2.5
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
				{Status: learnedStatusUnderstood},
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
