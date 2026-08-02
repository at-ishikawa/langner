package server

import (
	"context"
	"fmt"
	"sync"

	"connectrpc.com/connect"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// skippedGradeResult is the canonical GradeResult for a user-skipped answer.
// Recorded as incorrect with the lowest SRS quality so the word is scheduled
// for re-study. Classification is left as ClassificationWrong so the reverse
// handler's synonym-retry path treats it as a normal wrong answer (saved
// immediately, no synonym retry prompt).
func skippedGradeResult() quiz.GradeResult {
	return quiz.GradeResult{
		Correct:        false,
		Reason:         "skipped by user",
		Quality:        1,
		Classification: string(inference.ClassificationWrong),
	}
}

// BatchSubmitAnswers grades a batch of standard quiz answers.
// Grading runs in parallel (N OpenAI calls concurrently); learning history
// writes are serialized to preserve deterministic save order.
func (h *QuizHandler) BatchSubmitAnswers(
	ctx context.Context,
	req *connect.Request[apiv1.BatchSubmitAnswersRequest],
) (*connect.Response[apiv1.BatchSubmitAnswersResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	answers := req.Msg.GetAnswers()

	cards := make([]quiz.Card, len(answers))
	h.mu.Lock()
	for i, a := range answers {
		card, ok := h.noteStore[a.GetNoteId()]
		if !ok {
			h.mu.Unlock()
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found", a.GetNoteId()))
		}
		cards[i] = card
	}
	h.mu.Unlock()

	grades, err := parallelGrade(ctx, answers, func(i int) (quiz.GradeResult, error) {
		if answers[i].GetIsSkipped() {
			return skippedGradeResult(), nil
		}
		return h.svc.GradeNotebookAnswer(ctx, cards[i], answers[i].GetAnswer(), answers[i].GetResponseTimeMs())
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answers: %w", err))
	}

	responses := make([]*apiv1.SubmitAnswerResponse, len(answers))
	for i := range answers {
		if err := h.svc.SaveResult(ctx, cards[i], grades[i], answers[i].GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save result: %w", err))
		}
		learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(cards[i].NotebookName, cards[i].ID, cards[i].Entry, notebook.QuizTypeNotebook)
		responses[i] = &apiv1.SubmitAnswerResponse{
			Correct:        grades[i].Correct,
			Meaning:        cards[i].Meaning,
			Reason:         grades[i].Reason,
			WordDetail:     toProtoWordDetail(cards[i].WordDetail),
			NextReviewDate: nextReviewDate,
			LearnedAt:      learnedAt,
			Images:         cards[i].Images,
			SenseId:        cards[i].ID,
		}
	}

	return connect.NewResponse(&apiv1.BatchSubmitAnswersResponse{Responses: responses}), nil
}

// BatchSubmitReverseAnswers grades a batch of reverse quiz answers.
//
// Synonym classifications are returned as-is so the frontend can ask the user
// to retry. On a retry batch the frontend sets AcceptSynonymAsCorrect=true,
// in which case a remaining synonym is persisted as a correct result with
// reduced quality — this gives the user SRS progress and enables the
// override button on the feedback card.
func (h *QuizHandler) BatchSubmitReverseAnswers(
	ctx context.Context,
	req *connect.Request[apiv1.BatchSubmitReverseAnswersRequest],
) (*connect.Response[apiv1.BatchSubmitReverseAnswersResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	answers := req.Msg.GetAnswers()

	cards := make([]quiz.ReverseCard, len(answers))
	h.mu.Lock()
	for i, a := range answers {
		card, ok := h.reverseStore[a.GetNoteId()]
		if !ok {
			h.mu.Unlock()
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found", a.GetNoteId()))
		}
		cards[i] = card
	}
	h.mu.Unlock()

	grades, err := parallelGrade(ctx, answers, func(i int) (quiz.GradeResult, error) {
		if answers[i].GetIsSkipped() {
			return skippedGradeResult(), nil
		}
		return h.svc.GradeReverseAnswer(ctx, cards[i], answers[i].GetAnswer(), answers[i].GetResponseTimeMs())
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answers: %w", err))
	}

	responses := make([]*apiv1.SubmitReverseAnswerResponse, len(answers))
	for i := range answers {
		isSynonym := grades[i].Classification == string(inference.ClassificationSynonym)
		if isSynonym && answers[i].GetAcceptSynonymAsCorrect() {
			// Accepted-on-retry synonym: save as correct with reduced quality.
			grades[i].Correct = true
			grades[i].Quality = synonymAcceptedQuality
		}
		shouldSave := !isSynonym || answers[i].GetAcceptSynonymAsCorrect()
		if shouldSave {
			if err := h.svc.SaveReverseResult(ctx, cards[i], grades[i], answers[i].GetResponseTimeMs()); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save result: %w", err))
			}
		}
		var contexts []string
		for _, c := range cards[i].Contexts {
			contexts = append(contexts, c.Context)
		}
		learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(cards[i].NotebookName, cards[i].ID, cards[i].Expression, notebook.QuizTypeReverse)
		responses[i] = &apiv1.SubmitReverseAnswerResponse{
			Correct:        grades[i].Correct,
			Expression:     cards[i].Expression,
			Meaning:        cards[i].Meaning,
			Reason:         grades[i].Reason,
			Contexts:       contexts,
			WordDetail:     toProtoWordDetail(cards[i].WordDetail),
			Classification: grades[i].Classification,
			NextReviewDate: nextReviewDate,
			LearnedAt:      learnedAt,
			Images:         cards[i].Images,
			SenseId:        cards[i].ID,
		}
	}

	return connect.NewResponse(&apiv1.BatchSubmitReverseAnswersResponse{Responses: responses}), nil
}

// synonymAcceptedQuality is the SRS quality used when an answer is a synonym
// of the expected word and the user has accepted it on retry. Lower than a
// regular correct answer (3-5) so repeated synonym-only answers don't advance
// the word as fast as exact-match answers.
const synonymAcceptedQuality = 2

// BatchSubmitEtymologyOriginAnswers grades a batch of etymology-origin cards,
// each with its own set of family-word answers.
func (h *QuizHandler) BatchSubmitEtymologyOriginAnswers(
	ctx context.Context,
	req *connect.Request[apiv1.BatchSubmitEtymologyOriginAnswersRequest],
) (*connect.Response[apiv1.BatchSubmitEtymologyOriginAnswersResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	answers := req.Msg.GetAnswers()

	cards := make([]quiz.EtymologyOriginCard, len(answers))
	h.mu.Lock()
	for i, a := range answers {
		card, ok := h.etymologyOriginStore[a.GetCardId()]
		if !ok {
			h.mu.Unlock()
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("card %d not found", a.GetCardId()))
		}
		cards[i] = card
	}
	h.mu.Unlock()

	responses := make([]*apiv1.SubmitEtymologyOriginAnswerResponse, len(answers))
	for i := range answers {
		resp, err := h.gradeAndSaveEtymologyOrigin(ctx, cards[i], answers[i].GetAnswers(), answers[i].GetResponseTimeMs(), answers[i].GetIsSkipped())
		if err != nil {
			return nil, err
		}
		responses[i] = resp
	}

	return connect.NewResponse(&apiv1.BatchSubmitEtymologyOriginAnswersResponse{Responses: responses}), nil
}

// parallelGrade runs gradeFn(i) for i in [0, len(items)) concurrently and
// returns results in original order. If any call returns an error, the first
// error encountered (by index) is returned.
func parallelGrade[T any](ctx context.Context, items []T, gradeFn func(int) (quiz.GradeResult, error)) ([]quiz.GradeResult, error) {
	results := make([]quiz.GradeResult, len(items))
	errs := make([]error, len(items))
	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if ctx.Err() != nil {
				errs[i] = ctx.Err()
				return
			}
			res, err := gradeFn(i)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = res
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
	}
	return results, nil
}
