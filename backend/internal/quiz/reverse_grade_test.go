package quiz

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// newExampleServiceWithClient is newExampleService with a caller-supplied
// inference client. A gomock client with no expectations fails the test if
// any grader method is called.
func newExampleServiceWithClient(t *testing.T, client inference.Client) *Service {
	t.Helper()
	dir := t.TempDir()
	return NewService(
		exampleNotebooksConfig(t, dir),
		inference.StaticResolver(client),
		make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(dir, nil),
		config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true},
	)
}

// deterministicReverseCases are answers the matcher settles, so the LLM must
// never see them.
var deterministicReverseCases = []struct {
	accepted []string
	answer   string
	correct  bool
}{
	{[]string{"harbour"}, "harbour", true},
	{[]string{"harbour"}, "Harbour.", true},
	{[]string{"step on someone's toes", "stepped on my toes"}, "stepped on my toes", true},
	{[]string{"go", "went"}, "went", true},
	{[]string{"step on someone's toes"}, "stepped on his toes", true},
	{[]string{"hurt oneself"}, "hurt himself", true},
	{[]string{"(get) cold feet"}, "cold feet", true},
	{[]string{"spill the beans"}, "spill the beens", true},
	{[]string{"spill the beans"}, "spilled the beens", true},
	{[]string{"millennium"}, "millennium", true},
	{[]string{"millennium"}, "millenium", false},
}

func TestGradeReverseAnswer_DeterministicAnswersSkipLLM(t *testing.T) {
	svc := newExampleServiceWithClient(t, mock_inference.NewMockClient(gomock.NewController(t)))
	for _, tc := range deterministicReverseCases {
		t.Run(tc.answer, func(t *testing.T) {
			card := ReverseCard{Expression: tc.accepted[0], AcceptedForms: tc.accepted}
			got, err := svc.GradeReverseAnswer(context.Background(), 0, card, tc.answer, 1000)
			require.NoError(t, err)
			want, ok := matchReverseAnswer(tc.accepted, tc.answer, 1000)
			require.True(t, ok)
			assert.Equal(t, want, got)
			assert.Equal(t, tc.correct, got.Correct)
		})
	}
}

func TestGradeReverseAnswer_UnmatchedAnswerGoesToLLMWithAcceptedForms(t *testing.T) {
	client := mock_inference.NewMockClient(gomock.NewController(t))
	accepted := []string{"step on someone's toes", "stepped on my toes"}
	client.EXPECT().ValidateWordForm(gomock.Any(), inference.ValidateWordFormRequest{
		Expected: "step on someone's toes", AcceptedForms: accepted, UserAnswer: "stepped over my toes",
		Meaning: "to upset someone", ResponseTimeMs: 1000,
	}).Return(inference.ValidateWordFormResponse{Classification: inference.ClassificationWrong, Reason: "You changed the preposition."}, nil)
	svc := newExampleServiceWithClient(t, client)

	got, err := svc.GradeReverseAnswer(context.Background(), 0,
		ReverseCard{Expression: "step on someone's toes", Meaning: "to upset someone", AcceptedForms: accepted}, "stepped over my toes", 1000)
	require.NoError(t, err)
	assert.False(t, got.Correct)
	assert.Equal(t, "You changed the preposition.", got.Reason)
}

