package notebook

import (
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestDate_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		wantDay int
	}{
		{
			name:    "YYYY-MM-DD format",
			yaml:    "date: 2025-06-15\n",
			wantDay: 15,
		},
		{
			name:    "RFC3339 format",
			yaml:    "date: 2025-06-15T10:30:00Z\n",
			wantDay: 15,
		},
		{
			name:    "RFC3339Nano format",
			yaml:    "date: 2025-06-15T10:30:00.123456789Z\n",
			wantDay: 15,
		},
		{
			name:    "invalid format",
			yaml:    "date: not-a-date\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result struct {
				Date Date `yaml:"date"`
			}
			err := yaml.Unmarshal([]byte(tt.yaml), &result)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantDay, result.Date.Day())
		})
	}
}

func TestDate_MarshalYAML(t *testing.T) {
	d := NewDate(time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC))
	got, err := d.MarshalYAML()
	assert.NoError(t, err)
	assert.Equal(t, "2025-06-15T10:30:00Z", got)
}

func TestNewDate(t *testing.T) {
	t.Run("with time argument", func(t *testing.T) {
		fixedTime := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		d := NewDate(fixedTime)
		assert.Equal(t, fixedTime, d.Time)
	})

	t.Run("without argument uses current time", func(t *testing.T) {
		before := time.Now()
		d := NewDate()
		after := time.Now()
		assert.False(t, d.Time.Before(before))
		assert.False(t, d.Time.After(after))
	})
}

