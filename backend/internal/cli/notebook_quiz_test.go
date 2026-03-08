package cli

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/internal/testutil"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewNotebookQuizCLI(t *testing.T) {
	tests := []struct {
		name              string
		setupFunc         func(t *testing.T) (storiesDir, learningNotesDir string)
		notebookName      string
		expectedCardCount int
		validate          func(t *testing.T, cli *NotebookQuizCLI)
	}{
		{
			name: "Usable word past review interval - word INCLUDED",
			setupFunc: func(t *testing.T) (string, string) {
				storiesDir := t.TempDir()
				learningNotesDir := t.TempDir()

				// Create notebook directory structure
				notebookDir := filepath.Join(storiesDir, "test-notebook")
				require.NoError(t, os.MkdirAll(notebookDir, 0755))

				// Create index.yml
				index := notebook.Index{
					Kind:          "story",
					ID:            "test-notebook",
					Name:          "Test Notebook",
					NotebookPaths: []string{"stories.yml"},
				}
				indexPath := filepath.Join(notebookDir, "index.yml")
				require.NoError(t, notebook.WriteYamlFile(indexPath, index))

				// Create stories.yml
				stories := []notebook.StoryNotebook{
					{
						Event: "Story 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "This is a test word"},
								},
								Definitions: []notebook.Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				}
				storiesPath := filepath.Join(notebookDir, "stories.yml")
				require.NoError(t, notebook.WriteYamlFile(storiesPath, stories))

				// Create learning history with usable word 4 days ago (past 3-day threshold)
				learningHistory := []notebook.LearningHistory{
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "test-notebook",
							Title:      "Story 1",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "test",
										LearnedLogs: []notebook.LearningRecord{
											{Status: "usable", LearnedAt: notebook.NewDate(time.Now().Add(-4 * 24 * time.Hour))},
										},
									},
								},
							},
						},
					},
				}
				learningHistoryPath := filepath.Join(learningNotesDir, "test-notebook.yml")
				require.NoError(t, notebook.WriteYamlFile(learningHistoryPath, learningHistory))

				return storiesDir, learningNotesDir
			},
			notebookName:      "test-notebook",
			expectedCardCount: 1,
			validate: func(t *testing.T, cli *NotebookQuizCLI) {
				assert.Equal(t, 1, len(cli.cards))
				assert.Equal(t, "test", cli.cards[0].Entry)
			},
		},
		{
			name: "Usable word within review interval - word NOT included",
			setupFunc: func(t *testing.T) (string, string) {
				storiesDir := t.TempDir()
				learningNotesDir := t.TempDir()

				// Create notebook directory structure
				notebookDir := filepath.Join(storiesDir, "test-notebook")
				require.NoError(t, os.MkdirAll(notebookDir, 0755))

				// Create index.yml
				index := notebook.Index{
					Kind:          "story",
					ID:            "test-notebook",
					Name:          "Test Notebook",
					NotebookPaths: []string{"stories.yml"},
				}
				indexPath := filepath.Join(notebookDir, "index.yml")
				require.NoError(t, notebook.WriteYamlFile(indexPath, index))

				// Create stories.yml
				stories := []notebook.StoryNotebook{
					{
						Event: "Story 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "This is a test word"},
								},
								Definitions: []notebook.Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				}
				storiesPath := filepath.Join(notebookDir, "stories.yml")
				require.NoError(t, notebook.WriteYamlFile(storiesPath, stories))

				// Create learning history with usable word 1 day ago (within 3-day threshold)
				learningHistory := []notebook.LearningHistory{
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "test-notebook",
							Title:      "Story 1",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "test",
										LearnedLogs: []notebook.LearningRecord{
											{Status: "usable", LearnedAt: notebook.NewDate(time.Now().Add(-1 * 24 * time.Hour))},
										},
									},
								},
							},
						},
					},
				}
				learningHistoryPath := filepath.Join(learningNotesDir, "test-notebook.yml")
				require.NoError(t, notebook.WriteYamlFile(learningHistoryPath, learningHistory))

				return storiesDir, learningNotesDir
			},
			notebookName:      "test-notebook",
			expectedCardCount: 0,
		},
		{
			name: "Misunderstood word always included",
			setupFunc: func(t *testing.T) (string, string) {
				storiesDir := t.TempDir()
				learningNotesDir := t.TempDir()
				testutil.CreateStoryNotebook(t, storiesDir, learningNotesDir, "test-notebook")
				return storiesDir, learningNotesDir
			},
			notebookName:      "test-notebook",
			expectedCardCount: 1,
			validate: func(t *testing.T, cli *NotebookQuizCLI) {
				assert.Equal(t, 1, len(cli.cards))
				assert.Equal(t, "eager", cli.cards[0].Entry)
			},
		},
		{
			name: "No learning history - word included",
			setupFunc: func(t *testing.T) (string, string) {
				storiesDir := t.TempDir()
				learningNotesDir := t.TempDir()

				// Create notebook directory structure
				notebookDir := filepath.Join(storiesDir, "test-notebook")
				require.NoError(t, os.MkdirAll(notebookDir, 0755))

				// Create index.yml
				index := notebook.Index{
					Kind:          "story",
					ID:            "test-notebook",
					Name:          "Test Notebook",
					NotebookPaths: []string{"stories.yml"},
				}
				indexPath := filepath.Join(notebookDir, "index.yml")
				require.NoError(t, notebook.WriteYamlFile(indexPath, index))

				// Create stories.yml
				stories := []notebook.StoryNotebook{
					{
						Event: "Story 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "This is a test word"},
								},
								Definitions: []notebook.Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				}
				storiesPath := filepath.Join(notebookDir, "stories.yml")
				require.NoError(t, notebook.WriteYamlFile(storiesPath, stories))

				// Create empty learning history (expression exists but no logs)
				learningHistory := []notebook.LearningHistory{
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "test-notebook",
							Title:      "Story 1",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression:  "test",
										LearnedLogs: []notebook.LearningRecord{},
									},
								},
							},
						},
					},
				}
				learningHistoryPath := filepath.Join(learningNotesDir, "test-notebook.yml")
				require.NoError(t, notebook.WriteYamlFile(learningHistoryPath, learningHistory))

				return storiesDir, learningNotesDir
			},
			notebookName:      "test-notebook",
			expectedCardCount: 1,
			validate: func(t *testing.T, cli *NotebookQuizCLI) {
				assert.Equal(t, 1, len(cli.cards))
				assert.Equal(t, "test", cli.cards[0].Entry)
			},
		},
		{
			name: "All notebooks - empty notebookName loads all stories",
			setupFunc: func(t *testing.T) (string, string) {
				storiesDir := t.TempDir()
				learningNotesDir := t.TempDir()
				testutil.CreateStoryNotebook(t, storiesDir, learningNotesDir, "test-notebook")
				return storiesDir, learningNotesDir
			},
			notebookName:      "",
			expectedCardCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storiesDir, learningNotesDir := tt.setupFunc(t)
			dictionaryCacheDir := t.TempDir()

			// Create mock OpenAI client
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := mock_inference.NewMockClient(ctrl)

			cli, err := NewNotebookQuizCLI(
				tt.notebookName,
				config.NotebooksConfig{
					StoriesDirectories:     []string{storiesDir},
					LearningNotesDirectory: learningNotesDir,
				},
				dictionaryCacheDir,
				mockClient,
				true, // includeNoCorrectAnswers
			)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedCardCount, cli.GetCardCount())

			if tt.validate != nil {
				tt.validate(t, cli)
			}
		})
	}
}

