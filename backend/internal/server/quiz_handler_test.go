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
	"google.golang.org/protobuf/proto"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
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
			LearningNotesDirectory: learningNotesDir,
		},
	}

	return NewQuizHandler(cfg, openaiClient, make(map[string]rapidapi.Response))
}

func newTestHandlerWithFixtures(t *testing.T, openaiClient inference.Client) *QuizHandler {
	t.Helper()

	storiesDir := t.TempDir()
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	// Create story fixtures
	storyDir := filepath.Join(storiesDir, "test-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: test-story
name: Test Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "That sounds preposterous to me."
      definitions:
        - expression: "preposterous"
          meaning: "contrary to reason or common sense"
`), 0644))

	// Create flashcard fixtures
	vocabDir := filepath.Join(flashcardsDir, "test-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: test-vocab
name: Test Vocabulary
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "serendipity"
      meaning: "a fortunate discovery by accident"
      examples:
        - "It was pure serendipity that they met."
`), 0644))

	cfg := &config.Config{
		Notebooks: config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			FlashcardsDirectories:  []string{flashcardsDir},
			LearningNotesDirectory: learningDir,
		},
	}

	return NewQuizHandler(cfg, openaiClient, make(map[string]rapidapi.Response))
}

func TestQuizHandler_GetQuizOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandler(t, mockClient)

	resp, err := handler.GetQuizOptions(
		context.Background(),
		connect.NewRequest(&apiv1.GetQuizOptionsRequest{}),
	)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Empty(t, resp.Msg.GetNotebooks())
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
				connect.NewRequest(&apiv1.StartQuizRequest{
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

func TestQuizHandler_GetQuizOptions_WithFixtures(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandlerWithFixtures(t, mockClient)

	resp, err := handler.GetQuizOptions(
		context.Background(),
		connect.NewRequest(&apiv1.GetQuizOptionsRequest{}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	notebooks := resp.Msg.GetNotebooks()
	assert.Len(t, notebooks, 2)

	summaryMap := make(map[string]*apiv1.NotebookSummary)
	for _, nb := range notebooks {
		summaryMap[nb.GetNotebookId()] = nb
	}

	storySummary := summaryMap["test-story"]
	require.NotNil(t, storySummary)
	assert.Equal(t, "Test Story", storySummary.GetName())
	assert.Equal(t, int32(1), storySummary.GetReviewCount())

	vocabSummary := summaryMap["test-vocab"]
	require.NotNil(t, vocabSummary)
	assert.Equal(t, "Test Vocabulary", vocabSummary.GetName())
	assert.Equal(t, int32(1), vocabSummary.GetReviewCount())
}

func TestQuizHandler_StartQuiz_WithStoryNotebook(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandlerWithFixtures(t, mockClient)

	resp, err := handler.StartQuiz(
		context.Background(),
		connect.NewRequest(&apiv1.StartQuizRequest{
			NotebookIds:      []string{"test-story"},
			IncludeUnstudied: true,
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	flashcards := resp.Msg.GetFlashcards()
	require.Len(t, flashcards, 1)
	assert.Equal(t, "preposterous", flashcards[0].GetEntry())
	assert.Greater(t, flashcards[0].GetNoteId(), int64(0))
	// buildFromConversations should find the conversation containing "preposterous"
	assert.NotEmpty(t, flashcards[0].GetExamples())
}

func TestQuizHandler_StartQuiz_WithFlashcardNotebook(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandlerWithFixtures(t, mockClient)

	resp, err := handler.StartQuiz(
		context.Background(),
		connect.NewRequest(&apiv1.StartQuizRequest{
			NotebookIds:      []string{"test-vocab"},
			IncludeUnstudied: true,
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	flashcards := resp.Msg.GetFlashcards()
	require.Len(t, flashcards, 1)
	assert.Equal(t, "serendipity", flashcards[0].GetEntry())
	assert.Greater(t, flashcards[0].GetNoteId(), int64(0))
	require.Len(t, flashcards[0].GetExamples(), 1)
	assert.Equal(t, "It was pure serendipity that they met.", flashcards[0].GetExamples()[0].GetText())
}

func TestQuizHandler_StartQuiz_WithMultipleNotebooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandlerWithFixtures(t, mockClient)

	resp, err := handler.StartQuiz(
		context.Background(),
		connect.NewRequest(&apiv1.StartQuizRequest{
			NotebookIds:      []string{"test-story", "test-vocab"},
			IncludeUnstudied: true,
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Msg.GetFlashcards(), 2)
}

func TestQuizHandler_StartQuiz_WithDefinitionField(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)

	storiesDir := t.TempDir()
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	// Create story with definition field set (covers expression = definition.Definition branch)
	storyDir := filepath.Join(storiesDir, "test-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: test-story
name: Test Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "He ran away quickly."
      definitions:
        - expression: "run"
          definition: "ran"
          meaning: "to move swiftly on foot"
`), 0644))

	// Create flashcard with definition field set
	vocabDir := filepath.Join(flashcardsDir, "test-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: test-vocab
name: Test Vocabulary
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Verb Forms"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "go"
      definition: "went"
      meaning: "to move or travel"
      examples:
        - "She went to the store."
`), 0644))

	cfg := &config.Config{
		Notebooks: config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			FlashcardsDirectories:  []string{flashcardsDir},
			LearningNotesDirectory: learningDir,
		},
	}
	handler := NewQuizHandler(cfg, mockClient, make(map[string]rapidapi.Response))

	// Test story with definition field
	t.Run("story uses definition field as entry", func(t *testing.T) {
		resp, err := handler.StartQuiz(
			context.Background(),
			connect.NewRequest(&apiv1.StartQuizRequest{
				NotebookIds:      []string{"test-story"},
				IncludeUnstudied: true,
			}),
		)
		require.NoError(t, err)
		require.NotNil(t, resp)

		flashcards := resp.Msg.GetFlashcards()
		require.Len(t, flashcards, 1)
		assert.Equal(t, "ran", flashcards[0].GetEntry())
	})

	// Test flashcard with definition field
	t.Run("flashcard uses definition field as entry", func(t *testing.T) {
		resp, err := handler.StartQuiz(
			context.Background(),
			connect.NewRequest(&apiv1.StartQuizRequest{
				NotebookIds:      []string{"test-vocab"},
				IncludeUnstudied: true,
			}),
		)
		require.NoError(t, err)
		require.NotNil(t, resp)

		flashcards := resp.Msg.GetFlashcards()
		require.Len(t, flashcards, 1)
		assert.Equal(t, "went", flashcards[0].GetEntry())
	})
}

func TestQuizHandler_SubmitAnswer_UpdatesLearningHistory(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler := newTestHandlerWithFixtures(t, mockClient)

	// Start a quiz to populate the note store
	_, err := handler.StartQuiz(
		context.Background(),
		connect.NewRequest(&apiv1.StartQuizRequest{
			NotebookIds:      []string{"test-vocab"},
			IncludeUnstudied: true,
		}),
	)
	require.NoError(t, err)

	// Find the note ID for "serendipity"
	var noteID int64
	handler.mu.Lock()
	for id, note := range handler.noteStore {
		if note.expression == "serendipity" {
			noteID = id
			break
		}
	}
	handler.mu.Unlock()
	require.Greater(t, noteID, int64(0))

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{
			Answers: []inference.AnswerMeaning{
				{
					Expression: "serendipity",
					Meaning:    "a fortunate discovery by accident",
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

	resp, err := handler.SubmitAnswer(
		context.Background(),
		connect.NewRequest(&apiv1.SubmitAnswerRequest{
			NoteId:         noteID,
			Answer:         "a fortunate discovery by accident",
			ResponseTimeMs: 1000,
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.GetCorrect())
	assert.Equal(t, "a fortunate discovery by accident", resp.Msg.GetMeaning())

	// Verify learning history file was created
	learningDir := handler.cfg.Notebooks.LearningNotesDirectory
	historyPath := filepath.Join(learningDir, "test-vocab.yml")
	_, err = os.Stat(historyPath)
	assert.NoError(t, err)
}

func TestQuizHandler_SubmitAnswer_UpdateLearningHistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)

	learningDir := t.TempDir()

	cfg := &config.Config{
		Notebooks: config.NotebooksConfig{
			LearningNotesDirectory: learningDir,
		},
	}
	handler := NewQuizHandler(cfg, mockClient, make(map[string]rapidapi.Response))

	// Place a malformed YAML file in the learning directory to trigger error in updateLearningHistory
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-notebook.yml"), []byte("{{invalid yaml"), 0644))

	// Manually populate the note store
	handler.noteStore[1] = &quizNote{
		notebookName: "test-notebook",
		storyTitle:   "flashcards",
		expression:   "comprehend",
		meaning:      "to understand completely",
	}

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{
			Answers: []inference.AnswerMeaning{
				{
					Expression: "comprehend",
					Meaning:    "to understand completely",
					AnswersForContext: []inference.AnswersForContext{
						{
							Correct: true,
							Reason:  "Good answer.",
							Quality: 4,
						},
					},
				},
			},
		}, nil,
	)

	resp, err := handler.SubmitAnswer(
		context.Background(),
		connect.NewRequest(&apiv1.SubmitAnswerRequest{
			NoteId:         1,
			Answer:         "to understand",
			ResponseTimeMs: 1000,
		}),
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

func TestQuizHandler_GetQuizOptions_LearningHistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)

	learningDir := t.TempDir()

	cfg := &config.Config{
		Notebooks: config.NotebooksConfig{
			LearningNotesDirectory: learningDir,
		},
	}
	handler := NewQuizHandler(cfg, mockClient, make(map[string]rapidapi.Response))

	// Place a malformed YAML file in the learning directory
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "broken.yml"), []byte("{{invalid yaml"), 0644))

	resp, err := handler.GetQuizOptions(
		context.Background(),
		connect.NewRequest(&apiv1.GetQuizOptionsRequest{}),
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

func TestQuizHandler_GetQuizOptions_FilterError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)

	storiesDir := t.TempDir()
	learningDir := t.TempDir()

	// Create a story with an empty expression to trigger FilterStoryNotebooks error
	storyDir := filepath.Join(storiesDir, "bad-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: bad-story
name: Bad Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "Hello there."
      definitions:
        - expression: ""
          meaning: "should fail validation"
`), 0644))

	cfg := &config.Config{
		Notebooks: config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			LearningNotesDirectory: learningDir,
		},
	}
	handler := NewQuizHandler(cfg, mockClient, make(map[string]rapidapi.Response))

	resp, err := handler.GetQuizOptions(
		context.Background(),
		connect.NewRequest(&apiv1.GetQuizOptionsRequest{}),
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

func TestQuizHandler_StartQuiz_FilterFlashcardError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)

	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	// Create a flashcard with an empty expression to trigger FilterFlashcardNotebooks error
	vocabDir := filepath.Join(flashcardsDir, "bad-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: bad-vocab
name: Bad Vocabulary
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Bad Cards"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: ""
      meaning: "should fail validation"
`), 0644))

	cfg := &config.Config{
		Notebooks: config.NotebooksConfig{
			FlashcardsDirectories:  []string{flashcardsDir},
			LearningNotesDirectory: learningDir,
		},
	}
	handler := NewQuizHandler(cfg, mockClient, make(map[string]rapidapi.Response))

	resp, err := handler.StartQuiz(
		context.Background(),
		connect.NewRequest(&apiv1.StartQuizRequest{
			NotebookIds:      []string{"bad-vocab"},
			IncludeUnstudied: true,
		}),
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

func TestQuizHandler_StartQuiz_FilterStoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)

	storiesDir := t.TempDir()
	learningDir := t.TempDir()

	// Create a story with an empty expression to trigger FilterStoryNotebooks error in loadStoryCards
	storyDir := filepath.Join(storiesDir, "bad-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: bad-story
name: Bad Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "Hello there."
      definitions:
        - expression: ""
          meaning: "should fail validation"
`), 0644))

	cfg := &config.Config{
		Notebooks: config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			LearningNotesDirectory: learningDir,
		},
	}
	handler := NewQuizHandler(cfg, mockClient, make(map[string]rapidapi.Response))

	resp, err := handler.StartQuiz(
		context.Background(),
		connect.NewRequest(&apiv1.StartQuizRequest{
			NotebookIds:      []string{"bad-story"},
			IncludeUnstudied: true,
		}),
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

func TestQuizHandler_StartQuiz_LearningHistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)

	storiesDir := t.TempDir()
	learningDir := t.TempDir()

	// Create a valid story directory so the notebook is found
	storyDir := filepath.Join(storiesDir, "test-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: test-story
name: Test Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "Hello there."
      definitions:
        - expression: "hello"
          meaning: "a greeting"
`), 0644))

	cfg := &config.Config{
		Notebooks: config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			LearningNotesDirectory: learningDir,
		},
	}
	handler := NewQuizHandler(cfg, mockClient, make(map[string]rapidapi.Response))

	// Place a malformed YAML file in the learning directory
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "broken.yml"), []byte("{{invalid yaml"), 0644))

	resp, err := handler.StartQuiz(
		context.Background(),
		connect.NewRequest(&apiv1.StartQuizRequest{
			NotebookIds:      []string{"test-story"},
			IncludeUnstudied: true,
		}),
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
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
		{
			name:   "returns INTERNAL when inference returns empty answers",
			noteID: 1,
			answer: "some answer",
			setupNoteStore: func(h *QuizHandler) {
				h.noteStore[1] = &quizNote{
					notebookName: "test-notebook",
					storyTitle:   "flashcards",
					expression:   "comprehend",
					meaning:      "to understand completely",
				}
			},
			setupMock: func(m *mock_inference.MockClient) {
				m.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
					inference.AnswerMeaningsResponse{Answers: nil}, nil,
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
				connect.NewRequest(&apiv1.SubmitAnswerRequest{
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

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name          string
		msg           proto.Message
		wantErr       bool
		wantDetailLen int
	}{
		{
			name:    "valid StartQuizRequest passes",
			msg:     &apiv1.StartQuizRequest{NotebookIds: []string{"nb-1"}},
			wantErr: false,
		},
		{
			name:          "empty notebook_ids returns error with detail",
			msg:           &apiv1.StartQuizRequest{},
			wantErr:       true,
			wantDetailLen: 1,
		},
		{
			name:    "valid SubmitAnswerRequest passes",
			msg:     &apiv1.SubmitAnswerRequest{NoteId: 1, Answer: "hello"},
			wantErr: false,
		},
		{
			name:          "zero note_id returns error with detail",
			msg:           &apiv1.SubmitAnswerRequest{NoteId: 0, Answer: "hello"},
			wantErr:       true,
			wantDetailLen: 1,
		},
		{
			name:          "empty answer returns error with detail",
			msg:           &apiv1.SubmitAnswerRequest{NoteId: 1, Answer: ""},
			wantErr:       true,
			wantDetailLen: 1,
		},
		{
			name:          "whitespace-only answer returns error with detail",
			msg:           &apiv1.SubmitAnswerRequest{NoteId: 1, Answer: "   "},
			wantErr:       true,
			wantDetailLen: 1,
		},
		{
			name:          "multiple violations returns error with detail",
			msg:           &apiv1.SubmitAnswerRequest{NoteId: 0, Answer: ""},
			wantErr:       true,
			wantDetailLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateRequest(tt.msg)
			if !tt.wantErr {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, connect.CodeInvalidArgument, got.Code())
			assert.Len(t, got.Details(), tt.wantDetailLen)
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

func TestBuildFromConversations(t *testing.T) {
	tests := []struct {
		name         string
		scene        notebook.StoryScene
		definition   notebook.Note
		wantExamples int
	}{
		{
			name: "skips empty quotes",
			scene: notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "Alice", Quote: ""},
					{Speaker: "Bob", Quote: "This is absolutely preposterous."},
				},
			},
			definition: notebook.Note{
				Expression: "preposterous",
				Meaning:    "absurd",
			},
			wantExamples: 1,
		},
		{
			name: "skips non-matching quotes",
			scene: notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "Alice", Quote: "Hello there."},
					{Speaker: "Bob", Quote: "Good morning."},
				},
			},
			definition: notebook.Note{
				Expression: "preposterous",
				Meaning:    "absurd",
			},
			wantExamples: 0,
		},
		{
			name: "matches multiple quotes containing expression",
			scene: notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "Alice", Quote: "That is preposterous!"},
					{Speaker: "Bob", Quote: "I agree, totally preposterous."},
				},
			},
			definition: notebook.Note{
				Expression: "preposterous",
				Meaning:    "absurd",
			},
			wantExamples: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			examples, contexts := buildFromConversations(&tt.scene, &tt.definition)
			assert.Len(t, examples, tt.wantExamples)
			assert.Len(t, contexts, tt.wantExamples)
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
