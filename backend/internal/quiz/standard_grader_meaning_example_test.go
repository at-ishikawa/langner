package quiz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
)

// TestExampleData_StandardGradeSendsStoredMeaningWithoutContexts reproduces
// failure mode A through the REAL Service/Reader construction the server uses
// (config.example.yml dirs → LoadCards → GradeNotebookAnswer), not a hand-built
// card.
//
// The example flashcard "ephemeral" declares only an expression + meaning and NO
// example sentences, so its loaded card has zero Contexts. Before the fix the
// card's stored meaning reached the grader ONLY through a per-context
// reference_definition, so a context-less card sent the judge NOTHING
// authoritative and it graded against its own dictionary knowledge. This test
// loads that card for real and asserts the stored meaning is now carried to the
// judge as the expression-level ReferenceMeaning regardless of contexts.
func TestExampleData_StandardGradeSendsStoredMeaningWithoutContexts(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	learningDir := t.TempDir()
	svc := NewService(
		exampleNotebooksConfig(t, learningDir),
		inference.StaticResolver(mockClient),
		make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true},
	)

	cards, err := svc.LoadCards(0, []string{"vocabulary"}, true, nil)
	require.NoError(t, err)

	var card *Card
	for i := range cards {
		if cards[i].Entry == "ephemeral" {
			card = &cards[i]
			break
		}
	}
	require.NotNil(t, card, "expected the example flashcard 'ephemeral' to load")
	require.Empty(t, card.Contexts,
		"the 'ephemeral' flashcard has no example sentences, so the stored meaning can only reach the grader via ReferenceMeaning")

	var got inference.AnswerMeaningsRequest
	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
			got = req
			return inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{{
					Expression:        req.Expressions[0].Expression,
					AnswersForContext: []inference.AnswersForContext{{Correct: true, Quality: 5}},
				}},
			}, nil
		})

	_, err = svc.GradeNotebookAnswer(context.Background(), 0, *card, "lasting a very short time", 1000)
	require.NoError(t, err)

	require.Len(t, got.Expressions, 1)
	assert.Equal(t, card.Meaning, got.Expressions[0].ReferenceMeaning,
		"the stored meaning must reach the grader as ReferenceMeaning even with no example contexts")
	assert.Equal(t, "lasting for a very short time; transitory", got.Expressions[0].ReferenceMeaning,
		"the authoritative meaning must be the one authored in the notebook")
}
