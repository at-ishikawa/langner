package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestValidateInput(t *testing.T) {
	tests := []struct {
		name        string
		word        string
		meaning     string
		expectError bool
		errorType   error
	}{
		{
			name:        "Valid input",
			word:        "test",
			meaning:     "examination",
			expectError: false,
		},
		{
			name:        "Empty word",
			word:        "",
			meaning:     "meaning",
			expectError: true,
			errorType:   ErrEmptyWord,
		},
		{
			name:        "Empty meaning",
			word:        "word",
			meaning:     "",
			expectError: true,
			errorType:   ErrEmptyMeaning,
		},
		{
			name:        "Both empty",
			word:        "",
			meaning:     "",
			expectError: true,
			errorType:   ErrEmptyWord, // Word is checked first
		},
		{
			name:        "Whitespace word",
			word:        "   ",
			meaning:     "meaning",
			expectError: false, // ValidateInput doesn't trim
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInput(tt.word, tt.meaning)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorType, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{Message: "test error"}
	assert.Equal(t, "test error", err.Error())
}

func TestFreeformQuizCLI_displayResult(t *testing.T) {
	tests := []struct {
		name                string
		grade               quiz.FreeformGradeResult
		wantMeaningInOutput string
		wantNotInOutput     string
		wantContextInOutput string
		wantReasonInOutput  string
	}{
		{
			name: "Correct answer shows meaning and reason",
			grade: quiz.FreeformGradeResult{
				Correct: true,
				Word:    "trinket",
				Meaning: "a small ornament",
				Reason:  "partial match: user captured main sense",
			},
			wantMeaningInOutput: "a small ornament",
			wantReasonInOutput:  "partial match: user captured main sense",
		},
		{
			name: "Incorrect answer shows correct meaning and context",
			grade: quiz.FreeformGradeResult{
				Correct: false,
				Word:    "trinket",
				Meaning: "a small ornament or piece of jewelry",
				Context: "She wore a beautiful trinket on her necklace",
				Reason:  "unrelated: user said 'container' but it means 'small ornament'",
			},
			wantMeaningInOutput: "a small ornament or piece of jewelry",
			wantContextInOutput: "She wore a beautiful trinket on her necklace",
			wantReasonInOutput:  "unrelated: user said 'container' but it means 'small ornament'",
		},
		{
			name: "Correct answer with context",
			grade: quiz.FreeformGradeResult{
				Correct: true,
				Word:    "trinket",
				Meaning: "a small ornament",
				Context: "She wore a beautiful trinket on her necklace",
			},
			wantMeaningInOutput: "a small ornament",
			wantContextInOutput: "She wore a beautiful trinket on her necklace",
		},
		{
			name: "Incorrect answer with no context",
			grade: quiz.FreeformGradeResult{
				Correct: false,
				Word:    "test",
				Meaning: "wrong meaning",
			},
			wantMeaningInOutput: "wrong meaning",
		},
		{
			name: "Correct answer without reason or context",
			grade: quiz.FreeformGradeResult{
				Correct: true,
				Word:    "gather",
				Meaning: "to collect or bring together",
			},
			wantMeaningInOutput: "to collect or bring together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Disable color for testing
			color.NoColor = true
			defer func() { color.NoColor = false }()

			// Create buffer to capture output
			var buf bytes.Buffer

			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					stdoutWriter: &buf,
					bold:         color.New(color.Bold),
					italic:       color.New(color.Italic),
				},
			}

			cli.displayFreeformResult(tt.grade)

			outputStr := buf.String()
			assert.Contains(t, outputStr, tt.wantMeaningInOutput, "Output should contain the expected meaning")

			if tt.wantNotInOutput != "" {
				assert.NotContains(t, outputStr, tt.wantNotInOutput, "Output should not contain the wrong meaning")
			}

			if tt.wantContextInOutput != "" {
				assert.Contains(t, outputStr, tt.wantContextInOutput, "Output should contain the context")
			}

			if tt.wantReasonInOutput != "" {
				assert.Contains(t, outputStr, tt.wantReasonInOutput, "Output should contain the reason")
			}
		})
	}
}

