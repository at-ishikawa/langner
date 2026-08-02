package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

func TestQuizHandler_BatchSubmitAnswers(t *testing.T) {
	tests := []struct {
		name              string
		answers           []*apiv1.SubmitAnswerRequest
		setupNoteStore    func(h *QuizHandler)
		setupMock         func(m *mock_inference.MockClient)
		wantCode          connect.Code
		wantErr           bool
		wantResponseCount int
		wantFirstCorrect  bool
	}{
		{
			name:     "returns INVALID_ARGUMENT when request has no answers",
			answers:  nil,
			wantCode: connect.CodeInvalidArgument,
			wantErr:  true,
		},
		{
			name: "returns NOT_FOUND when any note is missing",
			answers: []*apiv1.SubmitAnswerRequest{
				{NoteId: 1, Answer: "x", ResponseTimeMs: 500},
				{NoteId: 999, Answer: "y", ResponseTimeMs: 500},
			},
			setupNoteStore: func(h *QuizHandler) {
				h.noteStore[1] = quiz.Card{NotebookName: "n", Entry: "a", Meaning: "b"}
			},
			wantCode: connect.CodeNotFound,
			wantErr:  true,
		},
		{
			name: "grades all answers and preserves order",
			answers: []*apiv1.SubmitAnswerRequest{
				{NoteId: 1, Answer: "correct answer", ResponseTimeMs: 1000},
				{NoteId: 2, Answer: "wrong answer", ResponseTimeMs: 2000},
			},
			setupNoteStore: func(h *QuizHandler) {
				h.noteStore[1] = quiz.Card{NotebookName: "n1", Entry: "word1", Meaning: "meaning1"}
				h.noteStore[2] = quiz.Card{NotebookName: "n2", Entry: "word2", Meaning: "meaning2"}
			},
			setupMock: func(m *mock_inference.MockClient) {
				// The batch handler fires two concurrent AnswerMeanings calls; gomock
				// matches any order so either arrival sequence is fine.
				m.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, req inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
						expr := req.Expressions[0]
						isCorrect := expr.Expression == "word1"
						reason := "ok"
						if !isCorrect {
							reason = "no"
						}
						return inference.AnswerMeaningsResponse{
							Answers: []inference.AnswerMeaning{{
								Expression: expr.Expression,
								Meaning:    expr.Meaning,
								AnswersForContext: []inference.AnswersForContext{{Correct: isCorrect, Reason: reason, Quality: 3}},
							}},
						}, nil
					},
				).Times(2)
			},
			wantResponseCount: 2,
			wantFirstCorrect:  true,
		},
		{
			name: "returns INTERNAL when any grade fails",
			answers: []*apiv1.SubmitAnswerRequest{
				{NoteId: 1, Answer: "a", ResponseTimeMs: 500},
			},
			setupNoteStore: func(h *QuizHandler) {
				h.noteStore[1] = quiz.Card{NotebookName: "n", Entry: "w", Meaning: "m"}
			},
			setupMock: func(m *mock_inference.MockClient) {
				m.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
					inference.AnswerMeaningsResponse{}, assert.AnError,
				)
			},
			wantCode: connect.CodeInternal,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockClient := mock_inference.NewMockClient(ctrl)
			handler := newTestHandler(t, mockClient)

			if tt.setupNoteStore != nil {
				tt.setupNoteStore(handler)
			}
			if tt.setupMock != nil {
				tt.setupMock(mockClient)
			}

			resp, err := handler.BatchSubmitAnswers(
				context.Background(),
				connect.NewRequest(&apiv1.BatchSubmitAnswersRequest{Answers: tt.answers}),
			)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, resp)
				connectErr, ok := err.(*connect.Error)
				require.True(t, ok)
				assert.Equal(t, tt.wantCode, connectErr.Code())
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Len(t, resp.Msg.GetResponses(), tt.wantResponseCount)
			if tt.wantResponseCount > 0 {
				assert.Equal(t, tt.wantFirstCorrect, resp.Msg.GetResponses()[0].GetCorrect())
			}
		})
	}
}