func TestNote_setDetails(t *testing.T) {
	tests := []struct {
		name          string
		note          Note
		dictionaryMap map[string]rapidapi.Response
		youTubeURL    string
		wantErr       bool
		wantMeaning   string
	}{
		{
			name: "set details from dictionary",
			note: Note{
				Expression:       "hello",
				DictionaryNumber: 1,
			},
			dictionaryMap: map[string]rapidapi.Response{
				"hello": {
					Word: "hello",
					Pronunciation: rapidapi.Pronunciation{All: "heh-loh"},
					Results: []rapidapi.Result{
						{
							PartOfSpeech: "interjection",
							Definition:   "a greeting",
							Synonyms:     []string{"hi"},
							Examples:     []string{"Hello there!"},
						},
					},
				},
			},
			wantMeaning: "a greeting",
		},
		{
			name: "dictionary number out of range",
			note: Note{
				Expression:       "hello",
				DictionaryNumber: 5,
			},
			dictionaryMap: map[string]rapidapi.Response{
				"hello": {
					Word: "hello",
					Results: []rapidapi.Result{
						{Definition: "a greeting"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "word not in dictionary but has meaning",
			note: Note{
				Expression: "hello",
				Meaning:    "a greeting",
			},
			dictionaryMap: map[string]rapidapi.Response{},
			wantMeaning:   "a greeting",
		},
		{
			name: "word not in dictionary, no meaning, no images, no statements",
			note: Note{
				Expression: "hello",
				Level:      ExpressionLevelNew,
			},
			dictionaryMap: map[string]rapidapi.Response{},
			wantErr:       true,
		},
		{
			name: "word not in dictionary but has statements",
			note: Note{
				Expression: "hello",
				Statements: []Phrase{{Remarks: "hi there"}},
			},
			dictionaryMap: map[string]rapidapi.Response{},
		},
		{
			name: "word not in dictionary but has images",
			note: Note{
				Expression: "hello",
				Level:      ExpressionLevelNew,
				Images:     []string{"hello.png"},
			},
			dictionaryMap: map[string]rapidapi.Response{},
		},
		{
			name: "word not in dictionary but has synonyms",
			note: Note{
				Expression: "hello",
				Level:      ExpressionLevelNew,
				Synonyms:   []string{"hi"},
			},
			dictionaryMap: map[string]rapidapi.Response{},
		},
		{
			name: "word with unusable level and no meaning is valid",
			note: Note{
				Expression: "hello",
				Level:      ExpressionLevelUnusable,
			},
			dictionaryMap: map[string]rapidapi.Response{},
		},
		{
			name: "uses definition field as dictionary key",
			note: Note{
				Expression:       "greetings",
				Definition:       "hello",
				DictionaryNumber: 1,
			},
			dictionaryMap: map[string]rapidapi.Response{
				"hello": {
					Word:          "hello",
					Pronunciation: rapidapi.Pronunciation{All: "heh-loh"},
					Results: []rapidapi.Result{
						{Definition: "a greeting", PartOfSpeech: "noun"},
					},
				},
			},
			wantMeaning: "a greeting",
		},
		{
			name: "sets youtube URL when time seconds present",
			note: Note{
				Expression:         "hello",
				DictionaryNumber:   1,
				YouTubeTimeSeconds: 42,
			},
			dictionaryMap: map[string]rapidapi.Response{
				"hello": {
					Word: "hello",
					Results: []rapidapi.Result{
						{Definition: "a greeting"},
					},
				},
			},
			youTubeURL:  "https://youtube.com/watch?v=abc",
			wantMeaning: "a greeting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note := tt.note
			err := note.setDetails(tt.dictionaryMap, tt.youTubeURL)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			if tt.wantMeaning != "" {
				assert.Equal(t, tt.wantMeaning, note.Meaning)
			}
		})
	}
}

func TestGetThresholdDaysFromCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  int
	}{
		{name: "count 0", count: 0, want: 0},
		{name: "count 1", count: 1, want: 3},
		{name: "count 6", count: 6, want: 90},
		{name: "count 12", count: 12, want: 1095},
		{name: "count > 12", count: 15, want: 9223372036854775807}, // math.MaxInt
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetThresholdDaysFromCount(tt.count)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNote_getLearnScore(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		note           Note
		wantUpperLimit int
		wantLowerLimit int
	}{
		{
			name: "no learning logs - returns negative score based on time",
			note: Note{
				Expression:   "hello",
				Definition:   "greeting",
				notebookDate: now.Add(-30 * 24 * time.Hour),
			},
			// lastLearnedAt returns time.Time{} (year 0001), so days penalty is enormous
			wantUpperLimit: -10_000,
			wantLowerLimit: -1_000_000_000,
		},
		{
			name: "with understood logs - higher than no logs but still negative",
			note: Note{
				Expression:   "hello",
				Definition:   "greeting",
				notebookDate: now.Add(-30 * 24 * time.Hour),
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(now.Add(-1 * time.Hour))},
				},
			},
			// score=10, days≈0, notebookDays≈30 → ~-20
			wantUpperLimit: 0,
			wantLowerLimit: -100,
		},
		{
			name: "with usable logs - positive score",
			note: Note{
				Expression:   "hello",
				Definition:   "greeting",
				notebookDate: now.Add(-10 * 24 * time.Hour),
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(now.Add(-5 * 24 * time.Hour))},
				},
			},
			// score=1000, days≈5, notebookDays≈10 → ~985
			wantUpperLimit: 1000,
			wantLowerLimit: 900,
		},
		{
			name: "with misunderstood logs - negative score",
			note: Note{
				Expression:   "hello",
				Definition:   "greeting",
				notebookDate: now.Add(-10 * 24 * time.Hour),
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(now.Add(-1 * time.Hour))},
				},
			},
			// score=-5, days≈0, notebookDays≈10 → ~-15
			wantUpperLimit: 0,
			wantLowerLimit: -100,
		},
		{
			name: "with intuitively used logs - very high positive score",
			note: Note{
				Expression:   "hello",
				Definition:   "greeting",
				notebookDate: now.Add(-10 * 24 * time.Hour),
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusIntuitivelyUsed, LearnedAt: NewDate(now.Add(-1 * time.Hour))},
				},
			},
			// score=100000, days≈0, notebookDays≈10 → ~99990
			wantUpperLimit: 100_000,
			wantLowerLimit: 99_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.note.getLearnScore()
			assert.Less(t, got, tt.wantUpperLimit)
			assert.Greater(t, got, tt.wantLowerLimit)
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
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(time.Now())},
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
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Now().Add(-4 * 24 * time.Hour))},
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
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Now().Add(-2 * 24 * time.Hour))},
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
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Now().Add(-8 * 24 * time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Now().Add(-12 * 24 * time.Hour))},
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
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Now().Add(-5 * 24 * time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Now().Add(-10 * 24 * time.Hour))},
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
					{Status: learnedStatusIntuitivelyUsed, LearnedAt: NewDate(time.Now().Add(-15 * 24 * time.Hour))},
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Now().Add(-20 * 24 * time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Now().Add(-25 * 24 * time.Hour))},
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
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Now().Add(-time.Hour))},
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
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Now().Add(-time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Now().Add(-2 * time.Hour))},
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
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Now().Add(-time.Hour))},        // counted (2)
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(time.Now().Add(-2 * time.Hour))}, // not counted
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
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(baseTime)},
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
					{Status: learnedStatusLearning, LearnedAt: NewDate(baseTime)},
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
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(baseTime)},
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
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(baseTime)},
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
					{Status: learnedStatusIntuitivelyUsed, LearnedAt: NewDate(baseTime)},
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
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(baseTime.Add(-3 * time.Hour))},
					{Status: learnedStatusLearning, LearnedAt: NewDate(baseTime.Add(-2 * time.Hour))},
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(baseTime.Add(-1 * time.Hour))},
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
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(baseTime.Add(-2 * time.Hour))},
					{Status: learnedStatusLearning, LearnedAt: NewDate(baseTime.Add(-1 * time.Hour))},
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

func TestNote_needsToLearnInNotebook(t *testing.T) {
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
			name: "only misunderstood - needs learning (no correct answers)",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(baseTime)},
				},
			},
			expected: true,
		},
		{
			name: "latest is misunderstood after correct - needs learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(baseTime)},
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(baseTime.Add(-24 * time.Hour))},
				},
			},
			expected: true,
		},
		{
			name: "latest is correct - doesn't need learning (no spaced repetition)",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(baseTime)},
				},
			},
			expected: false,
		},
		{
			name: "multiple correct answers, latest is correct - doesn't need learning",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(baseTime)},
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(baseTime.Add(-24 * time.Hour))},
				},
			},
			expected: false,
		},
		{
			name: "correct answer from long ago - doesn't need learning (notebook ignores time threshold)",
			note: Note{
				Expression: "hello",
				Definition: "greeting",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(baseTime.Add(-365 * 24 * time.Hour))},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.note.needsToLearnInNotebook()
			assert.Equal(t, tt.expected, result)
		})
	}
}
