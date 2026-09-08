package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/inference"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/quiz"
)

func TestGradeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{
			name: "429 maps to resource exhausted",
			err:  errors.New("response error 429: quota exceeded"),
			want: connect.CodeResourceExhausted,
		},
		{
			name: "RESOURCE_EXHAUSTED maps to resource exhausted",
			err:  errors.New("some wrapper: RESOURCE_EXHAUSTED"),
			want: connect.CodeResourceExhausted,
		},
		{
			name: "other errors stay internal",
			err:  errors.New("json.Unmarshal failed"),
			want: connect.CodeInternal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gradeError("grade answers", tt.err)
			assert.Equal(t, tt.want, connect.CodeOf(got))
		})
	}
}

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
				// The batch handler now grades all answers in ONE AnswerMeanings
				// call, returning one answer per input expression in order.
				m.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, req inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
						answers := make([]inference.AnswerMeaning, len(req.Expressions))
						for i, expr := range req.Expressions {
							isCorrect := expr.Expression == "word1"
							reason := "ok"
							if !isCorrect {
								reason = "no"
							}
							answers[i] = inference.AnswerMeaning{
								Expression:        expr.Expression,
								Meaning:           expr.Meaning,
								AnswersForContext: []inference.AnswersForContext{{Correct: isCorrect, Reason: reason, Quality: 3}},
							}
						}
						return inference.AnswerMeaningsResponse{Answers: answers}, nil
					},
				).Times(1)
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

			// The reverse batch now grades through the single-call batched path.
			mockClient.EXPECT().ValidateWordFormBatch(gomock.Any(), gomock.Any()).Return(
				[]inference.ValidateWordFormResponse{{
					Classification: inference.ClassificationSynonym,
					Reason:         "valid synonym",
					Quality:        2,
				}}, nil,
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

// TestQuizHandler_BatchSubmitReverseAnswers_OneLLMCallForBatch is the core
// regression test for the 429-burst fix: a batch submit with N>1 non-skipped
// answers makes EXACTLY ONE ValidateWordFormBatch call (not one per answer),
// returns the correct per-item grade in order, and never sends a skipped item
// to the LLM (nor calls the per-item ValidateWordForm grader).
func TestQuizHandler_BatchSubmitReverseAnswers_OneLLMCallForBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandler(t, mockClient)

	handler.reverseStore[1] = quiz.ReverseCard{NotebookName: "n", Expression: "break the ice", Meaning: "to ease tension"}
	handler.reverseStore[2] = quiz.ReverseCard{NotebookName: "n", Expression: "lose one's temper", Meaning: "to become angry"}
	handler.reverseStore[3] = quiz.ReverseCard{NotebookName: "n", Expression: "hit the sack", Meaning: "to go to bed"}
	handler.reverseStore[4] = quiz.ReverseCard{NotebookName: "n", Expression: "spill the beans", Meaning: "to reveal a secret"}

	// Exactly ONE batched call grades the three non-skipped answers; the per-item
	// grader must never be reached.
	mockClient.EXPECT().
		ValidateWordFormBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params []inference.ValidateWordFormRequest) ([]inference.ValidateWordFormResponse, error) {
			require.Len(t, params, 3, "skipped answers must not be sent to the LLM")
			out := make([]inference.ValidateWordFormResponse, len(params))
			for i, p := range params {
				if strings.EqualFold(strings.TrimSpace(p.UserAnswer), p.Expected) {
					out[i] = inference.ValidateWordFormResponse{Index: i, Classification: inference.ClassificationSameWord, Reason: "match", Quality: 5}
				} else {
					out[i] = inference.ValidateWordFormResponse{Index: i, Classification: inference.ClassificationWrong, Reason: "no match", Quality: 1}
				}
			}
			return out, nil
		}).
		Times(1)
	mockClient.EXPECT().ValidateWordForm(gomock.Any(), gomock.Any()).Times(0)

	resp, err := handler.BatchSubmitReverseAnswers(context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitReverseAnswersRequest{
			Answers: []*apiv1.SubmitReverseAnswerRequest{
				{NoteId: 1, Answer: "break the ice"},  // correct
				{NoteId: 2, Answer: "totally unrelated"}, // wrong
				{NoteId: 4, IsSkipped: true},           // skipped -> no LLM, no per-item call
				{NoteId: 3, Answer: "hit the sack"},    // correct
			},
		}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), 4)

	// Grades map back to the original order, including the skipped item.
	assert.True(t, resp.Msg.GetResponses()[0].GetCorrect())
	assert.Equal(t, "break the ice", resp.Msg.GetResponses()[0].GetExpression())
	assert.False(t, resp.Msg.GetResponses()[1].GetCorrect())
	assert.False(t, resp.Msg.GetResponses()[2].GetCorrect(), "skipped answer is incorrect")
	assert.Equal(t, "skipped by user", resp.Msg.GetResponses()[2].GetReason())
	assert.True(t, resp.Msg.GetResponses()[3].GetCorrect())
	assert.Equal(t, "hit the sack", resp.Msg.GetResponses()[3].GetExpression())
}

