package server

import (
	"context"
	"fmt"
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
		learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(cards[i].NotebookName, cards[i].Entry, cards[i].PartOfSpeech, notebook.QuizTypeNotebook)
		responses[i] = &apiv1.SubmitAnswerResponse{
			Correct:        grades[i].Correct,
			Meaning:        cards[i].Meaning,
			Reason:         grades[i].Reason,
			WordDetail:     toProtoWordDetail(cards[i].WordDetail),
			NextReviewDate: nextReviewDate,
			LearnedAt:      learnedAt,
			Images:         cards[i].Images,
			PartOfSpeech:   cards[i].PartOfSpeech,
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
		learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(cards[i].NotebookName, cards[i].Expression, cards[i].PartOfSpeech, notebook.QuizTypeReverse)
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
			PartOfSpeech:   cards[i].PartOfSpeech,
		}
	}

	return connect.NewResponse(&apiv1.BatchSubmitReverseAnswersResponse{Responses: responses}), nil
}

// synonymAcceptedQuality is the SRS quality used when an answer is a synonym
// of the expected word and the user has accepted it on retry. Lower than a
// regular correct answer (3-5) so repeated synonym-only answers don't advance
// the word as fast as exact-match answers.
const synonymAcceptedQuality = 2

// BatchSubmitEtymologyStandardAnswers grades a batch of etymology standard answers.
func (h *QuizHandler) BatchSubmitEtymologyStandardAnswers(
	ctx context.Context,
	req *connect.Request[apiv1.BatchSubmitEtymologyStandardAnswersRequest],
) (*connect.Response[apiv1.BatchSubmitEtymologyStandardAnswersResponse], error) {
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

	grades, err := parallelGrade(ctx, answers, func(i int) (quiz.GradeResult, error) {
		if answers[i].GetIsSkipped() {
			return skippedGradeResult(), nil
		}
		return h.svc.GradeEtymologyStandardAnswer(ctx, cards[i], answers[i].GetAnswer(), answers[i].GetResponseTimeMs())
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answers: %w", err))
	}

	// Load the graph context once per notebook for the elaborative scaffold
	// the feedback card renders. Failures here are non-fatal — feedback
	// stays usable without the graph.
	var graphReader *notebook.Reader
	conceptsByNotebook := make(map[string]map[string]*graphConceptInfo)
	if r, err := h.svc.NewReader(); err == nil {
		graphReader = r
		seen := make(map[string]bool)
		for _, c := range cards {
			if seen[c.NotebookName] {
				continue
			}
			seen[c.NotebookName] = true
			conceptsByNotebook[c.NotebookName] = loadBookConcepts(ctx, r, c.NotebookName)
		}
	}

	examplesByKey, _ := h.svc.LoadEtymologyExampleWords(cards)

	responses := make([]*apiv1.SubmitEtymologyStandardAnswerResponse, len(answers))
	for i := range answers {
		if err := h.svc.SaveEtymologyOriginResult(cards[i], grades[i].Quality, grades[i].Correct, answers[i].GetResponseTimeMs(), notebook.QuizTypeEtymologyStandard, true); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save etymology result: %w", err))
		}
		learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(cards[i].NotebookName, cards[i].Origin, "", notebook.QuizTypeEtymologyStandard)
		h.mu.Lock()
		noteID := h.nextID
		h.nextID++
		h.etymologyOriginStore[noteID] = cards[i]
		h.mu.Unlock()
		var graphContext *apiv1.GraphPrompt
		if graphReader != nil {
			graphContext = buildGraphContextForCard(ctx, graphReader, cards[i], conceptsByNotebook[cards[i].NotebookName])
		}
		exampleKey := strings.ToLower(strings.TrimSpace(cards[i].Origin)) + "\x00" + cards[i].SessionTitle + "\x00" + cards[i].Sense
		responses[i] = &apiv1.SubmitEtymologyStandardAnswerResponse{
			Correct:        grades[i].Correct,
			Reason:         grades[i].Reason,
			CorrectMeaning: cards[i].Meaning,
			NextReviewDate: nextReviewDate,
			LearnedAt:      learnedAt,
			NoteId:         noteID,
			GraphContext:   graphContext,
			ExampleWords:   examplesByKey[exampleKey],
		}
	}

	return connect.NewResponse(&apiv1.BatchSubmitEtymologyStandardAnswersResponse{Responses: responses}), nil
}

// BatchSubmitEtymologyReverseAnswers grades a batch of etymology reverse answers.
func (h *QuizHandler) BatchSubmitEtymologyReverseAnswers(
	ctx context.Context,
	req *connect.Request[apiv1.BatchSubmitEtymologyReverseAnswersRequest],
) (*connect.Response[apiv1.BatchSubmitEtymologyReverseAnswersResponse], error) {
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

	grades, err := parallelGrade(ctx, answers, func(i int) (quiz.GradeResult, error) {
		if answers[i].GetIsSkipped() {
			return skippedGradeResult(), nil
		}
		return h.svc.GradeEtymologyReverseAnswer(ctx, cards[i], answers[i].GetAnswer(), answers[i].GetResponseTimeMs())
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answers: %w", err))
	}

	// One scan of definitions for the whole batch — LoadEtymologyExampleWords
	// is O(definitions) regardless of card count, so doing it once and indexing
	// by (origin, session, sense) beats per-card calls inside the loop.
	examplesByKey, _ := h.svc.LoadEtymologyExampleWords(cards)

	responses := make([]*apiv1.SubmitEtymologyReverseAnswerResponse, len(answers))
	for i := range answers {
		if err := h.svc.SaveEtymologyOriginResult(cards[i], grades[i].Quality, grades[i].Correct, answers[i].GetResponseTimeMs(), notebook.QuizTypeEtymologyReverse, true); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save etymology result: %w", err))
		}
		learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(cards[i].NotebookName, cards[i].Origin, "", notebook.QuizTypeEtymologyReverse)
		h.mu.Lock()
		noteID := h.nextID
		h.nextID++
		h.etymologyOriginStore[noteID] = cards[i]
		h.mu.Unlock()
		exampleKey := strings.ToLower(strings.TrimSpace(cards[i].Origin)) + "\x00" + cards[i].SessionTitle + "\x00" + cards[i].Sense
		responses[i] = &apiv1.SubmitEtymologyReverseAnswerResponse{
			Correct:        grades[i].Correct,
			Reason:         grades[i].Reason,
			CorrectOrigin:  cards[i].Origin,
			Type:           cards[i].Type,
			Language:       cards[i].Language,
			NextReviewDate: nextReviewDate,
			LearnedAt:      learnedAt,
			NoteId:         noteID,
			ExampleWords:   examplesByKey[exampleKey],
		}
	}

	return connect.NewResponse(&apiv1.BatchSubmitEtymologyReverseAnswersResponse{Responses: responses}), nil
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