func TestNewFlashcardQuizCLI(t *testing.T) {
	tests := []struct {
		name              string
		setupFunc         func(t *testing.T) (flashcardsDir, learningNotesDir string)
		notebookName      string
		expectedCardCount int
		wantErr           bool
	}{
		{
			name: "loads flashcard with learning history - misunderstood word included",
			setupFunc: func(t *testing.T) (string, string) {
				flashcardsDir := t.TempDir()
				learningNotesDir := t.TempDir()
				testutil.CreateFlashcardNotebook(t, flashcardsDir, learningNotesDir, "test-flashcard")
				return flashcardsDir, learningNotesDir
			},
			notebookName:      "test-flashcard",
			expectedCardCount: 1,
		},
		{
			name: "no learning history returns error",
			setupFunc: func(t *testing.T) (string, string) {
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
							{Expression: "test", Meaning: "test meaning"},
						},
					},
				}))

				return flashcardsDir, learningNotesDir
			},
			notebookName: "test-flashcard",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flashcardsDir, learningNotesDir := tt.setupFunc(t)
			dictionaryCacheDir := t.TempDir()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := mock_inference.NewMockClient(ctrl)

			cli, err := NewFlashcardQuizCLI(
				tt.notebookName,
				config.NotebooksConfig{
					FlashcardsDirectories:  []string{flashcardsDir},
					LearningNotesDirectory: learningNotesDir,
				},
				dictionaryCacheDir,
				mockClient,
			)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedCardCount, cli.GetCardCount())
		})
	}
}