// TestQuizHandler_BatchSubmitReverseAnswers_FallsBackOnMalformedBatch proves the
// robustness contract: when the single batched call returns an unusable result
// (here a wrong-length array, which the service tags ErrMalformedResponse), the
// handler falls back to grading each answer individually and still returns the
// correct results, so batching can never make grading worse than before.
func TestQuizHandler_BatchSubmitReverseAnswers_FallsBackOnMalformedBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandler(t, mockClient)

	handler.reverseStore[1] = quiz.ReverseCard{NotebookName: "n", Expression: "break the ice", Meaning: "to ease tension"}
	handler.reverseStore[2] = quiz.ReverseCard{NotebookName: "n", Expression: "hit the sack", Meaning: "to go to bed"}

	// One batched attempt returns a wrong-length array (1 result for 2 answers).
	mockClient.EXPECT().
		ValidateWordFormBatch(gomock.Any(), gomock.Any()).
		Return([]inference.ValidateWordFormResponse{
			{Classification: inference.ClassificationSameWord, Reason: "only one", Quality: 5},
		}, nil).
		Times(1)
	// Fallback: exactly one per-item call per answer.
	mockClient.EXPECT().
		ValidateWordForm(gomock.Any(), gomock.Any()).
		Return(inference.ValidateWordFormResponse{Classification: inference.ClassificationSameWord, Reason: "per-item match", Quality: 4}, nil).
		Times(2)

	resp, err := handler.BatchSubmitReverseAnswers(context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitReverseAnswersRequest{
			Answers: []*apiv1.SubmitReverseAnswerRequest{
				{NoteId: 1, Answer: "break the ice"},
				{NoteId: 2, Answer: "hit the sack"},
			},
		}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), 2)
	assert.True(t, resp.Msg.GetResponses()[0].GetCorrect())
	assert.True(t, resp.Msg.GetResponses()[1].GetCorrect())
	assert.Equal(t, "per-item match", resp.Msg.GetResponses()[0].GetReason())
}

