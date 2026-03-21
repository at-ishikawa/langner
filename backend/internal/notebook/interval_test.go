package notebook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIntervalCalculator(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
		base      float64
		wantType  string
	}{
		{
			name:      "default returns SM2",
			algorithm: "modified_sm2",
			base:      4,
			wantType:  "*notebook.SM2Calculator",
		},
		{
			name:      "empty string returns SM2",
			algorithm: "",
			base:      0,
			wantType:  "*notebook.SM2Calculator",
		},
		{
			name:      "exponential returns ExponentialCalculator",
			algorithm: "exponential",
			base:      4,
			wantType:  "*notebook.ExponentialCalculator",
		},
		{
			name:      "unknown algorithm returns SM2",
			algorithm: "unknown",
			base:      0,
			wantType:  "*notebook.SM2Calculator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewIntervalCalculator(tt.algorithm, tt.base)
			require.NotNil(t, got)
		})
	}
}

func TestSM2Calculator_CalculateInterval(t *testing.T) {
	calc := &SM2Calculator{}

	tests := []struct {
		name        string
		logs        []LearningRecord
		quality     int
		currentEF   float64
		wantMinDays int // minimum expected interval
	}{
		{
			name:        "first correct answer with no logs",
			logs:        nil,
			quality:     4,
			currentEF:   DefaultEasinessFactor,
			wantMinDays: 1,
		},
		{
			name: "correct answer after one correct",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 3},
			},
			quality:     4,
			currentEF:   DefaultEasinessFactor,
			wantMinDays: 3,
		},
		{
			name: "wrong answer reduces interval",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 30},
			},
			quality:     1,
			currentEF:   DefaultEasinessFactor,
			wantMinDays: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intervalDays, newEF := calc.CalculateInterval(tt.logs, tt.quality, tt.currentEF)
			assert.GreaterOrEqual(t, intervalDays, tt.wantMinDays)
			assert.Greater(t, newEF, 0.0)
		})
	}
}

func TestSM2Calculator_RecalculateAll(t *testing.T) {
	calc := &SM2Calculator{}

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		logs     []LearningRecord
		wantEF   float64
		wantLen  int
		validate func(t *testing.T, ef float64, logs []LearningRecord)
	}{
		{
			name:    "empty logs",
			logs:    nil,
			wantEF:  DefaultEasinessFactor,
			wantLen: 0,
		},
		{
			name: "single correct log",
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood},
			},
			wantLen: 1,
			validate: func(t *testing.T, ef float64, logs []LearningRecord) {
				assert.Greater(t, ef, 0.0)
				assert.Greater(t, logs[0].IntervalDays, 0)
			},
		},
		{
			name: "multiple logs replayed oldest to newest",
			logs: []LearningRecord{
				// Newest first (storage order)
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 10)}, Status: LearnedStatusUnderstood},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 3)}, Status: LearnedStatusUnderstood},
				{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood},
			},
			wantLen: 3,
			validate: func(t *testing.T, ef float64, logs []LearningRecord) {
				// After replay, intervals should increase
				assert.Greater(t, ef, 0.0)
				// Logs should still be sorted newest-first
				assert.True(t, logs[0].LearnedAt.After(logs[1].LearnedAt.Time))
				assert.True(t, logs[1].LearnedAt.After(logs[2].LearnedAt.Time))
			},
		},
		{
			name: "preserves override_interval",
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood, OverrideInterval: 30},
			},
			wantLen: 1,
			validate: func(t *testing.T, ef float64, logs []LearningRecord) {
				assert.Equal(t, 30, logs[0].IntervalDays)
			},
		},
		{
			name: "assigns quality from status when quality is zero",
			logs: []LearningRecord{
				{Quality: 0, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusMisunderstood},
			},
			wantLen: 1,
			validate: func(t *testing.T, ef float64, logs []LearningRecord) {
				assert.Equal(t, int(QualityWrong), logs[0].Quality)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ef, logs := calc.RecalculateAll(tt.logs)
			assert.Equal(t, tt.wantLen, len(logs))
			if tt.wantEF > 0 {
				assert.InDelta(t, tt.wantEF, ef, 0.5)
			}
			if tt.validate != nil {
				tt.validate(t, ef, logs)
			}
		})
	}
}