func TestFreeformQuizCLI_Run(t *testing.T) {
	tests := []struct {
		name          string
		answerCorrect bool
		wantFileSaved bool
		wantStatus    notebook.LearnedStatus
	}{
		{
			name:          "Always saves on correct answer",
			answerCorrect: true,
			wantFileSaved: true,
			wantStatus:    "understood",
		},
		{
			name:          "Does not save on incorrect answer",
			answerCorrect: false,
			wantFileSaved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := mock_inference.NewMockClient(ctrl)

			matchedCard := quiz.FreeformCard{
				NotebookName: "test-notebook",
				StoryTitle:   "Test Story",
				SceneTitle:   "Test Scene",
				Expression:   "test",
				Meaning:      "a procedure to check something",
			}

			freeformCards := []quiz.FreeformCard{matchedCard}

			quality := 4
			if !tt.answerCorrect {
				quality = 1
			}

			grade := quiz.FreeformGradeResult{
				Correct:      tt.answerCorrect,
				Word:         "test",
				Meaning:      "a procedure to check something",
				Quality:      quality,
				NotebookName: "test-notebook",
			}

			if tt.answerCorrect {
				grade.MatchedCard = &matchedCard
			}

			svc := quiz.NewService(
				config.NotebooksConfig{
					LearningNotesDirectory: tmpDir,
				},
				mockClient,
				nil,
				learning.NewYAMLLearningRepository(tmpDir, nil),
				config.QuizConfig{},
			)

			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					stdoutWriter: os.Stdout,
				},
				svc:           svc,
				freeformCards: freeformCards,
			}

			// Simulate the save path: if grade has MatchedCard, call SaveFreeformResult
			if grade.MatchedCard != nil {
				err := cli.svc.SaveFreeformResult(context.Background(), *grade.MatchedCard, grade, 1000)
				require.NoError(t, err)
			}

			// Verify file saved based on expectation
			learningNotePath := filepath.Join(tmpDir, "test-notebook.yml")
			_, err := os.Stat(learningNotePath)
			if !tt.wantFileSaved {
				assert.True(t, os.IsNotExist(err), "Learning note file should not be created for incorrect answer")
				return
			}

			require.NoError(t, err, "Learning note file should be created after correct answer")

			// Read and verify the saved history
			histories, err := notebook.NewLearningHistories(tmpDir)
			require.NoError(t, err)

			// Verify the word was recorded with correct status
			gotStatus := findExpressionStatus(histories["test-notebook"], "Test Story", "Test Scene", "test")
			require.NotNil(t, gotStatus, "The word 'test' should be recorded in learning history")
			assert.Equal(t, tt.wantStatus, *gotStatus)
		})
	}
}

func findExpressionStatus(histories []notebook.LearningHistory, storyTitle, sceneTitle, expression string) *notebook.LearnedStatus {
	for _, history := range histories {
		if history.Metadata.Title != storyTitle {
			continue
		}
		for _, scene := range history.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if expr.Expression != expression {
					continue
				}
				status := expr.GetLatestStatus()
				return &status
			}
		}
	}
	return nil
}

