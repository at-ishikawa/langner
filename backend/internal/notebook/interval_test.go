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
		{name: "default returns SM2", algorithm: "modified_sm2"},
		{name: "empty string returns SM2", algorithm: ""},
		{name: "fixed returns FixedLevelCalculator", algorithm: "fixed"},
		{name: "unknown algorithm returns SM2", algorithm: "unknown"},
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

	t.Run("empty logs", func(t *testing.T) {
		ef, logs := calc.RecalculateAll(nil)
		assert.InDelta(t, DefaultEasinessFactor, ef, 0.5)
		assert.Empty(t, logs)
	})

	t.Run("preserves override_interval", func(t *testing.T) {
		_, logs := calc.RecalculateAll([]LearningRecord{
			{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood, OverrideInterval: 30},
		})
		assert.Equal(t, 30, logs[0].IntervalDays)
	})
}

func TestSM2Calculator_DeriveEF(t *testing.T) {
	calc := &SM2Calculator{}
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty logs returns default EF", func(t *testing.T) {
		ef := calc.DeriveEF(nil)
		assert.InDelta(t, DefaultEasinessFactor, ef, 0.001)
	})

	t.Run("single correct answer", func(t *testing.T) {
		ef := calc.DeriveEF([]LearningRecord{
			{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood},
		})
		assert.Greater(t, ef, MinEasinessFactor)
	})

	t.Run("matches RecalculateAll EF", func(t *testing.T) {
		logs := []LearningRecord{
			{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 3)}, Status: LearnedStatusUnderstood},
			{Quality: 1, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 1)}, Status: LearnedStatusMisunderstood},
			{Quality: 4, LearnedAt: Date{Time: baseTime}, Status: LearnedStatusUnderstood},
		}
		derivedEF := calc.DeriveEF(logs)
		recalcEF, _ := calc.RecalculateAll(logs)
		assert.InDelta(t, recalcEF, derivedEF, 0.001)
	})
}

func TestFixedLevelCalculator_DeriveEF(t *testing.T) {
	calc := &FixedLevelCalculator{}
	ef := calc.DeriveEF([]LearningRecord{
		{Quality: 4},
	})
	assert.Equal(t, 0.0, ef)
}

func TestQualityToLevelDelta(t *testing.T) {
	assert.Equal(t, 2, qualityToLevelDelta(5))
	assert.Equal(t, 1, qualityToLevelDelta(4))
	assert.Equal(t, 1, qualityToLevelDelta(3))
	assert.Equal(t, -1, qualityToLevelDelta(1))
	assert.Equal(t, -1, qualityToLevelDelta(2))
}

func TestFixedLevelCalculator_LevelFromInterval(t *testing.T) {
	// Default intervals: [1, 7, 30, 90, 365, 1095, 1825]
	calc := &FixedLevelCalculator{}

	tests := []struct {
		interval  int
		wantLevel int
	}{
		{0, 0},
		{1, 0},
		{5, 0},
		{7, 1},
		{20, 1},
		{30, 2},
		{75, 2},
		{90, 3},
		{200, 3},
		{365, 4},
		{1000, 4},
		{1095, 5},
		{1825, 6},
		{5000, 6},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.wantLevel, calc.levelFromInterval(tt.interval), "interval %d", tt.interval)
	}
}

func TestFixedLevelCalculator_SnapToNextLevel(t *testing.T) {
	calc := &FixedLevelCalculator{}

	tests := []struct {
		interval int
		want     int
	}{
		{0, 1},
		{1, 1},
		{3, 7},
		{7, 7},
		{10, 30},
		{25, 30},
		{30, 30},
		{63, 90},
		{75, 90},
		{90, 90},
		{158, 365},
		{365, 365},
		{395, 1095},
		{1095, 1095},
		{1500, 1825},
		{1825, 1825},
		{9999, 1825},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, calc.snapToNextLevel(tt.interval), "interval %d", tt.interval)
	}
}

