// Package server provides Connect RPC handlers for the quiz service.
package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
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
			NotebookId:         s.NotebookID,
			Name:               s.Name,
			ReviewCount:        int32(s.ReviewCount),
			Kind:               s.Kind,
			ReverseReviewCount: int32(s.ReverseReviewCount),
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

	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Entry, notebook.QuizTypeNotebook)

	return connect.NewResponse(&apiv1.SubmitAnswerResponse{
		Correct:        grade.Correct,
		Meaning:        card.Meaning,
		Reason:         grade.Reason,
		WordDetail:     toProtoWordDetail(card.WordDetail),
		NextReviewDate: nextReviewDate,
		LearnedAt:      learnedAt,
	}), nil
}

func toProtoWordDetail(wd quiz.WordDetail) *apiv1.WordDetail {
	if wd.Origin == "" && wd.Pronunciation == "" && wd.PartOfSpeech == "" && len(wd.Synonyms) == 0 && len(wd.Antonyms) == 0 && wd.Memo == "" {
		return nil
	}
	return &apiv1.WordDetail{
		Origin:        wd.Origin,
		Pronunciation: wd.Pronunciation,
		PartOfSpeech:  wd.PartOfSpeech,
		Synonyms:      wd.Synonyms,
		Antonyms:      wd.Antonyms,
		Memo:          wd.Memo,
	}
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

	// Don't save synonym results — the frontend will offer a retry
	if grade.Classification != string(inference.ClassificationSynonym) {
		if err := h.svc.SaveReverseResult(card, grade, req.Msg.GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
		}
	}

	var contexts []string
	for _, ctx := range card.Contexts {
		contexts = append(contexts, ctx.Context)
	}

	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Expression, notebook.QuizTypeReverse)

	return connect.NewResponse(&apiv1.SubmitReverseAnswerResponse{
		Correct:        grade.Correct,
		Expression:     card.Expression,
		Meaning:        card.Meaning,
		Reason:         grade.Reason,
		Contexts:       contexts,
		WordDetail:     toProtoWordDetail(card.WordDetail),
		Classification: grade.Classification,
		NextReviewDate: nextReviewDate,
		LearnedAt:      learnedAt,
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

	nextReviewDates, err := h.svc.GetFreeformNextReviewDates(cards)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get next review dates: %w", err))
	}

	h.mu.Lock()
	h.freeformCards = cards
	h.mu.Unlock()

	seen := make(map[string]struct{}, len(cards)*2)
	expressions := make([]string, 0, len(cards))
	addExpr := func(expr string) {
		lower := strings.ToLower(expr)
		if expr != "" {
			if _, ok := seen[lower]; !ok {
				seen[lower] = struct{}{}
				expressions = append(expressions, expr)
			}
		}
	}
	for _, card := range cards {
		addExpr(card.Expression)
		addExpr(card.OriginalExpression)
	}

	return connect.NewResponse(&apiv1.StartFreeformQuizResponse{
		WordCount:                int32(len(cards)),
		Expressions:              expressions,
		ExpressionNextReviewDate: nextReviewDates,
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

	var learnedAt, nextReviewDate string
	if grade.MatchedCard != nil {
		learnedAt, nextReviewDate = h.svc.GetLatestLearnedInfo(grade.MatchedCard.NotebookName, grade.MatchedCard.Expression, notebook.QuizTypeFreeform)
	}

	return connect.NewResponse(&apiv1.SubmitFreeformAnswerResponse{
		Correct:      grade.Correct,
		Word:         grade.Word,
		Meaning:      grade.Meaning,
		Reason:       grade.Reason,
		Context:      grade.Context,
		NotebookName: grade.NotebookName,
		WordDetail: func() *apiv1.WordDetail {
			if grade.MatchedCard != nil {
				return toProtoWordDetail(grade.MatchedCard.WordDetail)
			}
			return nil
		}(),
		NextReviewDate: nextReviewDate,
		LearnedAt:      learnedAt,
	}), nil
}

// protoQuizTypeToNotebook converts a proto QuizType enum to a notebook QuizType string.
func protoQuizTypeToNotebook(qt apiv1.QuizType) notebook.QuizType {
	switch qt {
	case apiv1.QuizType_QUIZ_TYPE_STANDARD:
		return notebook.QuizTypeNotebook
	case apiv1.QuizType_QUIZ_TYPE_REVERSE:
		return notebook.QuizTypeReverse
	case apiv1.QuizType_QUIZ_TYPE_FREEFORM:
		return notebook.QuizTypeFreeform
	default:
		return notebook.QuizTypeNotebook
	}
}

// OverrideAnswer overrides a previously graded quiz answer.
func (h *QuizHandler) OverrideAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.OverrideAnswerRequest],
) (*connect.Response[apiv1.OverrideAnswerResponse], error) {
	noteID := req.Msg.GetNoteId()
	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	learnedAt := req.Msg.GetLearnedAt()

	// Look up the card to find notebook name and expression
	var notebookName, expression string

	h.mu.Lock()
	if card, ok := h.noteStore[noteID]; ok {
		notebookName = card.NotebookName
		expression = card.Entry
	} else if rcard, ok := h.reverseStore[noteID]; ok {
		notebookName = rcard.NotebookName
		expression = rcard.Expression
	}
	h.mu.Unlock()

	if notebookName == "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active quiz session", noteID))
	}

	var markCorrect *bool
	if req.Msg.MarkCorrect != nil {
		mc := req.Msg.GetMarkCorrect()
		markCorrect = &mc
	}

	oq, os, oid, oef, nnr, err := h.svc.OverrideAnswer(notebookName, expression, quizType, learnedAt, markCorrect, req.Msg.GetNextReviewDate())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("override answer: %w", err))
	}

	return connect.NewResponse(&apiv1.OverrideAnswerResponse{
		NextReviewDate:         nnr,
		OriginalQuality:        int32(oq),
		OriginalStatus:         os,
		OriginalIntervalDays:   int32(oid),
		OriginalEasinessFactor: oef,
	}), nil
}