// TestQuizHandler_BatchSubmitAnswers_OneLLMCallForBatch proves the standard
// (recognition) batch path also collapses to ONE AnswerMeanings call for all
// non-skipped answers, mapping the array back by index.
func TestQuizHandler_BatchSubmitAnswers_OneLLMCallForBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandler(t, mockClient)

	handler.noteStore[1] = quiz.Card{NotebookName: "n", Entry: "break the ice", Meaning: "to ease tension"}
	handler.noteStore[2] = quiz.Card{NotebookName: "n", Entry: "hit the sack", Meaning: "to go to bed"}
	handler.noteStore[3] = quiz.Card{NotebookName: "n", Entry: "spill the beans", Meaning: "to reveal a secret"}

	mockClient.EXPECT().
		AnswerMeanings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
			require.Len(t, req.Expressions, 2, "skipped answers must not be sent to the LLM")
			answers := make([]inference.AnswerMeaning, len(req.Expressions))
			for i, e := range req.Expressions {
				correct := !strings.HasPrefix(strings.ToLower(e.Meaning), "wrong")
				answers[i] = inference.AnswerMeaning{
					Expression:        e.Expression,
					AnswersForContext: []inference.AnswersForContext{{Correct: correct, Reason: "r", Quality: 4}},
				}
			}
			return inference.AnswerMeaningsResponse{Answers: answers}, nil
		}).
		Times(1)

	resp, err := handler.BatchSubmitAnswers(context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitAnswersRequest{
			Answers: []*apiv1.SubmitAnswerRequest{
				{NoteId: 1, Answer: "to ease tension"}, // correct
				{NoteId: 3, IsSkipped: true},           // skipped
				{NoteId: 2, Answer: "wrong guess"},     // wrong
			},
		}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), 3)
	assert.True(t, resp.Msg.GetResponses()[0].GetCorrect())
	assert.False(t, resp.Msg.GetResponses()[1].GetCorrect(), "skipped answer is incorrect")
	assert.False(t, resp.Msg.GetResponses()[2].GetCorrect())
}

// TestQuizHandler_BatchSubmitReverseAnswers_ReorderedResultsMapByIndex is the
// core correctness regression for PR #68: the model (e.g. Gemini) may return the
// batch array in a DIFFERENT order than the input. The grader must map each
// result back to its own answer by the echoed Index, never by slice position.
// The fake here grades each item correctly but returns the results REVERSED
// (each still carrying its true Index and, in Reason, the word it graded). With
// position-based mapping this scrambles every grade; with Index-based mapping
// every answer is graded against its own word.
//
// This test FAILS against the old position-based code and PASSES after the fix.
func TestQuizHandler_BatchSubmitReverseAnswers_ReorderedResultsMapByIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandler(t, mockClient)

	handler.reverseStore[1] = quiz.ReverseCard{NotebookName: "n", Expression: "break the ice", Meaning: "to ease tension"}
	handler.reverseStore[2] = quiz.ReverseCard{NotebookName: "n", Expression: "hit the sack", Meaning: "to go to bed"}
	handler.reverseStore[3] = quiz.ReverseCard{NotebookName: "n", Expression: "spill the beans", Meaning: "to reveal a secret"}

	mockClient.EXPECT().
		ValidateWordFormBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params []inference.ValidateWordFormRequest) ([]inference.ValidateWordFormResponse, error) {
			out := make([]inference.ValidateWordFormResponse, len(params))
			for i, p := range params {
				cls := inference.ClassificationWrong
				if strings.EqualFold(strings.TrimSpace(p.UserAnswer), p.Expected) {
					cls = inference.ClassificationSameWord
				}
				// Index echoes the input position; Reason carries the graded word
				// so the assertions can prove identity, not just correctness.
				out[i] = inference.ValidateWordFormResponse{Index: i, Classification: cls, Reason: p.Expected, Quality: 5}
			}
			// Return the array out of order (reversed) — a model that reorders.
			for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
				out[l], out[r] = out[r], out[l]
			}
			return out, nil
		}).
		Times(1)
	mockClient.EXPECT().ValidateWordForm(gomock.Any(), gomock.Any()).Times(0)

	resp, err := handler.BatchSubmitReverseAnswers(context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitReverseAnswersRequest{
			Answers: []*apiv1.SubmitReverseAnswerRequest{
				{NoteId: 1, Answer: "break the ice"},    // correct
				{NoteId: 2, Answer: "hit the sack"},      // correct
				{NoteId: 3, Answer: "totally unrelated"}, // wrong
			},
		}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), 3)

	// Each answer must be graded against ITS OWN word despite the reordering.
	assert.True(t, resp.Msg.GetResponses()[0].GetCorrect(), "break the ice was correct")
	assert.Equal(t, "break the ice", resp.Msg.GetResponses()[0].GetReason(), "grade must map to its own word")
	assert.True(t, resp.Msg.GetResponses()[1].GetCorrect(), "hit the sack was correct")
	assert.Equal(t, "hit the sack", resp.Msg.GetResponses()[1].GetReason())
	assert.False(t, resp.Msg.GetResponses()[2].GetCorrect(), "spill the beans answer was wrong")
	assert.Equal(t, "spill the beans", resp.Msg.GetResponses()[2].GetReason())
}

