// Package server provides Connect RPC handlers for the quiz service.
package server

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
)

// QuizHandler implements the QuizServiceHandler interface.
type QuizHandler struct {
	apiv1connect.UnimplementedQuizServiceHandler
}

// NewQuizHandler creates a new QuizHandler.
func NewQuizHandler() *QuizHandler {
	return &QuizHandler{}
}

// GetQuizOptions returns available notebooks with review counts.
func (h *QuizHandler) GetQuizOptions(
	ctx context.Context,
	req *connect.Request[apiv1.GetQuizOptionsRequest],
) (*connect.Response[apiv1.GetQuizOptionsResponse], error) {
	return connect.NewResponse(&apiv1.GetQuizOptionsResponse{}), nil
}

// StartQuiz starts a quiz session and returns flashcards for the selected notebooks.
func (h *QuizHandler) StartQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartQuizRequest],
) (*connect.Response[apiv1.StartQuizResponse], error) {
	if len(req.Msg.GetNotebookIds()) == 0 {
		return nil, newInvalidArgumentError("notebook_ids", "at least one notebook ID is required")
	}

	return connect.NewResponse(&apiv1.StartQuizResponse{}), nil
}

// SubmitAnswer grades a user's answer and updates learning history.
func (h *QuizHandler) SubmitAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitAnswerRequest],
) (*connect.Response[apiv1.SubmitAnswerResponse], error) {
	noteID := req.Msg.GetNoteId()
	answer := req.Msg.GetAnswer()

	if noteID == 0 {
		return nil, newInvalidArgumentError("note_id", "note_id is required")
	}
	if strings.TrimSpace(answer) == "" {
		return nil, newInvalidArgumentError("answer", "answer is required")
	}

	// No active session in stub -- always return not found
	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active quiz session", noteID))
}

func newInvalidArgumentError(field, description string) *connect.Error {
	err := connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s: %s", field, description))
	detail, detailErr := connect.NewErrorDetail(&errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{
			{Field: field, Description: description},
		},
	})
	if detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}