// UndoOverrideAnswer restores the original values of a previously overridden answer.
func (h *QuizHandler) UndoOverrideAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.UndoOverrideAnswerRequest],
) (*connect.Response[apiv1.UndoOverrideAnswerResponse], error) {
	noteID := req.Msg.GetNoteId()
	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	learnedAt := req.Msg.GetLearnedAt()

	var notebookName, expression string

	h.mu.Lock()
	if card, ok := h.noteStore[noteID]; ok {
		notebookName = card.NotebookName
		expression = card.Entry
	} else if rcard, ok := h.reverseStore[noteID]; ok {
		notebookName = rcard.NotebookName
		expression = rcard.Expression
	}
	h.mu.Unlock()

	if notebookName == "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active quiz session", noteID))
	}

	correct, nextReview, err := h.svc.UndoOverrideAnswer(
		notebookName, expression, quizType, learnedAt,
		int(req.Msg.GetOriginalQuality()),
		req.Msg.GetOriginalStatus(),
		int(req.Msg.GetOriginalIntervalDays()),
		req.Msg.GetOriginalEasinessFactor(),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("undo override answer: %w", err))
	}

	return connect.NewResponse(&apiv1.UndoOverrideAnswerResponse{
		Correct:        correct,
		NextReviewDate: nextReview,
	}), nil
}

// SkipWord marks a word to be skipped from future quizzes.
func (h *QuizHandler) SkipWord(
	ctx context.Context,
	req *connect.Request[apiv1.SkipWordRequest],
) (*connect.Response[apiv1.SkipWordResponse], error) {
	noteID := req.Msg.GetNoteId()

	var notebookName, expression string

	h.mu.Lock()
	if card, ok := h.noteStore[noteID]; ok {
		notebookName = card.NotebookName
		expression = card.Entry
	} else if rcard, ok := h.reverseStore[noteID]; ok {
		notebookName = rcard.NotebookName
		expression = rcard.Expression
	}
	h.mu.Unlock()

	if notebookName == "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active quiz session", noteID))
	}

	if err := h.svc.SkipWord(notebookName, expression); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("skip word: %w", err))
	}

	return connect.NewResponse(&apiv1.SkipWordResponse{}), nil
}

// ResumeWord clears the skip status for a word, allowing it to appear in quizzes again.
func (h *QuizHandler) ResumeWord(
	ctx context.Context,
	req *connect.Request[apiv1.ResumeWordRequest],
) (*connect.Response[apiv1.ResumeWordResponse], error) {
	noteID := req.Msg.GetNoteId()

	var notebookName, expression string

	h.mu.Lock()
	if card, ok := h.noteStore[noteID]; ok {
		notebookName = card.NotebookName
		expression = card.Entry
	} else if rcard, ok := h.reverseStore[noteID]; ok {
		notebookName = rcard.NotebookName
		expression = rcard.Expression
	}
	h.mu.Unlock()

	if notebookName == "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active quiz session", noteID))
	}

	if err := h.svc.ResumeWord(notebookName, expression); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resume word: %w", err))
	}

	return connect.NewResponse(&apiv1.ResumeWordResponse{}), nil
}