// TestQuizHandler_BatchSubmitAnswers_Skip verifies that within a batch,
// answers with IsSkipped=true bypass the LLM and are recorded as incorrect,
// while non-skipped answers in the same batch still get graded normally.
func TestQuizHandler_BatchSubmitAnswers_Skip(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	// Exactly one grading call: only the non-skipped answer should reach
	// the LLM. The skipped one must short-circuit.
	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{
			Answers: []inference.AnswerMeaning{{
				Expression: "word2",
				Meaning:    "meaning2",
				AnswersForContext: []inference.AnswersForContext{{Correct: true, Reason: "ok", Quality: 4}},
			}},
		}, nil,
	).Times(1)

	handler := newTestHandler(t, mockClient)
	handler.noteStore[1] = quiz.Card{NotebookName: "n1", Entry: "word1", Meaning: "meaning1"}
	handler.noteStore[2] = quiz.Card{NotebookName: "n2", Entry: "word2", Meaning: "meaning2"}

	resp, err := handler.BatchSubmitAnswers(
		context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitAnswersRequest{
			Answers: []*apiv1.SubmitAnswerRequest{
				{NoteId: 1, Answer: "", IsSkipped: true, ResponseTimeMs: 500},
				{NoteId: 2, Answer: "meaning2", ResponseTimeMs: 500},
			},
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Msg.GetResponses(), 2)
	assert.False(t, resp.Msg.GetResponses()[0].GetCorrect(), "skipped answer must be incorrect")
	assert.Equal(t, "skipped by user", resp.Msg.GetResponses()[0].GetReason())
	assert.True(t, resp.Msg.GetResponses()[1].GetCorrect(), "graded answer must reflect inference")
}

// newTestEtymologyHandler builds a QuizHandler with only LearningNotesDirectory
// wired (no notebook/definitions fixtures) so tests can construct
// quiz.EtymologyOriginCard values directly and inspect the YAML
// gradeAndSaveEtymologyOrigin writes.
func newTestEtymologyHandler(t *testing.T, openaiClient inference.Client) (*QuizHandler, string) {
	t.Helper()
	learningDir := t.TempDir()
	svc := quiz.NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, openaiClient, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
	return NewQuizHandler(svc), learningDir
}

