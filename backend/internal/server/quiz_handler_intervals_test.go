package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// Without a database there is nowhere per-user to keep a schedule: Get reports
// the config default and Update refuses a save (but validates first).
func TestQuizHandler_IntervalSettings_WithoutDatabase(t *testing.T) {
	svc := quiz.NewService(config.NotebooksConfig{}, nil, nil, nil, config.QuizConfig{Algorithm: "fixed", FixedIntervals: []int{1, 7, 30}})
	h := NewQuizHandler(svc)
	ctx := auth.WithUserID(context.Background(), 5)
	wantDefault := &apiv1.IntervalSettings{Algorithm: "fixed", FixedIntervals: []int32{1, 7, 30}}

	got, err := h.GetIntervalSettings(ctx, connect.NewRequest(&apiv1.GetIntervalSettingsRequest{}))
	require.NoError(t, err)
	assert.True(t, got.Msg.GetIsDefault())
	assert.Equal(t, wantDefault.String(), got.Msg.GetSettings().String())
	assert.Equal(t, wantDefault.String(), got.Msg.GetDefaultSettings().String())

	tests := []struct {
		name     string
		req      *apiv1.UpdateIntervalSettingsRequest
		wantCode connect.Code
	}{
		{
			name:     "invalid ladder",
			req:      &apiv1.UpdateIntervalSettingsRequest{Settings: &apiv1.IntervalSettings{Algorithm: "fixed", FixedIntervals: []int32{7, 1}}},
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:     "valid schedule but no database",
			req:      &apiv1.UpdateIntervalSettingsRequest{Settings: &apiv1.IntervalSettings{Algorithm: "modified_sm2"}},
			wantCode: connect.CodeFailedPrecondition,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.UpdateIntervalSettings(ctx, connect.NewRequest(tt.req))
			assert.Equal(t, tt.wantCode, connect.CodeOf(err))
		})
	}

	reset, err := h.UpdateIntervalSettings(ctx, connect.NewRequest(&apiv1.UpdateIntervalSettingsRequest{ResetToDefault: true}))
	require.NoError(t, err)
	assert.True(t, reset.Msg.GetIsDefault())
}
