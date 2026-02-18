package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	quizv1 "github.com/at-ishikawa/langner/gen/quiz/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func newTestHandler(t *testing.T, openaiClient inference.Client) *QuizHandler {
	t.Helper()

	storiesDir := t.TempDir()
	learningNotesDir := t.TempDir()

	cfg := &config.Config{
		Notebooks: config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			FlashcardsDirectories:  []string{},
			BooksDirectories:       []string{},
			DefinitionsDirectories: []string{},
			LearningNotesDirectory: learningNotesDir,
		},
	}

	return NewQuizHandler(cfg, openaiClient, make(map[string]rapidapi.Response))
}

func TestQuizHandler_GetQuizOptions(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "returns empty notebooks when no data exists",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockClient := mock_inference.NewMockClient(ctrl)
			handler := newTestHandler(t, mockClient)

			resp, err := handler.GetQuizOptions(
				context.Background(),
				connect.NewRequest(&quizv1.GetQuizOptionsRequest{}),
			)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Empty(t, resp.Msg.GetNotebooks())
		})
	}
}

func TestQuizHandler_StartQuiz(t *testing.T) {
	tests := []struct {
		name        string
		notebookIDs []string
		wantCode    connect.Code
		wantErr     bool
	}{
		{
			name:        "returns INVALID_ARGUMENT when no notebook IDs provided",
			notebookIDs: nil,
			wantCode:    connect.CodeInvalidArgument,
			wantErr:     true,
		},
		{
			name:        "returns INVALID_ARGUMENT when empty notebook IDs slice",
			notebookIDs: []string{},
			wantCode:    connect.CodeInvalidArgument,
			wantErr:     true,
		},
		{
			name:        "returns NOT_FOUND for non-existent notebook",
			notebookIDs: []string{"non-existent"},
			wantCode:    connect.CodeNotFound,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockClient := mock_inference.NewMockClient(ctrl)
			handler := newTestHandler(t, mockClient)

			resp, err := handler.StartQuiz(
				context.Background(),
				connect.NewRequest(&quizv1.StartQuizRequest{
					NotebookIds: tt.notebookIDs,
				}),
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
			assert.NotNil(t, resp)
		})
	}
}

func TestQuizHandler_SubmitAnswer(t *testing.T) {
	tests := []struct {
		name           string
		noteID         int64
		answer         string
		setupNoteStore func(h *QuizHandler)
		setupMock      func(m *mock_inference.MockClient)
		wantCode       connect.Code
		wantErr        bool
		wantCorrect    bool
		wantMeaning    string
		wantReason     string
	}{
		{
			name:     "returns INVALID_ARGUMENT when note_id is zero",
			noteID:   0,
			answer:   "some answer",
			wantCode: connect.CodeInvalidArgument,
			wantErr:  true,
		},
		{
			name:     "returns INVALID_ARGUMENT when answer is empty",
			noteID:   1,
			answer:   "",
			wantCode: connect.CodeInvalidArgument,
			wantErr:  true,
		},
		{
			name:     "returns INVALID_ARGUMENT when answer is whitespace only",
			noteID:   1,
			answer:   "   ",
			wantCode: connect.CodeInvalidArgument,
			wantErr:  true,
		},
		{
			name:     "returns NOT_FOUND when note does not exist in session",
			noteID:   999,
			answer:   "some answer",
			wantCode: connect.CodeNotFound,
			wantErr:  true,
		},
		{
			name:   "returns correct result for correct answer",
			noteID: 1,
			answer: "to understand",
			setupNoteStore: func(h *QuizHandler) {
				h.noteStore[1] = &quizNote{
					notebookName: "test-notebook",
					storyTitle:   "flashcards",
					sceneTitle:   "",
					expression:   "comprehend",
					meaning:      "to understand completely",
				}
			},
			setupMock: func(m *mock_inference.MockClient) {
				m.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
					inference.AnswerMeaningsResponse{
						Answers: []inference.AnswerMeaning{
							{
								Expression: "comprehend",
								Meaning:    "to understand completely",
								AnswersForContext: []inference.AnswersForContext{
									{
										Correct: true,
										Reason:  "The answer captures the core meaning.",
										Quality: 4,
									},
								},
							},
						},
					}, nil,
				)
			},
			wantCorrect: true,
			wantMeaning: "to understand completely",
			wantReason:  "The answer captures the core meaning.",
		},
		{
			name:   "returns incorrect result for wrong answer",
			noteID: 1,
			answer: "to eat",
			setupNoteStore: func(h *QuizHandler) {
				h.noteStore[1] = &quizNote{
					notebookName: "test-notebook",
					storyTitle:   "flashcards",
					sceneTitle:   "",
					expression:   "comprehend",
					meaning:      "to understand completely",
				}
			},
			setupMock: func(m *mock_inference.MockClient) {
				m.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
					inference.AnswerMeaningsResponse{
						Answers: []inference.AnswerMeaning{
							{
								Expression: "comprehend",
								Meaning:    "to understand completely",
								AnswersForContext: []inference.AnswersForContext{
									{
										Correct: false,
										Reason:  "The answer does not match the meaning.",
										Quality: 1,
									},
								},
							},
						},
					}, nil,
				)
			},
			wantCorrect: false,
			wantMeaning: "to understand completely",
			wantReason:  "The answer does not match the meaning.",
		},
		{
			name:   "returns INTERNAL when inference fails",
			noteID: 1,
			answer: "some answer",
			setupNoteStore: func(h *QuizHandler) {
				h.noteStore[1] = &quizNote{
					notebookName: "test-notebook",
					storyTitle:   "flashcards",
					sceneTitle:   "",
					expression:   "comprehend",
					meaning:      "to understand completely",
				}
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

			resp, err := handler.SubmitAnswer(
				context.Background(),
				connect.NewRequest(&quizv1.SubmitAnswerRequest{
					NoteId:         tt.noteID,
					Answer:         tt.answer,
					ResponseTimeMs: 1000,
				}),
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
			assert.Equal(t, tt.wantCorrect, resp.Msg.GetCorrect())
			assert.Equal(t, tt.wantMeaning, resp.Msg.GetMeaning())
			assert.Equal(t, tt.wantReason, resp.Msg.GetReason())
		})
	}
}