func TestFixedLevelCalculator_CalculateInterval(t *testing.T) {
	// CalculateInterval derives level from the most recent log's stored interval,
	// then advances by quality delta.
	tests := []struct {
		name     string
		logs     []LearningRecord
		quality  int
		wantDays int
	}{
		{
			name:     "first correct q=4: level 0->1 = 7 days",
			logs:     nil,
			quality:  4,
			wantDays: 7,
		},
		{
			name:     "first wrong: level stays 0 = 1 day",
			logs:     nil,
			quality:  1,
			wantDays: 1,
		},
		{
			name: "after interval=7, correct q=4: level 1->2 = 30 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 7},
			},
			quality:  4,
			wantDays: 30,
		},
		{
			name: "after interval=30, correct q=4: level 2->3 = 90 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 30},
			},
			quality:  4,
			wantDays: 90,
		},
		{
			name:     "first q=5: level 0->2 = 30 days (skips a level)",
			logs:     nil,
			quality:  5,
			wantDays: 30,
		},
		{
			name: "after interval=7, q=5: level 1->3 = 90 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 7},
			},
			quality:  5,
			wantDays: 90,
		},
		{
			name: "after interval=90, wrong q=1: level 3->2 = 30 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 90},
			},
			quality:  1,
			wantDays: 30,
		},
		{
			name: "SM-2 interval 75 then correct: level 2->3 = 90 (not 1825)",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 75},
			},
			quality:  4,
			wantDays: 90,
		},
		{
			name: "at max level, stays at max: 1825 days",
			logs: []LearningRecord{
				{Quality: 4, IntervalDays: 1825},
			},
			quality:  4,
			wantDays: 1825,
		},
		{
			name: "multiple wrongs clamp at level 0: 1 day",
			logs: []LearningRecord{
				{Quality: 1, IntervalDays: 1},
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
			name: "snaps SM-2 interval 3 -> 7",
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}, IntervalDays: 3},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 7, logs[0].IntervalDays)
			},
		},
		{
			name: "snaps SM-2 intervals to fixed levels",
			logs: []LearningRecord{
				// Newest first
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 5)}, IntervalDays: 63},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 1)}, IntervalDays: 10},
				{Quality: 4, LearnedAt: Date{Time: baseTime}, IntervalDays: 3},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 90, logs[0].IntervalDays)  // 63 -> 90
				assert.Equal(t, 30, logs[1].IntervalDays)  // 10 -> 30
				assert.Equal(t, 7, logs[2].IntervalDays)   // 3 -> 7
			},
		},
		{
			name: "word with 9 correct and interval 75 -> 90 (not 1825)",
			logs: []LearningRecord{
				// Newest first, 9 correct answers, last interval was 75
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 80)}, IntervalDays: 75},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 50)}, IntervalDays: 50},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 30)}, IntervalDays: 25},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 20)}, IntervalDays: 15},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 14)}, IntervalDays: 10},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 8)}, IntervalDays: 6},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 5)}, IntervalDays: 4},
				{Quality: 4, LearnedAt: Date{Time: baseTime.AddDate(0, 0, 2)}, IntervalDays: 3},
				{Quality: 4, LearnedAt: Date{Time: baseTime}, IntervalDays: 1},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				// Most recent: 75 -> snaps to 90
				assert.Equal(t, 90, logs[0].IntervalDays)
			},
		},
		{
			name: "preserves override interval",
			logs: []LearningRecord{
				{Quality: 4, LearnedAt: Date{Time: baseTime}, IntervalDays: 10, OverrideInterval: 100},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 100, logs[0].IntervalDays)
			},
		},
		{
			name: "wrong answer interval stays at 1",
			logs: []LearningRecord{
				{Quality: 1, LearnedAt: Date{Time: baseTime}, IntervalDays: 1},
			},
			validate: func(t *testing.T, logs []LearningRecord) {
				assert.Equal(t, 1, logs[0].IntervalDays)
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