func TestFreeformQuizCLI_session(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		freeformCards []quiz.FreeformCard
		setupMock     func(*mock_inference.MockClient)
		wantErr       bool
	}{
		{
			name:  "Quit command",
			input: "quit\n",
			setupMock: func(mockClient *mock_inference.MockClient) {
				// No expectation - quit before any API call
			},
		},
		{
			name:  "Exit command",
			input: "exit\n",
			setupMock: func(mockClient *mock_inference.MockClient) {
				// No expectation - exit before any API call
			},
		},
		{
			name:  "Empty word triggers validation error",
			input: "\ntest meaning\n",
			setupMock: func(mockClient *mock_inference.MockClient) {
				// No expectation - validation fails before API call
			},
		},
		{
			name:  "Empty meaning triggers validation error",
			input: "test\n\n",
			setupMock: func(mockClient *mock_inference.MockClient) {
				// No expectation - validation fails before API call
			},
		},
		{
			name:          "Word not found in cards",
			input:         "unknown\ntest meaning\n",
			freeformCards: []quiz.FreeformCard{},
			setupMock: func(mockClient *mock_inference.MockClient) {
				// No expectation - GradeFreeformAnswer returns early when no cards match
			},
		},
		{
			name:  "Correct answer - calls grade and save",
			input: "test\ntest meaning\n",
			freeformCards: []quiz.FreeformCard{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Expression:   "test",
					Meaning:      "test meaning",
				},
			},
			setupMock: func(mockClient *mock_inference.MockClient) {
				mockClient.EXPECT().
					AnswerMeanings(gomock.Any(), gomock.Any()).
					Return(inference.AnswerMeaningsResponse{
						Answers: []inference.AnswerMeaning{
							{
								Expression: "test",
								Meaning:    "test meaning",
								AnswersForContext: []inference.AnswersForContext{
									{Correct: true, Context: "This is a test", Reason: "exact match with reference"},
								},
							},
						},
					}, nil).
					Times(1)
			},
		},
		{
			name:  "AnswerMeanings API error",
			input: "test\ntest meaning\n",
			freeformCards: []quiz.FreeformCard{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Expression:   "test",
					Meaning:      "test meaning",
				},
			},
			setupMock: func(mockClient *mock_inference.MockClient) {
				mockClient.EXPECT().
					AnswerMeanings(gomock.Any(), gomock.Any()).
					Return(inference.AnswerMeaningsResponse{}, fmt.Errorf("API error")).
					Times(1)
			},
			wantErr: true,
		},
		{
			name:  "AnswerMeanings returns empty answers",
			input: "test\ntest meaning\n",
			freeformCards: []quiz.FreeformCard{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Expression:   "test",
					Meaning:      "test meaning",
				},
			},
			setupMock: func(mockClient *mock_inference.MockClient) {
				mockClient.EXPECT().
					AnswerMeanings(gomock.Any(), gomock.Any()).
					Return(inference.AnswerMeaningsResponse{Answers: []inference.AnswerMeaning{}}, nil).
					Times(1)
			},
			wantErr: true,
		},
		{
			name:  "Incorrect answer - no save",
			input: "test\nwrong meaning\n",
			freeformCards: []quiz.FreeformCard{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Expression:   "test",
					Meaning:      "test meaning",
				},
			},
			setupMock: func(mockClient *mock_inference.MockClient) {
				mockClient.EXPECT().
					AnswerMeanings(gomock.Any(), gomock.Any()).
					Return(inference.AnswerMeaningsResponse{
						Answers: []inference.AnswerMeaning{
							{
								Expression: "test",
								Meaning:    "wrong meaning",
								AnswersForContext: []inference.AnswersForContext{
									{Correct: false, Context: "This is a test", Reason: "unrelated: meanings are from different semantic fields"},
								},
							},
						},
					}, nil).
					Times(1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			stdinReader := bufio.NewReader(strings.NewReader(tt.input))
			mockClient := mock_inference.NewMockClient(ctrl)

			// Setup mock expectations
			tt.setupMock(mockClient)

			tmpDir := t.TempDir()

			svc := quiz.NewService(
				config.NotebooksConfig{
					LearningNotesDirectory: tmpDir,
				},
				mockClient,
				nil,
				learning.NewYAMLLearningRepository(tmpDir, nil),
				config.QuizConfig{},
			)

			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					stdinReader:  stdinReader,
					stdoutWriter: os.Stdout,
					bold:         color.New(color.Bold),
					italic:       color.New(color.Italic),
				},
				svc:           svc,
				freeformCards: tt.freeformCards,
			}

			err := cli.Session(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewFreeformQuizCLI(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T) (storiesDir, flashcardsDir, learningNotesDir string)
		wantWordCount int
		wantErr       bool
	}{
		{
			name: "loads story notebooks",
			setupFunc: func(t *testing.T) (string, string, string) {
				t.Helper()
				storiesDir := t.TempDir()
				learningNotesDir := t.TempDir()

				storyDir := filepath.Join(storiesDir, "test-story")
				require.NoError(t, os.MkdirAll(storyDir, 0755))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(storyDir, "index.yml"), notebook.Index{
					Kind: "story", ID: "test-story", Name: "Test Story",
					NotebookPaths: []string{"stories.yml"},
				}))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(storyDir, "stories.yml"), []notebook.StoryNotebook{
					{
						Event: "Episode 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title:         "Scene 1",
								Conversations: []notebook.Conversation{{Speaker: "A", Quote: "A difficult situation."}},
								Definitions:   []notebook.Note{{Expression: "difficult", Meaning: "hard to deal with"}},
							},
						},
					},
				}))
				return storiesDir, "", learningNotesDir
			},
			wantWordCount: 1,
		},
		{
			name: "loads flashcard notebooks",
			setupFunc: func(t *testing.T) (string, string, string) {
				t.Helper()
				flashcardsDir := t.TempDir()
				learningNotesDir := t.TempDir()

				flashcardDir := filepath.Join(flashcardsDir, "test-flashcard")
				require.NoError(t, os.MkdirAll(flashcardDir, 0755))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "index.yml"), notebook.FlashcardIndex{
					ID: "test-flashcard", Name: "Test Flashcards",
					NotebookPaths: []string{"cards.yml"},
				}))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "cards.yml"), []notebook.FlashcardNotebook{
					{
						Title: "Common Words",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Cards: []notebook.Note{
							{Expression: "break the ice", Meaning: "to initiate social interaction"},
						},
					},
				}))
				return "", flashcardsDir, learningNotesDir
			},
			wantWordCount: 1,
		},
		{
			name: "empty directories",
			setupFunc: func(t *testing.T) (string, string, string) {
				return t.TempDir(), t.TempDir(), t.TempDir()
			},
			wantWordCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storiesDir, flashcardsDir, learningNotesDir := tt.setupFunc(t)
			dictionaryCacheDir := t.TempDir()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := mock_inference.NewMockClient(ctrl)

			var storiesDirs, flashcardsDirs []string
			if storiesDir != "" {
				storiesDirs = []string{storiesDir}
			}
			if flashcardsDir != "" {
				flashcardsDirs = []string{flashcardsDir}
			}

			got, err := NewFreeformQuizCLI(
				config.NotebooksConfig{
					StoriesDirectories:     storiesDirs,
					FlashcardsDirectories:  flashcardsDirs,
					LearningNotesDirectory: learningNotesDir,
				},
				dictionaryCacheDir,
				mockClient,
				config.QuizConfig{},
			)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantWordCount, got.WordCount())
		})
	}
}

func TestWordCount(t *testing.T) {
	tests := []struct {
		name          string
		freeformCards []quiz.FreeformCard
		want          int
	}{
		{
			name:          "empty",
			freeformCards: []quiz.FreeformCard{},
			want:          0,
		},
		{
			name: "multiple cards counted",
			freeformCards: []quiz.FreeformCard{
				{Expression: "a", Meaning: "first"},
				{Expression: "b", Meaning: "second"},
			},
			want: 2,
		},
		{
			name: "single card",
			freeformCards: []quiz.FreeformCard{
				{Expression: "hello", Meaning: "a greeting"},
			},
			want: 1,
		},
		{
			name:          "nil cards",
			freeformCards: nil,
			want:          0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{},
				freeformCards:      tt.freeformCards,
			}
			assert.Equal(t, tt.want, cli.WordCount())
		})
	}
}
