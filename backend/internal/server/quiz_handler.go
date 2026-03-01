// Package server provides Connect RPC handlers for the quiz service.
package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// QuizHandler implements the QuizServiceHandler interface.
type QuizHandler struct {
	apiv1connect.UnimplementedQuizServiceHandler

	svc       *quiz.Service
	mu        sync.Mutex
	noteStore map[int64]quiz.Card
	nextID    int64
}

// NewQuizHandler creates a new QuizHandler.
func NewQuizHandler(svc *quiz.Service) *QuizHandler {
	return &QuizHandler{
		svc:       svc,
		noteStore: make(map[int64]quiz.Card),
		nextID:    1,
	}
}

// GetQuizOptions returns available notebooks with review counts.
func (h *QuizHandler) GetQuizOptions(
	ctx context.Context,
	req *connect.Request[apiv1.GetQuizOptionsRequest],
) (*connect.Response[apiv1.GetQuizOptionsResponse], error) {
	summaries, err := h.svc.LoadNotebookSummaries()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load notebook summaries: %w", err))
	}

	var protoSummaries []*apiv1.NotebookSummary
	for _, s := range summaries {
		protoSummaries = append(protoSummaries, &apiv1.NotebookSummary{
			NotebookId:  s.NotebookID,
			Name:        s.Name,
			ReviewCount: int32(s.ReviewCount),
		})
	}

	sort.Slice(protoSummaries, func(i, j int) bool {
		return protoSummaries[i].GetNotebookId() < protoSummaries[j].GetNotebookId()
	})

	return connect.NewResponse(&apiv1.GetQuizOptionsResponse{
		Notebooks: protoSummaries,
	}), nil
}

// StartQuiz starts a quiz session and returns flashcards for the selected notebooks.
func (h *QuizHandler) StartQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartQuizRequest],
) (*connect.Response[apiv1.StartQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	cards, err := h.svc.LoadCards(req.Msg.GetNotebookIds(), req.Msg.GetIncludeUnstudied())
	if err != nil {
		var notFoundErr *quiz.NotFoundError
		if errors.As(err, &notFoundErr) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load cards: %w", err))
	}

	localStore := make(map[int64]quiz.Card)
	var nextID int64 = 1

	var flashcards []*apiv1.Flashcard
	for _, card := range cards {
		noteID := nextID
		nextID++
		localStore[noteID] = card

		var examples []*apiv1.Example
		for _, ex := range card.Examples {
			examples = append(examples, &apiv1.Example{
				Text:    ex.Text,
				Speaker: ex.Speaker,
			})
		}

		flashcards = append(flashcards, &apiv1.Flashcard{
			NoteId:   noteID,
			Entry:    card.Entry,
			Examples: examples,
		})
	}

	h.mu.Lock()
	h.noteStore = localStore
	h.nextID = nextID
	h.mu.Unlock()

	return connect.NewResponse(&apiv1.StartQuizResponse{
		Flashcards: flashcards,
	}), nil
}

// SubmitAnswer grades a user's answer and updates learning history.
func (h *QuizHandler) SubmitAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitAnswerRequest],
) (*connect.Response[apiv1.SubmitAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	noteID := req.Msg.GetNoteId()
	answer := req.Msg.GetAnswer()

	h.mu.Lock()
	card, ok := h.noteStore[noteID]
	h.mu.Unlock()

	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active quiz session", noteID))
	}

	grade, err := h.svc.GradeNotebookAnswer(ctx, card, answer, req.Msg.GetResponseTimeMs())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
	}

	if err := h.svc.SaveResult(card, grade, req.Msg.GetResponseTimeMs()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
	}

	return connect.NewResponse(&apiv1.SubmitAnswerResponse{
		Correct: grade.Correct,
		Meaning: card.Meaning,
		Reason:  grade.Reason,
	}), nil
}

func validateRequest(msg proto.Message) *connect.Error {
	if err := protovalidate.Validate(msg); err != nil {
		connectErr := connect.NewError(connect.CodeInvalidArgument, err)
		var valErr *protovalidate.ValidationError
		if errors.As(err, &valErr) {
			var fieldViolations []*errdetails.BadRequest_FieldViolation
			for _, v := range valErr.Violations {
				fieldViolations = append(fieldViolations, &errdetails.BadRequest_FieldViolation{
					Field:       protovalidate.FieldPathString(v.Proto.GetField()),
					Description: v.Proto.GetMessage(),
				})
			}
			if detail, detailErr := connect.NewErrorDetail(&errdetails.BadRequest{
				FieldViolations: fieldViolations,
			}); detailErr == nil {
				connectErr.AddDetail(detail)
			}
		}
		return connectErr
	}
	return nil
}