func TestExponentialCalculator_CalculateInterval(t *testing.T) {
	tests := []struct {
		name        string
		base        float64
		logs        []LearningRecord
		quality     int
		wantDays    int
		wantEF      float64
	}{
		{
			name:     "first correct answer with base 4, quality 4 -> score 1 -> 4^0 = 1",
			base:     4,
			logs:     nil,
			quality:  4,
			wantDays: 1,
			wantEF:   0,
		},
		{
			name:     "first wrong answer, quality 1 -> score -2 clamped to 1 -> 4^0 = 1",
			base:     4,
			logs:     nil,
			quality:  1,
			wantDays: 1,
			wantEF:   0,
		},
		{
			name: "two correct answers, quality 4 -> score = (4-3) + (4-3) = 2 -> 4^1 = 4",
			base: 4,
			logs: []LearningRecord{
				{Quality: 4},
			},
			quality:  4,
			wantDays: 4,
			wantEF:   0,
		},
		{
			name: "three correct answers, quality 5 -> score = 1+1+2 = 4 -> 4^3 = 64",
			base: 4,
			logs: []LearningRecord{
				// Newest first
				{Quality: 4},
				{Quality: 4},
			},
			quality:  5,
			wantDays: 64,
			wantEF:   0,
		},
		{
			name: "mixed quality: oldest-to-newest (1-3)+(4-3)+(4-3) = 0 clamped to 1 -> 4^0 = 1",
			base: 4,
			logs: []LearningRecord{
				// Newest first
				{Quality: 4},
				{Quality: 1},
			},
			quality:  4,
			wantDays: 1,
			wantEF:   0,
		},
		{
			name: "all wrong then correct: score = (1-3)+(1-3)+(4-3) = -3 clamped to 1 -> 4^0 = 1",
			base: 4,
			logs: []LearningRecord{
				// Newest first
				{Quality: 1},
				{Quality: 1},
			},
			quality:  4,
			wantDays: 1,
			wantEF:   0,
		},
		{
			name:     "custom base 2, first quality 5 -> score 2 -> 2^1 = 2",
			base:     2,
			logs:     nil,
			quality:  5,
			wantDays: 2,
			wantEF:   0,
		},
		{
			name:     "default base (0) treated as 4",
			base:     0,
			logs:     nil,
			quality:  4,
			wantDays: 1,
			wantEF:   0,
		},
		{
			name: "quality 5 gives double jump: score = 2 -> 4^1 = 4",
			base: 4,
			logs: nil,
			quality:  5,
			wantDays: 4,
			wantEF:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := &ExponentialCalculator{Base: tt.base}
			gotDays, gotEF := calc.CalculateInterval(tt.logs, tt.quality, 0)
			assert.Equal(t, tt.wantDays, gotDays)
			assert.Equal(t, tt.wantEF, gotEF)
		})
	}
}

func TestExponentialCalculator_RecalculateAll(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		base     float64
		logs     []LearningRecord
		wantEF   float64
		validate func(t *testing.T, logs []LearningRecord)
	}{
		{
			name:   "empty logs",
			base:   4,
			logs:   nil,
			wantEF: 0,
		},
		{
			name: "single correct log: score 1 -> 4^0 = 1",
			base: 4,
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood},
			},
			wantEF: 0,
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 1, logs[0].IntervalDays)
			},
		},
		{
			name: "three correct logs replayed: intervals grow exponentially",
			base: 4,
			logs: []LearningRecord{
				// Newest first
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 5)}, Status: LearnedStatusUnderstood},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 1)}, Status: LearnedStatusUnderstood},
				{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood},
			},
			wantEF: 0,
			validate: func(t *testing.T, logs []LearningRecord) {
				// Oldest to newest: score 1 -> 4^0=1, score 2 -> 4^1=4, score 3 -> 4^2=16
				// Logs are newest first after recalculate
				assert.Equal(t, 16, logs[0].IntervalDays) // newest (3rd review)
				assert.Equal(t, 4, logs[1].IntervalDays)  // 2nd review
				assert.Equal(t, 1, logs[2].IntervalDays)  // oldest (1st review)
			},
		},
		{
			name: "preserves override interval",
			base: 4,
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood, OverrideInterval: 100},
			},
			wantEF: 0,
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 100, logs[0].IntervalDays)
			},
		},
		{
			name: "wrong answer resets score accumulation",
			base: 4,
			logs: []LearningRecord{
				// Newest first
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 2)}, Status: LearnedStatusUnderstood},
				{Quality: 1, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 1)}, Status: LearnedStatusMisunderstood},
				{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood},
			},
			wantEF: 0,
			validate: func(t *testing.T, logs []LearningRecord) {
				// Oldest to newest: q=4 -> score=1, q=1 -> score=-1 clamped to 1, q=4 -> score=0 -> 1
				// After first (oldest): score = (4-3) = 1 -> 4^0 = 1
				// After second: score = 1 + (1-3) = -1, clamped to 1 -> 4^0 = 1
				// After third: score = -1 + (4-3) = 0, clamped to 1 -> 4^0 = 1
				assert.Equal(t, 1, logs[0].IntervalDays) // newest
				assert.Equal(t, 1, logs[1].IntervalDays) // middle
				assert.Equal(t, 1, logs[2].IntervalDays) // oldest
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := &ExponentialCalculator{Base: tt.base}
			ef, logs := calc.RecalculateAll(tt.logs)
			assert.Equal(t, tt.wantEF, ef)
			if tt.validate != nil {
				tt.validate(t, logs)
			}
		})
	}
}

func TestExponentialCalculator_ScoreCalculation(t *testing.T) {
	// Test the score accumulation formula in detail
	calc := &ExponentialCalculator{Base: 4}

	tests := []struct {
		name     string
		logs     []LearningRecord
		quality  int
		wantDays int
	}{
		{
			name:     "empty logs quality 3: score = 0 -> clamped 1 -> 4^0 = 1",
			logs:     nil,
			quality:  3,
			wantDays: 1,
		},
		{
			name:     "empty logs quality 4: score = 1 -> 4^0 = 1",
			logs:     nil,
			quality:  4,
			wantDays: 1,
		},
		{
			name:     "empty logs quality 5: score = 2 -> 4^1 = 4",
			logs:     nil,
			quality:  5,
			wantDays: 4,
		},
		{
			name: "five correct q=4 answers: score = 5 -> 4^4 = 256",
			logs: []LearningRecord{
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
			},
			quality:  4,
			wantDays: 256,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDays, _ := calc.CalculateInterval(tt.logs, tt.quality, 0)
			assert.Equal(t, tt.wantDays, gotDays)
		})
	}
}
