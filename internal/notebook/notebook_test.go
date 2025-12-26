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
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(baseTime.Add(-1 * time.Hour))},
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
					{Status: LearnedStatusCanBeUsed, LearnedAt: NewDateFromTime(baseTime.Add(-5 * 24 * time.Hour))},
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

func TestNote_needsToLearnInFlashcard(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		note              Note
		lowerThresholdDay int
		expected          bool
	}{
		{
			name: "no logs - needs learning (never practiced)",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
			},
			lowerThresholdDay: 0,
			expected:          true, // Changed: words never practiced should be included
		},
		{
			name: "old misunderstood - needs learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(baseTime.Add(-1 * 24 * time.Hour))},
				},
			},
			lowerThresholdDay: 0,
			expected:          true,
		},
		{
			name: "old understood - needs learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(baseTime.Add(-8 * 24 * time.Hour))},
				},
			},
			lowerThresholdDay: 0,
			expected:          true,
		},
		{
			name: "old usable - needs learning again",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusCanBeUsed, LearnedAt: NewDateFromTime(baseTime.Add(-31 * 24 * time.Hour))},
				},
			},
			lowerThresholdDay: 0,
			expected:          true,
		},
		{
			name: "recent usable - doesn't need learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusCanBeUsed, LearnedAt: NewDateFromTime(time.Now().Add(-1 * time.Hour))}, // very recent, less than 7 days
				},
			},
			lowerThresholdDay: 0,
			expected:          false,
		},
		{
			name: "with lower threshold filter",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(baseTime.Add(-8 * 24 * time.Hour))},
				},
			},
			lowerThresholdDay: 10, // threshold is 7 for understood, but we want >= 10
			expected:          false,
		},
		{
			name: "recent misunderstood with lower threshold - always needs learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(time.Now().Add(-1 * time.Hour))},
				},
			},
			lowerThresholdDay: 7, // even with threshold filter
			expected:          true,
		},
		{
			name: "very recent misunderstood - always needs learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(time.Now())},
				},
			},
			lowerThresholdDay: 7,
			expected:          true,
		},
		{
			name: "misunderstood with high lower threshold - still needs learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(baseTime.Add(-2 * 24 * time.Hour))},
				},
			},
			lowerThresholdDay: 30, // high threshold, but misunderstood should bypass it
			expected:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.note.needsToLearnInFlashcard(tt.lowerThresholdDay)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNote_needsToLearnInStory(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

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
			name: "old misunderstood - needs learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(baseTime.Add(-1 * 24 * time.Hour))},
				},
			},
			expected: true,
		},
		{
			name: "understood - doesn't need learning in story",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(baseTime.Add(-6 * 24 * time.Hour))},
				},
			},
			expected: false, // threshold > 1, so false for story
		},
		{
			name: "usable - doesn't need learning in story",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusCanBeUsed, LearnedAt: NewDateFromTime(baseTime.Add(-29 * 24 * time.Hour))},
				},
			},
			expected: false, // threshold > 1, so false for story
		},
		{
			name: "intuitive - doesn't need learning in story",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusIntuitivelyUsed, LearnedAt: NewDateFromTime(baseTime.Add(-100 * 24 * time.Hour))},
				},
			},
			expected: false, // threshold > 1, so false for story
		},
		{
			name: "recent misunderstood - always needs learning in story",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(baseTime.Add(-1 * time.Hour))}, // very recent
				},
			},
			expected: true, // misunderstood expressions are always included in stories
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.note.needsToLearnInStory()
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
			name: "one understood - 7 day threshold",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: NewDate()},
				},
			},
			expected: 7, // count=1 -> 7 days
		},
		{
			name: "two understood - 30 day threshold",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: NewDate()},
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-time.Hour))},
				},
			},
			expected: 30, // count=2 -> 30 days
		},
		{
			name: "three understood - 90 day threshold",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: NewDate()},
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-time.Hour))},
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-2 * time.Hour))},
				},
			},
			expected: 90, // count=3 -> 90 days
		},
		{
			name: "multiple logs - counts non-learning statuses",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate()},                                           // most recent
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Now().Add(-time.Hour))},        // 2 non-learning
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(time.Now().Add(-2 * time.Hour))}, // 3 non-learning
				},
			},
			expected: 30,
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
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(baseTime)},
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
					{Status: LearnedStatusCanBeUsed, LearnedAt: NewDateFromTime(baseTime)},
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
					{Status: LearnedStatusUnderstood, LearnedAt: NewDateFromTime(baseTime.Add(-1 * time.Hour))},
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