func TestFormatQuestion(t *testing.T) {
	tests := []struct {
		name     string
		card     quiz.Card
		expected string
	}{
		{
			name: "No examples",
			card: quiz.Card{
				Entry: "test",
			},
			expected: "What does 'test' mean?\n",
		},
		{
			name: "Flashcard with examples (no scene title)",
			card: quiz.Card{
				Entry:      "test",
				SceneTitle: "",
				Examples: []quiz.Example{
					{Text: "This is a test example"},
				},
			},
			expected: "What does 'test' mean?\nExamples:\n  1. This is a test example\n",
		},
		{
			name: "Story notebook single example",
			card: quiz.Card{
				Entry:      "test",
				StoryTitle: "Story 1",
				SceneTitle: "Scene 1",
				Examples: []quiz.Example{
					{Text: "This is a test example", Speaker: "Alice"},
				},
			},
			expected: "What does 'test' mean in the following context?\n  1. This is a test example\n",
		},
		{
			name: "Story notebook multiple examples",
			card: quiz.Card{
				Entry:      "test",
				StoryTitle: "Story 1",
				SceneTitle: "Scene 1",
				Examples: []quiz.Example{
					{Text: "First example", Speaker: "Alice"},
					{Text: "Second example", Speaker: "Bob"},
				},
			},
			expected: "What does 'test' mean in the following context?\n  1. First example\n  2. Second example\n",
		},
		{
			name: "Story with pre-cleaned text (no markers)",
			card: quiz.Card{
				Entry:      "excited",
				StoryTitle: "Story 1",
				SceneTitle: "Scene 1",
				Examples: []quiz.Example{
					{Text: "I am so excited about the trip!", Speaker: "Alice"},
				},
			},
			expected: "What does 'excited' mean in the following context?\n  1. I am so excited about the trip!\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatQuestion(&tc.card)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNotebookQuizCLI_GetCardCount(t *testing.T) {
	tests := []struct {
		name      string
		cardCount int
	}{
		{
			name:      "No cards",
			cardCount: 0,
		},
		{
			name:      "One card",
			cardCount: 1,
		},
		{
			name:      "Multiple cards",
			cardCount: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := make([]quiz.Card, tt.cardCount)
			for i := 0; i < tt.cardCount; i++ {
				cards[i] = quiz.Card{
					Entry:   "test",
					Meaning: "meaning",
				}
			}

			cli := &NotebookQuizCLI{
				cards: cards,
			}

			assert.Equal(t, tt.cardCount, cli.GetCardCount())
		})
	}
}

func TestNotebookQuizCLI_ShuffleCards(t *testing.T) {
	// Create cards with unique entries
	cards := []quiz.Card{
		{Entry: "word1"},
		{Entry: "word2"},
		{Entry: "word3"},
		{Entry: "word4"},
		{Entry: "word5"},
	}

	cli := &NotebookQuizCLI{
		cards: cards,
	}

	// Store original order
	originalOrder := make([]string, len(cards))
	for i, card := range cli.cards {
		originalOrder[i] = card.Entry
	}

	// Shuffle multiple times and check if order changes
	shuffled := false
	for i := 0; i < 10; i++ {
		cli.ShuffleCards()

		// Check if order changed
		orderChanged := false
		for j, card := range cli.cards {
			if card.Entry != originalOrder[j] {
				orderChanged = true
				break
			}
		}

		if orderChanged {
			shuffled = true
			break
		}
	}

	// Verify that cards were shuffled (order changed at least once)
	// Note: There's a very small probability this could fail randomly
	assert.True(t, shuffled, "Cards should be shuffled at least once in 10 attempts")

	// Verify all cards are still present
	assert.Equal(t, 5, len(cli.cards))
	entrySet := make(map[string]bool)
	for _, card := range cli.cards {
		entrySet[card.Entry] = true
	}
	assert.Equal(t, 5, len(entrySet), "All unique cards should still be present")
}

func TestNotebookQuizCLI_RemoveCurrentCard(t *testing.T) {
	tests := []struct {
		name           string
		initialCards   int
		wantCardsAfter int
	}{
		{
			name:           "Remove from multiple cards",
			initialCards:   3,
			wantCardsAfter: 2,
		},
		{
			name:           "Remove last card",
			initialCards:   1,
			wantCardsAfter: 0,
		},
		{
			name:           "Remove from empty list",
			initialCards:   0,
			wantCardsAfter: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := make([]quiz.Card, tt.initialCards)
			for i := 0; i < tt.initialCards; i++ {
				cards[i] = quiz.Card{
					Entry: "test" + string(rune('1'+i)),
				}
			}

			cli := &NotebookQuizCLI{
				cards: cards,
			}

			cli.removeCurrentCard()
			assert.Equal(t, tt.wantCardsAfter, len(cli.cards))
		})
	}
}

