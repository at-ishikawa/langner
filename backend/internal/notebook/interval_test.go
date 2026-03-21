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
	}{
		{
			name:      "default returns SM2",
			algorithm: "modified_sm2",
		},
		{
			name:      "empty string returns SM2",
			algorithm: "",
		},
		{
			name:      "fixed returns FixedLevelCalculator",
			algorithm: "fixed",
		},
		{
			name:      "unknown algorithm returns SM2",
			algorithm: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewIntervalCalculator(tt.algorithm, nil)
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
		wantMinDays int
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

func TestFixedLevelCalculator_CalculateInterval(t *testing.T) {
	// Default intervals: [1, 3, 7, 14, 30, 60, 120, 365]
	// correct (q >= 3): level += 1, wrong (q < 3): level -= 1
	tests := []struct {
		name     string
		logs     []LearningRecord
		quality  int
		wantDays int
	}{
		{
			name:     "first correct: level 0->1 = 3 days",
			logs:     nil,
			quality:  4,
			wantDays: 3,
		},
		{
			name:     "first wrong: level stays 0 = 1 day",
			logs:     nil,
			quality:  1,
			wantDays: 1,
		},
		{
			name: "two correct: level 1->2 = 7 days",
			logs: []LearningRecord{
				{Quality: 4},
			},
			quality:  4,
			wantDays: 7,
		},
		{
			name: "three correct: level 2->3 = 14 days",
			logs: []LearningRecord{
				{Quality: 4},
				{Quality: 4},
			},
			quality:  4,
			wantDays: 14,
		},
		{
			name: "slow correct still advances: level 0->1 = 3 days",
			logs: nil,
			quality:  3,
			wantDays: 3,
		},
		{
			name: "correct then wrong then correct: level 1->0->1 = 3 days",
			logs: []LearningRecord{
				// Newest first
				{Quality: 1},
				{Quality: 4},
			},
			quality:  4,
			wantDays: 3,
		},
		{
			name: "eight correct answers caps at max level: 365 days",
			logs: []LearningRecord{
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
			},
			quality:  4,
			wantDays: 365,
		},
		{
			name: "at max level, another correct stays at max: 365 days",
			logs: []LearningRecord{
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
				{Quality: 4},
			},
			quality:  4,
			wantDays: 365,
		},
		{
			name: "multiple wrongs clamp at level 0: 1 day",
			logs: []LearningRecord{
				{Quality: 1},
				{Quality: 1},
			},
			quality:  1,
			wantDays: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := &FixedLevelCalculator{}
			gotDays, gotEF := calc.CalculateInterval(tt.logs, tt.quality, 0)
			assert.Equal(t, tt.wantDays, gotDays)
			assert.Equal(t, 0.0, gotEF)
		})
	}
}

func TestFixedLevelCalculator_CalculateIntervalCustom(t *testing.T) {
	calc := &FixedLevelCalculator{Intervals: []int{1, 7, 30}}

	tests := []struct {
		name     string
		logs     []LearningRecord
		quality  int
		wantDays int
	}{
		{
			name:     "first correct with custom intervals: 7 days",
			logs:     nil,
			quality:  4,
			wantDays: 7,
		},
		{
			name: "two correct with custom intervals: 30 days",
			logs: []LearningRecord{
				{Quality: 4},
			},
			quality:  4,
			wantDays: 30,
		},
		{
			name: "caps at max custom level",
			logs: []LearningRecord{
				{Quality: 4},
				{Quality: 4},
			},
			quality:  4,
			wantDays: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDays, _ := calc.CalculateInterval(tt.logs, tt.quality, 0)
			assert.Equal(t, tt.wantDays, gotDays)
		})
	}
}

func TestFixedLevelCalculator_RecalculateAll(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		logs     []LearningRecord
		validate func(t *testing.T, logs []LearningRecord)
	}{
		{
			name: "empty logs",
			logs: nil,
		},
		{
			name: "single correct: level 1 = 3 days",
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 3, logs[0].IntervalDays)
			},
		},
		{
			name: "three correct: intervals grow through levels",
			logs: []LearningRecord{
				// Newest first
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 5)}},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 1)}},
				{Quality: 4, LearnedAt: Date{Time: baseTime}},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				// Oldest to newest: level 1=3, level 2=7, level 3=14
				// Logs returned newest first
				assert.Equal(t, 14, logs[0].IntervalDays) // newest
				assert.Equal(t, 7, logs[1].IntervalDays)
				assert.Equal(t, 3, logs[2].IntervalDays) // oldest
			},
		},
		{
			name: "wrong answer drops level",
			logs: []LearningRecord{
				// Newest first
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 3)}},
				{Quality: 1, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 2)}},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 1)}},
				{Quality: 4, LearnedAt: Date{Time: baseTime}},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				// Oldest to newest: q=4 level 1=3, q=4 level 2=7, q=1 level 1=3, q=4 level 2=7
				assert.Equal(t, 7, logs[0].IntervalDays) // newest
				assert.Equal(t, 3, logs[1].IntervalDays) // wrong
				assert.Equal(t, 7, logs[2].IntervalDays)
				assert.Equal(t, 3, logs[3].IntervalDays) // oldest
			},
		},
		{
			name: "preserves override interval",
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}, OverrideInterval: 100},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 100, logs[0].IntervalDays)
			},
		},
		{
			name: "assigns quality from status when zero",
			logs: []LearningRecord{
				{Quality: 0, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusMisunderstood},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, int(QualityWrong), logs[0].Quality)
				assert.Equal(t, 1, logs[0].IntervalDays) // wrong: level -1 clamped to 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := &FixedLevelCalculator{}
			ef, logs := calc.RecalculateAll(tt.logs)
			assert.Equal(t, 0.0, ef)
			if tt.validate != nil {
				tt.validate(t, logs)
			}
		})
	}
}
