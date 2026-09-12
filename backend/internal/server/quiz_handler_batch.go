package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

// gradeError converts a grader failure into a Connect error. A provider
// rate-limit / quota exhaustion (HTTP 429 / RESOURCE_EXHAUSTED) is surfaced as
// CodeResourceExhausted with a short, user-facing retry hint so the client can
// show "please retry in a moment" instead of an opaque internal error. Every
// other failure stays CodeInternal, wrapped with msg for context.
func gradeError(msg string, err error) error {
	s := err.Error()
	if strings.Contains(s, "response error 429") || strings.Contains(s, "RESOURCE_EXHAUSTED") {
		return connect.NewError(connect.CodeResourceExhausted,
			errors.New("grading is temporarily rate-limited — please retry in a moment"))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("%s: %w", msg, err))
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

	grades, err := gradeBatch(ctx, answers,
		func(i int) bool { return answers[i].GetIsSkipped() },
		func(ctx context.Context, idx []int) ([]quiz.GradeResult, error) {
			subCards := make([]quiz.Card, len(idx))
			subAns := make([]string, len(idx))
			subRT := make([]int64, len(idx))
			for k, i := range idx {
				subCards[k] = cards[i]
				subAns[k] = answers[i].GetAnswer()
				subRT[k] = answers[i].GetResponseTimeMs()
			}
			return h.svc.GradeNotebookAnswerBatch(ctx, subCards, subAns, subRT)
		},
		func(i int) (quiz.GradeResult, error) {
			return h.svc.GradeNotebookAnswer(ctx, cards[i], answers[i].GetAnswer(), answers[i].GetResponseTimeMs())
		},
	)
	if err != nil {
		return nil, gradeError("grade answers", err)
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

	grades, err := gradeBatch(ctx, answers,
		func(i int) bool { return answers[i].GetIsSkipped() },
		func(ctx context.Context, idx []int) ([]quiz.GradeResult, error) {
			subCards := make([]quiz.ReverseCard, len(idx))
			subAns := make([]string, len(idx))
			subRT := make([]int64, len(idx))
			for k, i := range idx {
				subCards[k] = cards[i]
				subAns[k] = answers[i].GetAnswer()
				subRT[k] = answers[i].GetResponseTimeMs()
			}
			return h.svc.GradeReverseAnswerBatch(ctx, subCards, subAns, subRT)
		},
		func(i int) (quiz.GradeResult, error) {
			return h.svc.GradeReverseAnswer(ctx, cards[i], answers[i].GetAnswer(), answers[i].GetResponseTimeMs())
		},
	)
	if err != nil {
		return nil, gradeError("grade answers", err)
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

// gradeBatch grades every answer in items and returns the results in original
// order. Skipped answers get skippedGradeResult with NO LLM call. The remaining
// answers are graded with ONE batched LLM call (batchFn) — this is the fix for
// the batch-submit 429 burst, where the old parallelGrade fanned out one call
// per answer.
//
// batchFn receives the indices of the non-skipped answers and must return one
// GradeResult per index, in that order. If the batched response is unusable
// (malformed / wrong-length — inference.ErrMalformedResponse), gradeBatch logs
// and falls back to grading exactly those answers individually (perItemFn, the
// previous N-parallel-calls path) so batching can never grade worse than before.
// A transport failure (network / 429) is returned as-is and does NOT trigger the
// N-call fallback, so a rate-limited batch does not re-burst the same quota.
func gradeBatch[T any](
	ctx context.Context,
	items []T,
	skipped func(i int) bool,
	batchFn func(ctx context.Context, indices []int) ([]quiz.GradeResult, error),
	perItemFn func(i int) (quiz.GradeResult, error),
) ([]quiz.GradeResult, error) {
	results := make([]quiz.GradeResult, len(items))
	toGrade := make([]int, 0, len(items))
	for i := range items {
		if skipped(i) {
			results[i] = skippedGradeResult()
			continue
		}
		toGrade = append(toGrade, i)
	}
	if len(toGrade) == 0 {
		return results, nil
	}

	graded, err := batchFn(ctx, toGrade)
	if err != nil {
		if !errors.Is(err, inference.ErrMalformedResponse) {
			return nil, err
		}
		slog.Warn("batched grading unusable; falling back to per-item grading",
			"error", err, "count", len(toGrade))
		graded, err = parallelGrade(ctx, toGrade, func(k int) (quiz.GradeResult, error) {
			return perItemFn(toGrade[k])
		})
		if err != nil {
			return nil, err
		}
	}
	if len(graded) != len(toGrade) {
		return nil, fmt.Errorf("graded %d of %d answers", len(graded), len(toGrade))
	}
	for k, idx := range toGrade {
		results[idx] = graded[k]
	}
	return results, nil
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