func TestNotebookQuizCLI_session(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		cards              []quiz.Card
		mockOpenAIResponse inference.AnswerMeaningsResponse
		mockOpenAIError    error
		wantErr            bool
		wantReturn         error
		wantCardsAfter     int
	}{
		{
			name:           "No more cards - returns errEnd",
			input:          "",
			cards:          []quiz.Card{},
			wantReturn:     errEnd,
			wantCardsAfter: 0,
		},
		{
			name:  "Correct answer - updates learning history and removes card",
			input: "test meaning\n",
			cards: []quiz.Card{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Entry:        "test",
					Meaning:      "test meaning",
					Contexts: []inference.Context{
						{Context: "This is a test", ReferenceDefinition: "test meaning"},
					},
				},
			},
			mockOpenAIResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "test",
						Meaning:    "test meaning",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: true, Context: "", Reason: "exact match with reference definition"},
						},
					},
				},
			},
			wantCardsAfter: 0,
		},
		{
			name:  "Incorrect answer - updates learning history and removes card",
			input: "wrong meaning\n",
			cards: []quiz.Card{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Entry:        "test",
					Meaning:      "test meaning",
				},
			},
			mockOpenAIResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "test",
						Meaning:    "wrong meaning",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: false, Context: "", Reason: "unrelated meanings"},
						},
					},
				},
			},
			wantCardsAfter: 0,
		},
		{
			name:  "Empty answer - marked as incorrect",
			input: "\n",
			cards: []quiz.Card{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Entry:        "test",
					Meaning:      "test meaning",
				},
			},
			mockOpenAIResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "test",
						Meaning:    "",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: true, Context: "", Reason: "empty user input"},
						},
					},
				},
			},
			wantCardsAfter: 0,
		},
		{
			name:  "Multiple cards - processes one at a time",
			input: "test meaning\n",
			cards: []quiz.Card{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Entry:        "test1",
					Meaning:      "test meaning 1",
				},
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Entry:        "test2",
					Meaning:      "test meaning 2",
				},
			},
			mockOpenAIResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "test1",
						Meaning:    "test meaning",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: true, Context: "", Reason: "synonym"},
						},
					},
				},
			},
			wantCardsAfter: 1, // One card should remain
		},
		{
			name:  "Flashcard card (no scene title) - uses flashcards as story title",
			input: "to initiate social interaction\n",
			cards: []quiz.Card{
				{
					NotebookName: "test-flashcard",
					StoryTitle:   "flashcards",
					SceneTitle:   "",
					Entry:        "break the ice",
					Meaning:      "to initiate social interaction",
					Examples: []quiz.Example{
						{Text: "She told a joke to break the ice."},
					},
				},
			},
			mockOpenAIResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "break the ice",
						Meaning:    "to initiate social interaction",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: true, Context: "", Reason: "exact match"},
						},
					},
				},
			},
			wantCardsAfter: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock stdin reader
			stdinReader := bufio.NewReader(strings.NewReader(tt.input))

			// Create mock OpenAI client using gomock
			mockClient := mock_inference.NewMockClient(ctrl)

			// Set expectation only if we have cards (otherwise errEnd is returned immediately)
			if len(tt.cards) > 0 {
				mockClient.EXPECT().
					AnswerMeanings(gomock.Any(), gomock.Any()).
					Return(tt.mockOpenAIResponse, tt.mockOpenAIError).
					AnyTimes()
			}

			learningDir := t.TempDir()

			// Set up CLI with mocks - use a real quiz.Service backed by temp dirs
			svc := quiz.NewService(config.NotebooksConfig{
				LearningNotesDirectory: learningDir,
			}, mockClient, make(map[string]rapidapi.Response))

			cli := &NotebookQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					learningNotesDir:  learningDir,
					learningHistories: map[string][]notebook.LearningHistory{},
					dictionaryMap:     make(map[string]rapidapi.Response),
					openaiClient:      mockClient,
					stdinReader:       stdinReader,
					bold:              color.New(color.Bold),
					italic:            color.New(color.Italic),
				},
				svc:          svc,
				notebookName: "test-notebook",
				cards:        tt.cards,
			}

			// Run the session
			err := cli.Session(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				if tt.wantReturn != nil {
					assert.Equal(t, tt.wantReturn, err)
				} else {
					assert.NoError(t, err)
				}
			}

			// Verify card count after session
			assert.Equal(t, tt.wantCardsAfter, len(cli.cards))
		})
	}
}
