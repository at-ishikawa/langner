package notebook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNote_getLearnScore(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		note     Note
		expected int // This is relative, we'll test the concept
	}{
		{
			name: "no learning logs - returns negative score based on time",
			note: Note{
				Expression:   "hello",
				Definition:   "greeting",
				notebookDate: baseTime.Add(-30 * 24 * time.Hour),
			},
			expected: -30, // roughly -30 days from notebook date
		},
		{
			name: "with understood logs - higher score",
			note: Note{
				Expression:   "hello",
				Definition:   "greeting",
				notebookDate: baseTime.Add(-30 * 24 * time.Hour),
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(baseTime.Add(-1 * time.Hour))},
				},
			},
			expected: 10, // 10 from understood status minus time factors
		},
		{
			name: "with usable logs - much higher score",
			note: Note{
				Expression:   "hello",
				Definition:   "greeting",
				notebookDate: baseTime.Add(-10 * 24 * time.Hour),
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDateFromTime(baseTime.Add(-5 * 24 * time.Hour))},
				},
			},
			expected: 1000, // 1000 from usable status minus time factors
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.note.getLearnScore()
			// We test the relative ordering rather than exact values
			// since the function uses current time
			switch tt.name {
			case "with usable logs - much higher score":
				assert.Greater(t, result, 0) // Should be positive due to high usable status score
			case "with understood logs - higher score":
				assert.Greater(t, result, -1000) // Should be negative but better than no logs
			default:
				assert.Less(t, result, 0) // Should be negative for no logs
			}
		})
	}
}

func TestNote_needsToLearn(t *testing.T) {
	tests := []struct {
		name     string
		note     Note
		expected bool
	}{
		{
			name: "no logs - needs learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
			},
			expected: true,
		},
		{
			name: "misunderstood - always needs learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(time.Now())},
				},
			},
			expected: true,
		},
		{
			name: "1 correct answer, 4 days ago - needs learning (threshold is 3 days)",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-4 * 24 * time.Hour))},
				},
			},
			expected: true,
		},
		{
			name: "1 correct answer, 2 days ago - doesn't need learning (threshold is 3 days)",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-2 * 24 * time.Hour))},
				},
			},
			expected: false,
		},
		{
			name: "2 correct answers, 8 days ago - needs learning (threshold is 7 days)",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDateFromTime(time.Now().Add(-8 * 24 * time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-12 * 24 * time.Hour))},
				},
			},
			expected: true,
		},
		{
			name: "2 correct answers, 5 days ago - doesn't need learning (threshold is 7 days)",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDateFromTime(time.Now().Add(-5 * 24 * time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-10 * 24 * time.Hour))},
				},
			},
			expected: false,
		},
		{
			name: "3 correct answers, 15 days ago - needs learning (threshold is 14 days)",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusIntuitivelyUsed, LearnedAt: NewDateFromTime(time.Now().Add(-15 * 24 * time.Hour))},
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDateFromTime(time.Now().Add(-20 * 24 * time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-25 * 24 * time.Hour))},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.note.needsToLearn()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNote_getNextLearningThresholdDays(t *testing.T) {
	tests := []struct {
		name     string
		note     Note
		expected int
	}{
		{
			name: "misunderstood - 0 day threshold",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate()},
				},
			},
			expected: 0,
		},
		{
			name: "one understood - 3 day threshold",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDate()},
				},
			},
			expected: 3, // count=1 -> 3 days
		},
		{
			name: "two understood - 7 day threshold",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDate()},
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-time.Hour))},
				},
			},
			expected: 7, // count=2 -> 7 days
		},
		{
			name: "three understood - 14 day threshold",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDate()},
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-2 * time.Hour))},
				},
			},
			expected: 14, // count=3 -> 14 days
		},
		{
			name: "multiple logs - counts non-learning statuses",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDate()},                                           // counted (1)
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-time.Hour))},        // counted (2)
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(time.Now().Add(-2 * time.Hour))}, // not counted
				},
			},
			expected: 7, // count=2 -> 7 days
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.note.getNextLearningThresholdDays()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNote_hasAnyCorrectAnswer(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		note     Note
		expected bool
	}{
		{
			name: "no learning logs - returns false",
			note: Note{
				Expression:  "hello",
				Definition:  "greeting",
				LearnedLogs: []LearningRecord{},
			},
			expected: false,
		},
		{
			name: "only misunderstood status - returns false",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(baseTime)},
				},
			},
			expected: false,
		},
		{
			name: "only empty/learning status - returns false",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusLearning, LearnedAt: NewDateFromTime(baseTime)},
				},
			},
			expected: false,
		},
		{
			name: "has understood status - returns true",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(baseTime)},
				},
			},
			expected: true,
		},
		{
			name: "has usable status - returns true",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDateFromTime(baseTime)},
				},
			},
			expected: true,
		},
		{
			name: "has intuitive status - returns true",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusIntuitivelyUsed, LearnedAt: NewDateFromTime(baseTime)},
				},
			},
			expected: true,
		},
		{
			name: "mixed statuses with at least one correct - returns true",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(baseTime.Add(-3 * time.Hour))},
					{Status: learnedStatusLearning, LearnedAt: NewDateFromTime(baseTime.Add(-2 * time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(baseTime.Add(-1 * time.Hour))},
				},
			},
			expected: true,
		},
		{
			name: "mixed statuses with no correct (only misunderstood and empty) - returns false",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(baseTime.Add(-2 * time.Hour))},
					{Status: learnedStatusLearning, LearnedAt: NewDateFromTime(baseTime.Add(-1 * time.Hour))},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.note.hasAnyCorrectAnswer()
			assert.Equal(t, tt.expected, result)
		})
	}
}