func TestGradeReverseAnswerBatch_OnlyUnmatchedAnswersReachLLM(t *testing.T) {
	client := mock_inference.NewMockClient(gomock.NewController(t))
	var sent []inference.ValidateWordFormRequest
	client.EXPECT().ValidateWordFormBatch(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params []inference.ValidateWordFormRequest) ([]inference.ValidateWordFormResponse, error) {
			sent = params
			// Out of order on purpose: results are mapped back by Index.
			return []inference.ValidateWordFormResponse{
				{Index: 1, Classification: inference.ClassificationSynonym, Reason: "Close, but not the expression asked for."},
				{Index: 0, Classification: inference.ClassificationWrong, Reason: "You changed the preposition."},
			}, nil
		}).Times(1)
	svc := newExampleServiceWithClient(t, client)

	toes := []string{"step on someone's toes", "stepped on my toes"}
	cards := []ReverseCard{
		{Expression: toes[0], AcceptedForms: toes},
		{Expression: "millennium", AcceptedForms: []string{"millennium"}},
		{Expression: toes[0], AcceptedForms: toes},
		{Expression: "harbour", AcceptedForms: []string{"harbour"}},
		{Expression: "happy", AcceptedForms: []string{"happy"}},
	}
	answers := []string{"stepped on my toes", "millenium", "stepped over my toes", "  ", "joyful"}
	got, err := svc.GradeReverseAnswerBatch(context.Background(), 0, cards, answers, []int64{1000, 1000, 1000, 1000, 1000})
	require.NoError(t, err)

	require.Len(t, sent, 2, "only the unmatched answers reach the LLM")
	assert.Equal(t, "stepped over my toes", sent[0].UserAnswer)
	assert.Equal(t, toes, sent[0].AcceptedForms)
	assert.Equal(t, "joyful", sent[1].UserAnswer)
	assert.Equal(t, []string{"happy"}, sent[1].AcceptedForms)

	require.Len(t, got, 5)
	assert.True(t, got[0].Correct)
	assert.Equal(t, reverseReasonExact, got[0].Reason)
	assert.False(t, got[1].Correct)
	assert.Equal(t, reverseReasonMisspelled, got[1].Reason)
	assert.Equal(t, string(inference.ClassificationWrong), got[2].Classification)
	assert.Equal(t, emptyAnswerGrade(), got[3])
	assert.Equal(t, string(inference.ClassificationSynonym), got[4].Classification)
}

// findReverseCard returns the loaded reverse card whose lemma is expression.
func findReverseCard(t *testing.T, cards []ReverseCard, expression string) ReverseCard {
	t.Helper()
	for _, c := range cards {
		if c.Expression == expression {
			return c
		}
	}
	t.Fatalf("reverse card %q not loaded", expression)
	return ReverseCard{}
}

// exampleReverseFormCases are the example notebooks' words (examples/stories/
// office-talk, examples/flashcards/reverse-forms) with the forms their cards
// must accept and an answer the matcher settles.
var exampleReverseFormCases = []struct {
	notebook string
	lemma    string
	accepted []string
	answer   string
	correct  bool
}{
	{"office-talk", "step on someone's toes", []string{"step on someone's toes", "stepped on my toes"}, "stepped on my toes", true},
	{"office-talk", "step on someone's toes", []string{"step on someone's toes", "stepped on my toes"}, "stepped on his toes", true},
	{"reverse-forms", "go", []string{"go", "went"}, "went", true},
	{"reverse-forms", "keep one's cool", []string{"keep one's cool", "kept her cool"}, "kept her cool", true},
	{"reverse-forms", "spill the beans", []string{"spill the beans", "spilled the beans"}, "spilled the beens", true},
	{"reverse-forms", "millennium", []string{"millennium"}, "millenium", false},
}

// TestExampleData_ReverseCardsAcceptTheirOwnForms drives the real loaders over
// config.example.yml: each reverse card carries the lemma, alt form and the
// example highlights as AcceptedForms, and grading those forms (single and
// batched) never calls the LLM.
func TestExampleData_ReverseCardsAcceptTheirOwnForms(t *testing.T) {
	ctx := context.Background()
	svc := newExampleServiceWithClient(t, mock_inference.NewMockClient(gomock.NewController(t)))

	cards, err := svc.LoadReverseCards(0, []string{"office-talk", "reverse-forms"}, false, true, nil)
	require.NoError(t, err)

	var batchCards []ReverseCard
	var answers []string
	for _, tc := range exampleReverseFormCases {
		card := findReverseCard(t, cards, tc.lemma)
		assert.Equal(t, tc.accepted, card.AcceptedForms, tc.lemma)
		got, err := svc.GradeReverseAnswer(ctx, 0, card, tc.answer, 1000)
		require.NoError(t, err)
		assert.Equal(t, tc.correct, got.Correct, "%s: %q", tc.lemma, tc.answer)
		batchCards = append(batchCards, card)
		answers = append(answers, tc.answer)
	}

	got, err := svc.GradeReverseAnswerBatch(ctx, 0, batchCards, answers, make([]int64, len(answers)))
	require.NoError(t, err)
	for i, tc := range exampleReverseFormCases {
		assert.Equal(t, tc.correct, got[i].Correct, "batch %s: %q", tc.lemma, tc.answer)
	}
}

