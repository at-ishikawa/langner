package server

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/learning"
)

// GetIntervalSettings returns the signed-in user's review schedule and the
// server default.
func (h *QuizHandler) GetIntervalSettings(ctx context.Context, _ *connect.Request[apiv1.GetIntervalSettingsRequest]) (*connect.Response[apiv1.GetIntervalSettingsResponse], error) {
	userID, _ := auth.UserIDFromContext(ctx)
	settings, isDefault, defaults, err := h.intervalSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetIntervalSettingsResponse{
		Settings: settings, IsDefault: isDefault, DefaultSettings: defaults,
	}), nil
}

// UpdateIntervalSettings saves (or, with reset_to_default, removes) the
// signed-in user's review schedule. It schedules future answers only.
func (h *QuizHandler) UpdateIntervalSettings(ctx context.Context, req *connect.Request[apiv1.UpdateIntervalSettingsRequest]) (*connect.Response[apiv1.UpdateIntervalSettingsResponse], error) {
	userID, _ := auth.UserIDFromContext(ctx)
	schedules := h.svc.IntervalSchedules()
	if req.Msg.GetResetToDefault() {
		if err := schedules.Reset(ctx, userID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	} else {
		err := schedules.Save(ctx, userID, intervalSettingsFromProto(req.Msg.GetSettings()))
		if errors.Is(err, learning.ErrInvalidIntervalSettings) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if errors.Is(err, learning.ErrIntervalSettingsUnavailable) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	settings, isDefault, defaults, err := h.intervalSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.UpdateIntervalSettingsResponse{
		Settings: settings, IsDefault: isDefault, DefaultSettings: defaults,
	}), nil
}

func (h *QuizHandler) intervalSettings(ctx context.Context, userID int64) (settings *apiv1.IntervalSettings, isDefault bool, defaults *apiv1.IntervalSettings, err error) {
	schedules := h.svc.IntervalSchedules()
	got, isDefault, err := schedules.Get(ctx, userID)
	if err != nil {
		return nil, false, nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get review schedule: %w", err))
	}
	return intervalSettingsToProto(got), isDefault, intervalSettingsToProto(schedules.Default()), nil
}

func intervalSettingsToProto(s learning.IntervalSettings) *apiv1.IntervalSettings {
	out := &apiv1.IntervalSettings{Algorithm: s.Algorithm}
	for _, d := range s.FixedIntervals {
		out.FixedIntervals = append(out.FixedIntervals, int32(d))
	}
	return out
}

func intervalSettingsFromProto(s *apiv1.IntervalSettings) learning.IntervalSettings {
	out := learning.IntervalSettings{Algorithm: s.GetAlgorithm()}
	for _, d := range s.GetFixedIntervals() {
		out.FixedIntervals = append(out.FixedIntervals, int(d))
	}
	return out
}
