package server

import (
	"context"
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