// TestExampleData_RelearnReverseCardsAcceptTheirOwnForms misses each example
// word in reverse through the real write path, then checks the Relearn
// reverse card built by LoadRelearnPool carries the same AcceptedForms and
// grades the example's own form without the LLM.
func TestExampleData_RelearnReverseCardsAcceptTheirOwnForms(t *testing.T) {
	ctx := context.Background()
	svc := newExampleServiceWithClient(t, mock_inference.NewMockClient(gomock.NewController(t)))

	cards, err := svc.LoadReverseCards(0, []string{"office-talk", "reverse-forms"}, false, true, nil)
	require.NoError(t, err)
	for _, tc := range exampleReverseFormCases {
		card := findReverseCard(t, cards, tc.lemma)
		require.NoError(t, svc.SaveReverseResult(ctx, 0, card, GradeResult{Correct: false, Quality: 1}, 1000))
	}

	pool, err := svc.LoadRelearnPool(0, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	for _, tc := range exampleReverseFormCases {
		var relearn *ReverseCard
		for i := range pool {
			if rc := pool[i].ReverseCard(); pool[i].Format == notebook.QuizTypeReverse && rc.Expression == tc.lemma {
				relearn = &rc
			}
		}
		require.NotNil(t, relearn, "reverse-missed %q must be in the relearn pool", tc.lemma)
		assert.Equal(t, tc.accepted, relearn.AcceptedForms, tc.lemma)
		got, err := svc.GradeReverseAnswer(ctx, 0, *relearn, tc.answer, 1000)
		require.NoError(t, err)
		assert.Equal(t, tc.correct, got.Correct, "relearn %s: %q", tc.lemma, tc.answer)
	}
}

// TestExampleData_RelearnOriginReverseCardAcceptsItsForms covers the
// etymology-origin family card: a reverse miss on an origin-bearing example
// word (roots-demo "transact") folds into its origin card, whose reverse card
// carries AcceptedForms and settles a regular inflection without the LLM.
func TestExampleData_RelearnOriginReverseCardAcceptsItsForms(t *testing.T) {
	ctx := context.Background()
	svc := newExampleServiceWithClient(t, mock_inference.NewMockClient(gomock.NewController(t)))

	cards, err := svc.LoadReverseCards(0, []string{"roots-demo"}, false, true, nil)
	require.NoError(t, err)
	card := findReverseCard(t, cards, "transact")
	require.NoError(t, svc.SaveReverseResult(ctx, 0, card, GradeResult{Correct: false, Quality: 1}, 1000))

	pool, err := svc.LoadRelearnPool(0, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	origin := relearnCardFor(pool, "transact")
	require.NotNil(t, origin)
	require.Equal(t, notebook.QuizTypeEtymologyOrigin, origin.Format)
	require.Equal(t, notebook.QuizTypeReverse, origin.Direction)
	assert.Equal(t, []string{"transact"}, origin.ReverseCard().AcceptedForms)

	got, err := svc.GradeReverseAnswer(ctx, 0, origin.ReverseCard(), "transacted", 1000)
	require.NoError(t, err)
	assert.True(t, got.Correct)
	assert.Equal(t, reverseReasonSameForm, got.Reason)
}
