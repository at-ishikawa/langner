package server

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// grammarKey composes the in-memory store key for a grammar card. Mistake ids
// are unique within a notebook; the notebook id disambiguates across notebooks.
func grammarKey(notebookID, cardID string) string {
	return notebookID + "\x00" + cardID
}

func toProtoGrammarCard(c quiz.GrammarCard) *apiv1.GrammarCard {
	return &apiv1.GrammarCard{
		NotebookId: c.NotebookID,
		CardId:     c.MistakeID,
		EntryId:    c.EntryID,
		Sentence:   c.Sentence,
		Incorrect:  c.Incorrect,
		Category:   c.Category,
		Note:       c.Note,
		Status:     c.Status,
	}
}

// StartGrammarQuiz loads the due grammar-correction cards for the requested
// journal notebooks and caches them for grading.
func (h *QuizHandler) StartGrammarQuiz(
	_ context.Context,
	req *connect.Request[apiv1.StartGrammarQuizRequest],
) (*connect.Response[apiv1.StartGrammarQuizResponse], error) {
	var cards []quiz.GrammarCard
	for _, notebookID := range req.Msg.GetNotebookIds() {
		loaded, err := h.svc.LoadGrammarCards(notebookID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load grammar cards for %q: %w", notebookID, err))
		}
		cards = append(cards, loaded...)
	}

	protoCards := make([]*apiv1.GrammarCard, 0, len(cards))
	h.mu.Lock()
	h.grammarStore = make(map[string]quiz.GrammarCard, len(cards))
	for _, c := range cards {
		h.grammarStore[grammarKey(c.NotebookID, c.MistakeID)] = c
		protoCards = append(protoCards, toProtoGrammarCard(c))
	}
	h.mu.Unlock()

	return connect.NewResponse(&apiv1.StartGrammarQuizResponse{Cards: protoCards}), nil
}

// SubmitGrammarAnswer grades a single grammar correction and records it.
func (h *QuizHandler) SubmitGrammarAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitGrammarAnswerRequest],
) (*connect.Response[apiv1.SubmitGrammarAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	resp, err := h.submitGrammarAnswer(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// BatchSubmitGrammarAnswers grades a batch sequentially. Grading is graded one
// at a time on purpose: every submit writes the same notebook's learning-notes
// YAML, so serialising avoids concurrent-write races on that file.
func (h *QuizHandler) BatchSubmitGrammarAnswers(
	ctx context.Context,
	req *connect.Request[apiv1.BatchSubmitGrammarAnswersRequest],
) (*connect.Response[apiv1.BatchSubmitGrammarAnswersResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	answers := req.Msg.GetAnswers()
	responses := make([]*apiv1.SubmitGrammarAnswerResponse, 0, len(answers))
	for _, a := range answers {
		resp, err := h.submitGrammarAnswer(ctx, a)
		if err != nil {
			return nil, err
		}
		responses = append(responses, resp)
	}
	return connect.NewResponse(&apiv1.BatchSubmitGrammarAnswersResponse{Responses: responses}), nil
}

func (h *QuizHandler) submitGrammarAnswer(
	ctx context.Context,
	msg *apiv1.SubmitGrammarAnswerRequest,
) (*apiv1.SubmitGrammarAnswerResponse, error) {
	key := grammarKey(msg.GetNotebookId(), msg.GetCardId())
	h.mu.Lock()
	card, ok := h.grammarStore[key]
	h.mu.Unlock()
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("grammar card %q not found", msg.GetCardId()))
	}

	var grade quiz.GradeResult
	if msg.GetIsSkipped() {
		grade = skippedGradeResult()
	} else {
		var err error
		grade, err = h.svc.GradeGrammarAnswer(ctx, card, msg.GetAnswer(), msg.GetResponseTimeMs())
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade grammar answer: %w", err))
		}
	}

	if err := h.svc.SaveGrammarResult(ctx, card, grade, msg.GetResponseTimeMs()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save grammar result: %w", err))
	}

	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookID, card.MistakeID, notebook.QuizTypeGrammar)
	return &apiv1.SubmitGrammarAnswerResponse{
		Correct:        grade.Correct,
		CorrectAnswer:  card.Correct,
		Reason:         grade.Reason,
		Incorrect:      card.Incorrect,
		NextReviewDate: nextReviewDate,
		LearnedAt:      learnedAt,
	}, nil
}