func TestExtractAnswerResult(t *testing.T) {
	tests := []struct {
		name        string
		result      inference.AnswerMeaning
		wantCorrect bool
		wantReason  string
		wantQuality int
	}{
		{
			name: "empty answers returns incorrect with quality 1",
			result: inference.AnswerMeaning{
				AnswersForContext: nil,
			},
			wantCorrect: false,
			wantReason:  "",
			wantQuality: 1,
		},
		{
			name: "correct answer extracts fields",
			result: inference.AnswerMeaning{
				AnswersForContext: []inference.AnswersForContext{
					{Correct: true, Reason: "Good answer", Quality: 5},
				},
			},
			wantCorrect: true,
			wantReason:  "Good answer",
			wantQuality: 5,
		},
		{
			name: "quality zero defaults to 4 for correct",
			result: inference.AnswerMeaning{
				AnswersForContext: []inference.AnswersForContext{
					{Correct: true, Reason: "OK", Quality: 0},
				},
			},
			wantCorrect: true,
			wantReason:  "OK",
			wantQuality: 4,
		},
		{
			name: "quality zero defaults to 1 for incorrect",
			result: inference.AnswerMeaning{
				AnswersForContext: []inference.AnswersForContext{
					{Correct: false, Reason: "Wrong", Quality: 0},
				},
			},
			wantCorrect: false,
			wantReason:  "Wrong",
			wantQuality: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCorrect, gotReason, gotQuality := extractAnswerResult(tt.result)
			assert.Equal(t, tt.wantCorrect, gotCorrect)
			assert.Equal(t, tt.wantReason, gotReason)
			assert.Equal(t, tt.wantQuality, gotQuality)
		})
	}
}

func TestCountStoryDefinitions(t *testing.T) {
	tests := []struct {
		name    string
		stories []notebook.StoryNotebook
		want    int
	}{
		{
			name:    "empty stories",
			stories: nil,
			want:    0,
		},
		{
			name: "counts definitions across stories and scenes",
			stories: []notebook.StoryNotebook{
				{
					Scenes: []notebook.StoryScene{
						{Definitions: []notebook.Note{{Expression: "a"}, {Expression: "b"}}},
						{Definitions: []notebook.Note{{Expression: "c"}}},
					},
				},
				{
					Scenes: []notebook.StoryScene{
						{Definitions: []notebook.Note{{Expression: "d"}}},
					},
				},
			},
			want: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countStoryDefinitions(tt.stories)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCountFlashcardCards(t *testing.T) {
	tests := []struct {
		name      string
		notebooks []notebook.FlashcardNotebook
		want      int
	}{
		{
			name:      "empty notebooks",
			notebooks: nil,
			want:      0,
		},
		{
			name: "counts cards across notebooks",
			notebooks: []notebook.FlashcardNotebook{
				{Cards: []notebook.Note{{Expression: "a"}, {Expression: "b"}}},
				{Cards: []notebook.Note{{Expression: "c"}}},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countFlashcardCards(tt.notebooks)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContainsExpression(t *testing.T) {
	tests := []struct {
		name       string
		textLower  string
		expression string
		definition string
		want       bool
	}{
		{
			name:       "matches expression",
			textLower:  "i need to comprehend this",
			expression: "comprehend",
			definition: "",
			want:       true,
		},
		{
			name:       "matches definition",
			textLower:  "he ran away quickly",
			expression: "run",
			definition: "ran",
			want:       true,
		},
		{
			name:       "no match",
			textLower:  "hello world",
			expression: "comprehend",
			definition: "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsExpression(tt.textLower, tt.expression, tt.definition)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewInvalidArgumentError(t *testing.T) {
	connectErr := newInvalidArgumentError("field_name", "field is required")

	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	assert.Contains(t, connectErr.Message(), "field_name")
	assert.Greater(t, len(connectErr.Details()), 0)
}
