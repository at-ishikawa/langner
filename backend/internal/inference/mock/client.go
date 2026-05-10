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

func (c *Client) AnswerMeanings(_ context.Context, params inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
	answers := make([]inference.AnswerMeaning, 0, len(params.Expressions))
	for _, expr := range params.Expressions {
		userAnswer := strings.ToLower(strings.TrimSpace(expr.Meaning))
		contexts := make([]inference.AnswersForContext, 0, len(expr.Contexts))
		for _, ctx := range expr.Contexts {
			correct := userAnswer != "" && userAnswer != "i don't know"
			contexts = append(contexts, inference.AnswersForContext{
				Correct: correct,
				Context: ctx.Context,
				Reason:  "mock grader: marked correct because the answer was non-empty",
				Quality: 3,
			})
		}
		if len(contexts) == 0 {
			contexts = append(contexts, inference.AnswersForContext{
				Correct: userAnswer != "" && userAnswer != "i don't know",
				Reason:  "mock grader: marked correct because the answer was non-empty",
				Quality: 3,
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
