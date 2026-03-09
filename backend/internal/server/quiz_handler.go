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

	svc           *quiz.Service
	mu            sync.Mutex
	noteStore     map[int64]quiz.Card
	reverseStore  map[int64]quiz.ReverseCard
	freeformCards []quiz.FreeformCard
	nextID        int64
}

// NewQuizHandler creates a new QuizHandler.
func NewQuizHandler(svc *quiz.Service) *QuizHandler {
	return &QuizHandler{
		svc:          svc,
		noteStore:    make(map[int64]quiz.Card),
		reverseStore: make(map[int64]quiz.ReverseCard),
		nextID:       1,
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

	sort.Slice(summaries, func(i, j int) bool {
		di, dj := summaries[i].LatestStoryDate, summaries[j].LatestStoryDate
		if !di.Equal(dj) {
			return di.After(dj)
		}
		return summaries[i].NotebookID < summaries[j].NotebookID
	})

	protoSummaries := make([]*apiv1.NotebookSummary, 0, len(summaries))
	for _, s := range summaries {
		protoSummaries = append(protoSummaries, &apiv1.NotebookSummary{
			NotebookId:  s.NotebookID,
			Name:        s.Name,
			ReviewCount: int32(s.ReviewCount),
			Kind:        s.Kind,
		})
	}

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

// StartReverseQuiz starts a reverse quiz session.
func (h *QuizHandler) StartReverseQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartReverseQuizRequest],
) (*connect.Response[apiv1.StartReverseQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	cards, err := h.svc.LoadReverseCards(req.Msg.GetNotebookIds(), req.Msg.GetListMissingContext())
	if err != nil {
		var notFoundErr *quiz.NotFoundError
		if errors.As(err, &notFoundErr) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load reverse cards: %w", err))
	}

	localStore := make(map[int64]quiz.ReverseCard)
	var nextID int64 = 1

	var flashcards []*apiv1.ReverseFlashcard
	for _, card := range cards {
		noteID := nextID
		nextID++
		localStore[noteID] = card

		var contexts []*apiv1.ContextSentence
		for _, ctx := range card.Contexts {
			contexts = append(contexts, &apiv1.ContextSentence{
				Context:       ctx.Context,
				MaskedContext: ctx.MaskedContext,
			})
		}

		flashcards = append(flashcards, &apiv1.ReverseFlashcard{
			NoteId:       noteID,
			Meaning:      card.Meaning,
			Contexts:     contexts,
			NotebookName: card.NotebookName,
			StoryTitle:   card.StoryTitle,
			SceneTitle:   card.SceneTitle,
		})
	}

	h.mu.Lock()
	h.reverseStore = localStore
	h.nextID = nextID
	h.mu.Unlock()

	return connect.NewResponse(&apiv1.StartReverseQuizResponse{
		Flashcards: flashcards,
	}), nil
}

// SubmitReverseAnswer grades a reverse quiz answer.
func (h *QuizHandler) SubmitReverseAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitReverseAnswerRequest],
) (*connect.Response[apiv1.SubmitReverseAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	noteID := req.Msg.GetNoteId()
	answer := req.Msg.GetAnswer()

	h.mu.Lock()
	card, ok := h.reverseStore[noteID]
	h.mu.Unlock()

	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active reverse quiz session", noteID))
	}

	grade, err := h.svc.GradeReverseAnswer(ctx, card, answer, req.Msg.GetResponseTimeMs())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
	}

	if err := h.svc.SaveReverseResult(card, grade, req.Msg.GetResponseTimeMs()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
	}

	var contexts []string
	for _, ctx := range card.Contexts {
		contexts = append(contexts, ctx.Context)
	}

	return connect.NewResponse(&apiv1.SubmitReverseAnswerResponse{
		Correct:    grade.Correct,
		Expression: card.Expression,
		Meaning:    card.Meaning,
		Reason:     grade.Reason,
		Contexts:   contexts,
	}), nil
}

// StartFreeformQuiz starts a freeform quiz session.
func (h *QuizHandler) StartFreeformQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartFreeformQuizRequest],
) (*connect.Response[apiv1.StartFreeformQuizResponse], error) {
	cards, err := h.svc.LoadAllWords()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load all words: %w", err))
	}

	h.mu.Lock()
	h.freeformCards = cards
	h.mu.Unlock()

	return connect.NewResponse(&apiv1.StartFreeformQuizResponse{
		WordCount: int32(len(cards)),
	}), nil
}

// SubmitFreeformAnswer grades a freeform quiz answer.
func (h *QuizHandler) SubmitFreeformAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitFreeformAnswerRequest],
) (*connect.Response[apiv1.SubmitFreeformAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	h.mu.Lock()
	cards := h.freeformCards
	h.mu.Unlock()

	grade, err := h.svc.GradeFreeformAnswer(ctx, req.Msg.GetWord(), req.Msg.GetMeaning(), req.Msg.GetResponseTimeMs(), cards)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
	}

	if grade.MatchedCard != nil {
		if err := h.svc.SaveFreeformResult(*grade.MatchedCard, grade, req.Msg.GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
		}
	}

	return connect.NewResponse(&apiv1.SubmitFreeformAnswerResponse{
		Correct:      grade.Correct,
		Word:         grade.Word,
		Meaning:      grade.Meaning,
		Reason:       grade.Reason,
		Context:      grade.Context,
		NotebookName: grade.NotebookName,
	}), nil
}