// TestQuizHandler_BatchSubmitAnswers_ReorderedResultsMapByExpression is the
// standard (recognition) counterpart: the AnswerMeanings array may come back in
// a different order, so the grader maps each answer to its card by the echoed
// expression, not by position. The fake grades correctly but reverses the
// array; expression-based mapping keeps every grade with its own word.
//
// This test FAILS against the old position-based code and PASSES after the fix.
func TestQuizHandler_BatchSubmitAnswers_ReorderedResultsMapByExpression(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandler(t, mockClient)

	handler.noteStore[1] = quiz.Card{NotebookName: "n", Entry: "break the ice", Meaning: "to ease tension"}
	handler.noteStore[2] = quiz.Card{NotebookName: "n", Entry: "hit the sack", Meaning: "to go to bed"}
	handler.noteStore[3] = quiz.Card{NotebookName: "n", Entry: "spill the beans", Meaning: "to reveal a secret"}

	mockClient.EXPECT().
		AnswerMeanings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
			answers := make([]inference.AnswerMeaning, len(req.Expressions))
			for i, e := range req.Expressions {
				// User answer travels in Meaning; a "wrong"-prefixed answer is wrong.
				correct := !strings.HasPrefix(strings.ToLower(e.Meaning), "wrong")
				answers[i] = inference.AnswerMeaning{
					Expression:        e.Expression,
					AnswersForContext: []inference.AnswersForContext{{Correct: correct, Reason: e.Expression, Quality: 4}},
				}
			}
			// Return the array out of order (reversed) — a model that reorders.
			for l, r := 0, len(answers)-1; l < r; l, r = l+1, r-1 {
				answers[l], answers[r] = answers[r], answers[l]
			}
			return inference.AnswerMeaningsResponse{Answers: answers}, nil
		}).
		Times(1)

	resp, err := handler.BatchSubmitAnswers(context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitAnswersRequest{
			Answers: []*apiv1.SubmitAnswerRequest{
				{NoteId: 1, Answer: "to ease tension"}, // correct
				{NoteId: 2, Answer: "to go to bed"},    // correct
				{NoteId: 3, Answer: "wrong guess"},     // wrong
			},
		}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), 3)

	assert.True(t, resp.Msg.GetResponses()[0].GetCorrect(), "break the ice was correct")
	assert.Equal(t, "break the ice", resp.Msg.GetResponses()[0].GetReason(), "grade must map to its own word")
	assert.True(t, resp.Msg.GetResponses()[1].GetCorrect(), "hit the sack was correct")
	assert.Equal(t, "hit the sack", resp.Msg.GetResponses()[1].GetReason())
	assert.False(t, resp.Msg.GetResponses()[2].GetCorrect(), "spill the beans answer was wrong")
	assert.Equal(t, "spill the beans", resp.Msg.GetResponses()[2].GetReason())
}

