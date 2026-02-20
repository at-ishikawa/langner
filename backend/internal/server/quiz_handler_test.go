package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
)

func TestQuizHandler_GetQuizOptions(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "returns empty notebooks when no data exists",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewQuizHandler()

			resp, err := handler.GetQuizOptions(
				context.Background(),
				connect.NewRequest(&apiv1.GetQuizOptionsRequest{}),
			)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Empty(t, resp.Msg.GetNotebooks())
		})
	}
}

func TestQuizHandler_StartQuiz(t *testing.T) {
	tests := []struct {
		name        string
		notebookIDs []string
		wantCode    connect.Code
		wantErr     bool
	}{
		{
			name:        "returns INVALID_ARGUMENT when no notebook IDs provided",
			notebookIDs: nil,
			wantCode:    connect.CodeInvalidArgument,
			wantErr:     true,
		},
		{
			name:        "returns INVALID_ARGUMENT when empty notebook IDs slice",
			notebookIDs: []string{},
			wantCode:    connect.CodeInvalidArgument,
			wantErr:     true,
		},
		{
			name:        "returns empty flashcards for valid notebook IDs",
			notebookIDs: []string{"test-notebook"},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewQuizHandler()

			resp, err := handler.StartQuiz(
				context.Background(),
				connect.NewRequest(&apiv1.StartQuizRequest{
					NotebookIds: tt.notebookIDs,
				}),
			)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, resp)
				connectErr, ok := err.(*connect.Error)
				require.True(t, ok)
				assert.Equal(t, tt.wantCode, connectErr.Code())
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Empty(t, resp.Msg.GetFlashcards())
		})
	}
}

func TestQuizHandler_SubmitAnswer(t *testing.T) {
	tests := []struct {
		name     string
		noteID   int64
		answer   string
		wantCode connect.Code
		wantErr  bool
	}{
		{
			name:     "returns INVALID_ARGUMENT when note_id is zero",
			noteID:   0,
			answer:   "some answer",
			wantCode: connect.CodeInvalidArgument,
			wantErr:  true,
		},
		{
			name:     "returns INVALID_ARGUMENT when answer is empty",
			noteID:   1,
			answer:   "",
			wantCode: connect.CodeInvalidArgument,
			wantErr:  true,
		},
		{
			name:     "returns INVALID_ARGUMENT when answer is whitespace only",
			noteID:   1,
			answer:   "   ",
			wantCode: connect.CodeInvalidArgument,
			wantErr:  true,
		},
		{
			name:     "returns NOT_FOUND when note does not exist in session",
			noteID:   999,
			answer:   "some answer",
			wantCode: connect.CodeNotFound,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewQuizHandler()

			resp, err := handler.SubmitAnswer(
				context.Background(),
				connect.NewRequest(&apiv1.SubmitAnswerRequest{
					NoteId:         tt.noteID,
					Answer:         tt.answer,
					ResponseTimeMs: 1000,
				}),
			)

			require.Error(t, err)
			assert.Nil(t, resp)
			connectErr, ok := err.(*connect.Error)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, connectErr.Code())
		})
	}
}

func TestNewInvalidArgumentError(t *testing.T) {
	connectErr := newInvalidArgumentError("field_name", "field is required")

	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	assert.Contains(t, connectErr.Message(), "field_name")
	assert.Greater(t, len(connectErr.Details()), 0)
}
