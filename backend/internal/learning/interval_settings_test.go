package learning

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestValidateIntervalSettings(t *testing.T) {
	tests := []struct {
		name    string
		in      IntervalSettings
		want    IntervalSettings
		wantErr bool
	}{
		{
			name: "fixed ascending ladder",
			in:   IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: []int{1, 7, 30, 90, 365}},
			want: IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: []int{1, 7, 30, 90, 365}},
		},
		{
			name: "modified_sm2 drops the ladder",
			in:   IntervalSettings{Algorithm: AlgorithmModifiedSM2, FixedIntervals: []int{3, 1}},
			want: IntervalSettings{Algorithm: AlgorithmModifiedSM2},
		},
		{name: "unknown algorithm", in: IntervalSettings{Algorithm: "weekly"}, wantErr: true},
		{name: "empty algorithm", in: IntervalSettings{}, wantErr: true},
		{name: "fixed without intervals", in: IntervalSettings{Algorithm: AlgorithmFixed}, wantErr: true},
		{name: "repeated interval", in: IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: []int{1, 7, 7}}, wantErr: true},
		{name: "descending", in: IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: []int{7, 1}}, wantErr: true},
		{name: "zero days", in: IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: []int{0, 7}}, wantErr: true},
		{name: "too long an interval", in: IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: []int{1, 3651}}, wantErr: true},
		{
			name:    "too many intervals",
			in:      IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateIntervalSettings(tt.in)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidIntervalSettings)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewIntervalSchedules_Default(t *testing.T) {
	tests := []struct {
		name    string
		quizCfg config.QuizConfig
		want    IntervalSettings
	}{
		{
			name:    "fixed with a ladder",
			quizCfg: config.QuizConfig{Algorithm: "fixed", FixedIntervals: []int{1, 3, 9}},
			want:    IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: []int{1, 3, 9}},
		},
		{
			name:    "fixed without a ladder uses the built-in one",
			quizCfg: config.QuizConfig{Algorithm: "fixed"},
			want:    IntervalSettings{Algorithm: AlgorithmFixed, FixedIntervals: notebook.DefaultFixedIntervals},
		},
		{
			name:    "modified_sm2",
			quizCfg: config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 3}},
			want:    IntervalSettings{Algorithm: AlgorithmModifiedSM2},
		},
		{
			name: "unset algorithm is modified_sm2, like the calculator",
			want: IntervalSettings{Algorithm: AlgorithmModifiedSM2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NewIntervalSchedules(nil, tt.quizCfg).Default())
		})
	}
}

func TestIntervalSchedules_WithoutDatabase(t *testing.T) {
	ctx := context.Background()
	schedules := NewIntervalSchedules(nil, config.QuizConfig{Algorithm: "fixed", FixedIntervals: []int{2, 4}})

	got, isDefault, err := schedules.Get(ctx, 42)
	require.NoError(t, err)
	assert.True(t, isDefault)
	assert.Equal(t, schedules.Default(), got)

	calc, err := schedules.CalculatorFor(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, &notebook.FixedLevelCalculator{Intervals: []int{2, 4}}, calc)

	assert.ErrorIs(t, schedules.Save(ctx, 42, IntervalSettings{Algorithm: AlgorithmModifiedSM2}), ErrIntervalSettingsUnavailable)
	assert.ErrorIs(t, schedules.Save(ctx, 42, IntervalSettings{Algorithm: "weekly"}), ErrInvalidIntervalSettings, "input is validated first")
	assert.NoError(t, schedules.Reset(ctx, 42))
}

func TestIntervalSchedules_WithCalculator(t *testing.T) {
	schedules := NewIntervalSchedules(nil, config.QuizConfig{Algorithm: "fixed", FixedIntervals: []int{2, 4}})
	preloaded := &notebook.SM2Calculator{}
	ctx := context.WithValue(context.Background(), calculatorKey{}, preloadedCalculator{userID: 7, calculator: preloaded})

	got, err := schedules.CalculatorFor(ctx, 7)
	require.NoError(t, err)
	assert.Same(t, preloaded, got, "the request's preloaded calculator is reused for its user")

	got, err = schedules.CalculatorFor(ctx, 8)
	require.NoError(t, err)
	assert.Equal(t, &notebook.FixedLevelCalculator{Intervals: []int{2, 4}}, got, "another user is looked up, never served the preload")

	got, err = schedules.CalculatorFor(schedules.WithCalculator(context.Background(), 9), 9)
	require.NoError(t, err)
	assert.Equal(t, &notebook.FixedLevelCalculator{Intervals: []int{2, 4}}, got)
}