// TestQuizHandler_BatchSubmitReverseAnswers_FallsBackOnDuplicateIndex proves the
// correct-or-fallback contract: a right-length batch whose indices don't form a
// clean 1:1 set (here a duplicated index, so one item is unmapped) is treated as
// ErrMalformedResponse, and the handler falls back to per-item grading.
func TestQuizHandler_BatchSubmitReverseAnswers_FallsBackOnDuplicateIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandler(t, mockClient)

	handler.reverseStore[1] = quiz.ReverseCard{NotebookName: "n", Expression: "break the ice", Meaning: "to ease tension"}
	handler.reverseStore[2] = quiz.ReverseCard{NotebookName: "n", Expression: "hit the sack", Meaning: "to go to bed"}

	// Right length (2), but both results claim Index 0 — index 1 is unmapped.
	mockClient.EXPECT().
		ValidateWordFormBatch(gomock.Any(), gomock.Any()).
		Return([]inference.ValidateWordFormResponse{
			{Index: 0, Classification: inference.ClassificationSameWord, Reason: "dup", Quality: 5},
			{Index: 0, Classification: inference.ClassificationSameWord, Reason: "dup", Quality: 5},
		}, nil).
		Times(1)
	// Fallback: exactly one per-item call per answer.
	mockClient.EXPECT().
		ValidateWordForm(gomock.Any(), gomock.Any()).
		Return(inference.ValidateWordFormResponse{Classification: inference.ClassificationSameWord, Reason: "per-item match", Quality: 4}, nil).
		Times(2)

	resp, err := handler.BatchSubmitReverseAnswers(context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitReverseAnswersRequest{
			Answers: []*apiv1.SubmitReverseAnswerRequest{
				{NoteId: 1, Answer: "break the ice"},
				{NoteId: 2, Answer: "hit the sack"},
			},
		}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), 2)
	assert.True(t, resp.Msg.GetResponses()[0].GetCorrect())
	assert.True(t, resp.Msg.GetResponses()[1].GetCorrect())
	assert.Equal(t, "per-item match", resp.Msg.GetResponses()[0].GetReason())
}

// TestQuizHandler_BatchSubmitAnswers_FallsBackOnUnknownExpression proves the
// standard-path correct-or-fallback contract: a right-length batch that echoes
// an expression not in the request (so one request expression is unmatched) is
// treated as ErrMalformedResponse, and the handler falls back to per-item.
func TestQuizHandler_BatchSubmitAnswers_FallsBackOnUnknownExpression(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandler(t, mockClient)

	handler.noteStore[1] = quiz.Card{NotebookName: "n", Entry: "break the ice", Meaning: "to ease tension"}
	handler.noteStore[2] = quiz.Card{NotebookName: "n", Entry: "hit the sack", Meaning: "to go to bed"}

	// The batched call sends both expressions at once; the per-item fallback
	// sends one at a time. Distinguish by count (the fallback calls run
	// concurrently, so avoid a shared counter that would race under -race).
	mockClient.EXPECT().
		AnswerMeanings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
			if len(req.Expressions) > 1 {
				// The batched call: return a bogus/extra expression nobody asked for.
				return inference.AnswerMeaningsResponse{Answers: []inference.AnswerMeaning{
					{Expression: "break the ice", AnswersForContext: []inference.AnswersForContext{{Correct: true, Reason: "ok", Quality: 4}}},
					{Expression: "some other phrase", AnswersForContext: []inference.AnswersForContext{{Correct: true, Reason: "ok", Quality: 4}}},
				}}, nil
			}
			// Fallback per-item calls: grade the single expression correctly.
			ans := make([]inference.AnswerMeaning, len(req.Expressions))
			for i, e := range req.Expressions {
				ans[i] = inference.AnswerMeaning{Expression: e.Expression, AnswersForContext: []inference.AnswersForContext{{Correct: true, Reason: "per-item", Quality: 4}}}
			}
			return inference.AnswerMeaningsResponse{Answers: ans}, nil
		}).
		Times(3) // 1 batched (malformed) + 2 per-item fallback

	resp, err := handler.BatchSubmitAnswers(context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitAnswersRequest{
			Answers: []*apiv1.SubmitAnswerRequest{
				{NoteId: 1, Answer: "to ease tension"},
				{NoteId: 2, Answer: "to go to bed"},
			},
		}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), 2)
	assert.True(t, resp.Msg.GetResponses()[0].GetCorrect())
	assert.True(t, resp.Msg.GetResponses()[1].GetCorrect())
	assert.Equal(t, "per-item", resp.Msg.GetResponses()[0].GetReason())
}
