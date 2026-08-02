// Package mock provides a deterministic inference.Client used by e2e tests.
// It avoids real OpenAI calls and instead grades answers by substring matching
// the user's input against the meaning the seed defines for each expression.
package mock

import (
	"context"
	"strings"

	"github.com/at-ishikawa/langner/internal/inference"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

// isWrongAnswer returns true for sentinel answers a test scenario can use to
// force the mock grader to mark a card incorrect. Any answer starting with
// "wrong" (case-insensitive), the literal "I don't know", or empty input is
// treated as incorrect. Every other non-empty answer is marked correct so
// happy-path tests don't need to know the expected meaning in advance.
func isWrongAnswer(userAnswer string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(userAnswer))
	if trimmed == "" || trimmed == "i don't know" {
		return true
	}
	return strings.HasPrefix(trimmed, "wrong")
}

func gradeResult(userAnswer string) (correct bool, reason string) {
	if isWrongAnswer(userAnswer) {
		return false, "mock grader: answer matches the wrong-answer sentinel"
	}
	return true, "mock grader: marked correct because the answer was non-empty"
}

func (c *Client) AnswerMeanings(_ context.Context, params inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
	answers := make([]inference.AnswerMeaning, 0, len(params.Expressions))
	for _, expr := range params.Expressions {
		correct, reason := gradeResult(expr.Meaning)
		quality := 3
		if !correct {
			quality = 1
		}
		contexts := make([]inference.AnswersForContext, 0, len(expr.Contexts))
		for _, ctx := range expr.Contexts {
			contexts = append(contexts, inference.AnswersForContext{
				Correct: correct,
				Context: ctx.Context,
				Reason:  reason,
				Quality: quality,
			})
		}
		if len(contexts) == 0 {
			contexts = append(contexts, inference.AnswersForContext{
				Correct: correct,
				Reason:  reason,
				Quality: quality,
			})
		}
		answers = append(answers, inference.AnswerMeaning{
			Expression:        expr.Expression,
			Meaning:           expr.Meaning,
			AnswersForContext: contexts,
		})
	}
	return inference.AnswerMeaningsResponse{Answers: answers}, nil
}

// GradeCorrection deterministically grades a grammar correction: the answer is
// correct when it contains the reference correction and no longer contains the
// incorrect span (case-insensitively), or when the answer equals the correction.
// The wrong-answer sentinels still force an incorrect result.
func (c *Client) GradeCorrection(_ context.Context, params inference.GradeCorrectionRequest) (inference.GradeCorrectionResponse, error) {
	answer := strings.ToLower(strings.TrimSpace(params.UserAnswer))
	correct := strings.ToLower(strings.TrimSpace(params.Correct))
	incorrect := strings.ToLower(strings.TrimSpace(params.Incorrect))

	isCorrect := !isWrongAnswer(params.UserAnswer) &&
		strings.Contains(answer, correct) &&
		(incorrect == "" || !strings.Contains(answer, incorrect))

	reason := "mock grader: answer resolves the mistake"
	quality := 3
	if !isCorrect {
		reason = "mock grader: answer does not resolve the mistake"
		quality = 1
	}
	return inference.GradeCorrectionResponse{
		Correct: isCorrect,
		Reason:  reason,
		Quality: quality,
	}, nil
}

func (c *Client) ValidateWordForm(_ context.Context, params inference.ValidateWordFormRequest) (inference.ValidateWordFormResponse, error) {
	user := strings.ToLower(strings.TrimSpace(params.UserAnswer))
	expected := strings.ToLower(strings.TrimSpace(params.Expected))
	classification := inference.ClassificationWrong
	reason := "mock grader: user answer differs from expected"
	if user == expected {
		classification = inference.ClassificationSameWord
		reason = "mock grader: exact match"
	} else if user != "" && (strings.Contains(expected, user) || strings.Contains(user, expected)) {
		classification = inference.ClassificationSynonym
		reason = "mock grader: substring match"
	}
	return inference.ValidateWordFormResponse{
		Classification: classification,
		Reason:         reason,
		Quality:        3,
	}, nil
}

func (c *Client) LookupWord(_ context.Context, params inference.LookupWordRequest) (inference.LookupWordResponse, error) {
	return inference.LookupWordResponse{
		Definitions: []inference.LookupWordDefinition{
			{
				PartOfSpeech: "noun",
				Definition:   "(mock definition for " + params.Word + ")",
			},
		},
	}, nil
}