// TestQuizHandler_BatchSubmitEtymologyOriginAnswers_BlankWordGradedIncorrect
// pins the answering model: there is no "skip"/"don't know" control, so a word
// the learner leaves blank is graded INCORRECT — a normal miss that counts
// against the origin's aggregate and keeps it due — while sibling words the
// learner typed are graded independently on their own merits. The blank word
// must never mark the origin as excluded from future quizzes (that stays a
// distinct, explicit SkipWord action; learning-history invariants L1/L4).
func TestQuizHandler_BatchSubmitEtymologyOriginAnswers_BlankWordGradedIncorrect(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler, learningDir := newTestEtymologyHandler(t, mockClient)

	const notebookName = "roots"
	card := quiz.EtymologyOriginCard{
		NotebookName: notebookName,
		SessionTitle: "Session 1",
		Origin:       "scribo",
		Meaning:      "to write",
		Words: []quiz.EtymologyFamilyWord{
			{Expression: "describe", Meaning: "to represent in words"},
			{Expression: "inscribe", Meaning: "to write or carve on a surface"},
			{Expression: "transcribe", Meaning: "to write out in full"},
		},
	}
	handler.etymologyOriginStore[1] = card

	// describe and transcribe are answered with exact matches (short-circuits
	// ValidateWordForm, so no mock expectation is needed); inscribe is left
	// blank. No OpenAI call should happen at all.
	resp, err := handler.BatchSubmitEtymologyOriginAnswers(
		context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitEtymologyOriginAnswersRequest{
			Answers: []*apiv1.SubmitEtymologyOriginAnswerRequest{{
				CardId: 1,
				Answers: []*apiv1.EtymologyWordAnswer{
					{WordId: 1, Answer: "to represent in words"},
					{WordId: 2, Answer: ""},
					{WordId: 3, Answer: "to write out in full"},
				},
				ResponseTimeMs: 500,
			}},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Msg.GetResponses(), 1)
	got := resp.Msg.GetResponses()[0]

	results := got.GetResults()
	require.Len(t, results, 3)
	assert.True(t, results[0].GetCorrect(), "describe: the sibling word actually answered must be graded correctly")
	assert.False(t, results[1].GetCorrect(), "inscribe: left blank, so graded incorrect")
	assert.True(t, results[2].GetCorrect(), "transcribe: unaffected by the blank sibling")

	// One blank word makes the aggregate incorrect; the origin is recorded as
	// a normal miss and stays due — not excluded.
	assert.False(t, got.GetCorrect(), "a blank word counts against the aggregate")
	require.NotEmpty(t, got.GetLearnedAt())

	raw, err := os.ReadFile(filepath.Join(learningDir, notebookName+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	expr := notebook.FindOriginExpression(histories, "Session 1", "scribo", "")
	require.NotNil(t, expr)
	assert.False(t, expr.SkippedAt.IsSkippedAny(),
		"a blank answer must never mark the origin as excluded from quizzes")
	require.Len(t, expr.EtymologyOriginLogs, 1)
	log := expr.EtymologyOriginLogs[0]
	assert.Equal(t, notebook.LearnedStatusMisunderstood, log.Status)
	assert.True(t, expr.NeedsEtymologyReview(notebook.QuizTypeEtymologyOrigin),
		"a missed origin must stay due for review")
	require.Len(t, log.WordResults, 3)
	assert.False(t, log.WordResults[1].Correct, "the blank word's per-word log entry must record a miss")
}

// TestQuizHandler_BatchSubmitEtymologyOriginAnswers_AllBlank verifies that
// leaving every derived word blank records ONE normal wrong attempt
// (misunderstood) for the origin — never an excluded one — so the origin
// resurfaces for review instead of silently disappearing from the quiz.
func TestQuizHandler_BatchSubmitEtymologyOriginAnswers_AllBlank(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler, learningDir := newTestEtymologyHandler(t, mockClient)

	const notebookName = "roots"
	card := quiz.EtymologyOriginCard{
		NotebookName: notebookName,
		SessionTitle: "Session 1",
		Origin:       "scribo",
		Meaning:      "to write",
		Words: []quiz.EtymologyFamilyWord{
			{Expression: "describe", Meaning: "to represent in words"},
		},
	}
	handler.etymologyOriginStore[1] = card

	resp, err := handler.BatchSubmitEtymologyOriginAnswers(
		context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitEtymologyOriginAnswersRequest{
			Answers: []*apiv1.SubmitEtymologyOriginAnswerRequest{{
				CardId:         1,
				Answers:        []*apiv1.EtymologyWordAnswer{{WordId: 1, Answer: ""}},
				ResponseTimeMs: 500,
			}},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, resp)
	got := resp.Msg.GetResponses()[0]
	assert.False(t, got.GetCorrect())

	raw, err := os.ReadFile(filepath.Join(learningDir, notebookName+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	expr := notebook.FindOriginExpression(histories, "Session 1", "scribo", "")
	require.NotNil(t, expr)
	assert.False(t, expr.SkippedAt.IsSkippedAny(),
		"leaving every word blank must not exclude the origin from future quizzes")
	require.Len(t, expr.EtymologyOriginLogs, 1)
	assert.Equal(t, notebook.LearnedStatusMisunderstood, expr.EtymologyOriginLogs[0].Status,
		"an all-blank attempt is recorded as a normal miss, not an exclusion")
}

// TestQuizHandler_BatchSubmitEtymologyOriginAnswers_TwoAnsweredTwoBlank pins
// independent per-word grading in a single submission: a 4-word family where
// the learner typed answers for 2 words (one right, one wrong) and left the
// other 2 blank. Each answered word is graded on its own merits, each blank
// word is graded incorrect (not a distinct "skipped" state), and none of them
// exclude the origin from future quizzes.
func TestQuizHandler_BatchSubmitEtymologyOriginAnswers_TwoAnsweredTwoBlank(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler, learningDir := newTestEtymologyHandler(t, mockClient)

	const notebookName = "roots"
	card := quiz.EtymologyOriginCard{
		NotebookName: notebookName,
		SessionTitle: "Session 1",
		Origin:       "graph",
		Meaning:      "to write",
		Words: []quiz.EtymologyFamilyWord{
			{Expression: "photograph", Meaning: "light writing"},
			{Expression: "autograph", Meaning: "self writing"},
			{Expression: "telegraph", Meaning: "distant writing"},
			{Expression: "paragraph", Meaning: "beside writing"},
		},
	}
	handler.etymologyOriginStore[1] = card

	// photograph: answered with an exact match (correct, no OpenAI call).
	// autograph: answered but wrong (needs a mocked OpenAI classification).
	// telegraph, paragraph: both left blank in the same request (no OpenAI).
	mockClient.EXPECT().ValidateWordForm(gomock.Any(), gomock.Any()).Return(
		inference.ValidateWordFormResponse{
			Classification: inference.ClassificationWrong,
			Reason:         "not close enough",
			Quality:        1,
		}, nil,
	)

	resp, err := handler.BatchSubmitEtymologyOriginAnswers(
		context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitEtymologyOriginAnswersRequest{
			Answers: []*apiv1.SubmitEtymologyOriginAnswerRequest{{
				CardId: 1,
				Answers: []*apiv1.EtymologyWordAnswer{
					{WordId: 1, Answer: "light writing"},
					{WordId: 2, Answer: "a wrong guess"},
					{WordId: 3, Answer: ""},
					{WordId: 4, Answer: ""},
				},
				ResponseTimeMs: 500,
			}},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Msg.GetResponses(), 1)
	got := resp.Msg.GetResponses()[0]

	results := got.GetResults()
	require.Len(t, results, 4)
	assert.True(t, results[0].GetCorrect(), "photograph: typed answer must be graded correct on its own merits")
	assert.False(t, results[1].GetCorrect(), "autograph: typed answer must be graded, and graded wrong here")
	assert.False(t, results[2].GetCorrect(), "telegraph: left blank, graded incorrect")
	assert.False(t, results[3].GetCorrect(), "paragraph: left blank, graded incorrect")

	assert.False(t, got.GetCorrect())

	raw, err := os.ReadFile(filepath.Join(learningDir, notebookName+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	expr := notebook.FindOriginExpression(histories, "Session 1", "graph", "")
	require.NotNil(t, expr)
	assert.False(t, expr.SkippedAt.IsSkippedAny(),
		"2 blank words alongside 2 answers must never exclude the origin from future quizzes")
	require.Len(t, expr.EtymologyOriginLogs, 1)
	log := expr.EtymologyOriginLogs[0]
	require.Len(t, log.WordResults, 4)
	assert.True(t, log.WordResults[0].Correct, "photograph's stored log entry must be correct")
	assert.False(t, log.WordResults[1].Correct, "autograph's stored log entry must be incorrect")
	assert.False(t, log.WordResults[2].Correct, "telegraph's stored log entry (blank) must be a miss")
	assert.False(t, log.WordResults[3].Correct, "paragraph's stored log entry (blank) must be a miss")
}

// TestQuizHandler_BatchSubmitReverseAnswers_SynonymPersistence documents the
// current behavior around synonym classifications in the reverse quiz and
// then, in the `accept_synonym_as_correct=true` case, pins the fixed
// behavior where a retry-accepted synonym is saved as a correct result.
func TestQuizHandler_BatchSubmitReverseAnswers_SynonymPersistence(t *testing.T) {
	tests := []struct {
		name                   string
		acceptSynonymAsCorrect bool
		wantCorrect            bool
		wantLearnedAtPopulated bool
		wantClassification     string
	}{
		{
			// Initial submission of a synonym — client will ask the user to
			// retry, so we purposefully skip saving. The response therefore
			// has an empty LearnedAt and the override button stays hidden.
			name:                   "synonym on initial submission is not persisted",
			acceptSynonymAsCorrect: false,
			wantCorrect:            false,
			wantLearnedAtPopulated: false,
			wantClassification:     "synonym",
		},
		{
			// Retry-accepted synonym: the frontend flags the retry batch so the
			// backend saves it as a correct result. LearnedAt is populated,
			// enabling the override button and advancing SRS.
			name:                   "synonym on retry is persisted as correct",
			acceptSynonymAsCorrect: true,
			wantCorrect:            true,
			wantLearnedAtPopulated: true,
			wantClassification:     "synonym",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockClient := mock_inference.NewMockClient(ctrl)
			handler := newTestHandler(t, mockClient)

			handler.reverseStore[1] = quiz.ReverseCard{
				NotebookName: "notebook",
				Expression:   "lose one's temper",
				Meaning:      "to become angry",
			}

			mockClient.EXPECT().ValidateWordForm(gomock.Any(), gomock.Any()).Return(
				inference.ValidateWordFormResponse{
					Classification: inference.ClassificationSynonym,
					Reason:         "valid synonym",
					Quality:        2,
				}, nil,
			)

			resp, err := handler.BatchSubmitReverseAnswers(
				context.Background(),
				connect.NewRequest(&apiv1.BatchSubmitReverseAnswersRequest{
					Answers: []*apiv1.SubmitReverseAnswerRequest{{
						NoteId:                 1,
						Answer:                 "get mad",
						ResponseTimeMs:         1000,
						AcceptSynonymAsCorrect: tt.acceptSynonymAsCorrect,
					}},
				}),
			)

			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Len(t, resp.Msg.GetResponses(), 1)
			got := resp.Msg.GetResponses()[0]

			assert.Equal(t, tt.wantCorrect, got.GetCorrect())
			assert.Equal(t, tt.wantClassification, got.GetClassification())
			if tt.wantLearnedAtPopulated {
				assert.NotEmpty(t, got.GetLearnedAt(),
					"expected a saved learning log so the frontend can show the override button")
			} else {
				assert.Empty(t, got.GetLearnedAt(),
					"expected no save for an unaccepted synonym submission")
			}
		})
	}
}
