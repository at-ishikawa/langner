package cli

import (
	"bufio"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestExtractWordOccurrences(t *testing.T) {
	tests := []struct {
		name              string
		notebookName      string
		stories           []notebook.StoryNotebook
		expectedCardCount int
		validate          func(t *testing.T, cards []*WordOccurrence)
	}{
		{
			name:              "Empty stories",
			notebookName:      "test-notebook",
			stories:           []notebook.StoryNotebook{},
			expectedCardCount: 0,
		},
		{
			name:         "No contexts - no conversations (word still included)",
			notebookName: "test-notebook",
			stories: []notebook.StoryNotebook{
				{
					Event: "Story",
					Scenes: []notebook.StoryScene{
						{
							Title: "Scene",
							Definitions: []notebook.Note{
								{Expression: "word1", Meaning: "meaning1"},
							},
						},
					},
				},
			},
			expectedCardCount: 1,
			validate: func(t *testing.T, cards []*WordOccurrence) {
				card := cards[0]
				assert.Equal(t, "word1", card.Definition.Expression)
				assert.Equal(t, "meaning1", card.GetMeaning())
				assert.Equal(t, 0, len(card.Contexts), "Word without context should have empty contexts")
			},
		},

		{
			name:         "Extract from conversations (including words without context)",
			notebookName: "test-notebook",
			stories: []notebook.StoryNotebook{
				{
					Event: "Story 1",
					Scenes: []notebook.StoryScene{
						{
							Title: "Scene 1",
							Conversations: []notebook.Conversation{
								{
									Speaker: "Character1",
									Quote:   "I need to test1 this feature",
								},
								{
									Speaker: "Character2",
									Quote:   "Let's do another test1 tomorrow",
								},
							},
							Definitions: []notebook.Note{
								{
									Expression: "test1",
									Meaning:    "meaning1",
								},
								{
									Expression: "test2",
									Meaning:    "meaning2",
									// No conversations contain test2 - still included with empty contexts
								},
							},
						},
					},
				},
				{
					Event: "Story 2",
					Scenes: []notebook.StoryScene{
						{
							Title: "Scene 2",
							Conversations: []notebook.Conversation{
								{
									Speaker: "Character3",
									Quote:   "We need to test3 the system",
								},
							},
							Definitions: []notebook.Note{
								{
									Expression: "test3",
									Meaning:    "meaning3",
									Images:     []string{"image1.jpg", "image2.jpg"},
								},
							},
						},
					},
				},
			},
			expectedCardCount: 3, // test1 (2 contexts), test2 (no context), test3 (1 context)
			validate: func(t *testing.T, cards []*WordOccurrence) {
				card1 := cards[0]
				assert.Equal(t, "test1", card1.Definition.Expression)
				assert.Equal(t, "meaning1", card1.GetMeaning())
				assert.Equal(t, 2, len(card1.Contexts))
				assert.Contains(t, card1.Contexts[0].Context, "test1")
				assert.Contains(t, card1.Contexts[1].Context, "test1")
				assert.Equal(t, "Story 1", card1.Story.Event)
				assert.Equal(t, "Scene 1", card1.Scene.Title)

				card2 := cards[1]
				assert.Equal(t, "test2", card2.Definition.Expression)
				assert.Equal(t, "meaning2", card2.GetMeaning())
				assert.Equal(t, 0, len(card2.Contexts), "Word without context should have empty contexts")

				card3 := cards[2]
				assert.Equal(t, "test3", card3.Definition.Expression)
				assert.Equal(t, "meaning3", card3.GetMeaning())
				assert.Equal(t, 1, len(card3.Contexts))
				assert.Contains(t, card3.Contexts[0].Context, "test3")
				assert.Equal(t, 2, len(card3.GetImages()))
			},
		},

		{
			name:         "With definition field",
			notebookName: "test-notebook",
			stories: []notebook.StoryNotebook{
				{
					Event: "Story 1",
					Scenes: []notebook.StoryScene{
						{
							Title: "Scene 1",
							Conversations: []notebook.Conversation{
								{
									Speaker: "Character1",
									Quote:   "Please take off your shoes",
								},
							},
							Definitions: []notebook.Note{
								{
									Expression: "remove",
									Definition: "take off",
									Meaning:    "to remove something",
								},
							},
						},
					},
				},
			},
			expectedCardCount: 1,
			validate: func(t *testing.T, cards []*WordOccurrence) {
				card := cards[0]
				assert.Equal(t, "remove", card.Definition.Expression)
				assert.Equal(t, "take off", card.Definition.Definition)
				assert.Equal(t, 1, len(card.Contexts))
				assert.Contains(t, card.Contexts[0].Context, "take off")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := extractWordOccurrences(tt.notebookName, tt.stories)
			assert.Equal(t, tt.expectedCardCount, len(cards))
			if tt.validate != nil {
				tt.validate(t, cards)
			}
		})
	}
}

func TestFormatQuestion(t *testing.T) {
	tests := []struct {
		name     string
		card     WordOccurrence
		expected string
	}{
		{
			name: "No contexts",
			card: WordOccurrence{
				Definition: &notebook.Note{Expression: "test"},
				Contexts:   []WordOccurrenceContext{},
			},
			expected: "What does 'test' mean?\n",
		},
		{
			name: "Single context",
			card: WordOccurrence{
				Scene:      &notebook.StoryScene{},
				Definition: &notebook.Note{Expression: "test"},
				Contexts:   []WordOccurrenceContext{{Context: "This is a test example", Usage: "test"}},
			},
			expected: "What does 'test' mean in the following context?\n  1. This is a test example\n",
		},
		{
			name: "Multiple contexts",
			card: WordOccurrence{
				Scene:      &notebook.StoryScene{},
				Definition: &notebook.Note{Expression: "test"},
				Contexts:   []WordOccurrenceContext{{Context: "First example", Usage: "test"}, {Context: "Second example", Usage: "test"}},
			},
			expected: "What does 'test' mean in the following context?\n  1. First example\n  2. Second example\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatQuestion(&tc.card)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNotebookQuizCLI_UpdateLearningHistoryRecord(t *testing.T) {
	tests := []struct {
		name           string
		initialHistory []notebook.LearningHistory
		expression     string
		isCorrect      bool
		isKnownWord    bool
		commandType    string // "practice" or "qa"
		wantLogs       int
		wantStatus     notebook.LearnedStatus
	}{
		// Practice vs QA command status differences
		{
			name:           "Practice command correct answer → usable",
			initialHistory: []notebook.LearningHistory{},
			expression:     "test word",
			isCorrect:      true,
			isKnownWord:    false,
			commandType:    "practice",
			wantLogs:       1,
			wantStatus:     "usable",
		},
		{
			name:           "QA command correct answer → understood",
			initialHistory: []notebook.LearningHistory{},
			expression:     "test word",
			isCorrect:      true,
			isKnownWord:    true,
			commandType:    "qa",
			wantLogs:       1,
			wantStatus:     "understood",
		},
		{
			name:           "Practice command incorrect answer → misunderstood",
			initialHistory: []notebook.LearningHistory{},
			expression:     "test word",
			isCorrect:      false,
			isKnownWord:    false,
			commandType:    "practice",
			wantLogs:       1,
			wantStatus:     "misunderstood",
		},
		{
			name:           "QA command incorrect answer → misunderstood",
			initialHistory: []notebook.LearningHistory{},
			expression:     "test word",
			isCorrect:      false,
			isKnownWord:    true,
			commandType:    "qa",
			wantLogs:       1,
			wantStatus:     "misunderstood",
		},
		// QA command always-record behavior
		{
			name: "QA should record duplicate understood status",
			initialHistory: []notebook.LearningHistory{
				{
					Metadata: notebook.LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Test Story",
					},
					Scenes: []notebook.LearningScene{
						{
							Metadata: notebook.LearningSceneMetadata{
								Title: "Test Scene",
							},
							Expressions: []notebook.LearningHistoryExpression{
								{
									Expression: "test",
									LearnedLogs: []notebook.LearningRecord{
										{
											Status:    "understood",
											LearnedAt: notebook.NewDateFromTime(time.Now().AddDate(0, 0, -1)),
										},
									},
								},
							},
						},
					},
				},
			},
			expression:  "test",
			isCorrect:   true,
			isKnownWord: true,
			commandType: "qa",
			wantLogs:    2,
			wantStatus:  "understood",
		},
		{
			name: "QA should NOT record duplicate misunderstood status",
			initialHistory: []notebook.LearningHistory{
				{
					Metadata: notebook.LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Test Story",
					},
					Scenes: []notebook.LearningScene{
						{
							Metadata: notebook.LearningSceneMetadata{
								Title: "Test Scene",
							},
							Expressions: []notebook.LearningHistoryExpression{
								{
									Expression: "test",
									LearnedLogs: []notebook.LearningRecord{
										{
											Status:    notebook.LearnedStatusMisunderstood,
											LearnedAt: notebook.NewDateFromTime(time.Now().AddDate(0, 0, -1)),
										},
									},
								},
							},
						},
					},
				},
			},
			expression:  "test",
			isCorrect:   false,
			isKnownWord: true,
			commandType: "qa",
			wantLogs:    1,
			wantStatus:  notebook.LearnedStatusMisunderstood,
		},
		{
			name: "QA should record incorrect answer as misunderstood",
			initialHistory: []notebook.LearningHistory{
				{
					Metadata: notebook.LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Test Story",
					},
					Scenes: []notebook.LearningScene{
						{
							Metadata: notebook.LearningSceneMetadata{
								Title: "Test Scene",
							},
							Expressions: []notebook.LearningHistoryExpression{},
						},
					},
				},
			},
			expression:  "test",
			isCorrect:   false,
			isKnownWord: true,
			commandType: "qa",
			wantLogs:    1,
			wantStatus:  notebook.LearnedStatusMisunderstood,
		},
		{
			name: "QA should record when status changes",
			initialHistory: []notebook.LearningHistory{
				{
					Metadata: notebook.LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Test Story",
					},
					Scenes: []notebook.LearningScene{
						{
							Metadata: notebook.LearningSceneMetadata{
								Title: "Test Scene",
							},
							Expressions: []notebook.LearningHistoryExpression{
								{
									Expression: "test",
									LearnedLogs: []notebook.LearningRecord{
										{
											Status:    notebook.LearnedStatusMisunderstood,
											LearnedAt: notebook.NewDateFromTime(time.Now().AddDate(0, 0, -1)),
										},
									},
								},
							},
						},
					},
				},
			},
			expression:  "test",
			isCorrect:   true,
			isKnownWord: true,
			commandType: "qa",
			wantLogs:    2,
			wantStatus:  "understood",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &InteractiveQuizCLI{
				learningNotesDir: t.TempDir(),
			}

			var gotHistory []notebook.LearningHistory
			var err error
			if tt.commandType == "practice" {
				gotHistory, err = cli.updateLearningHistory(
					"test-notebook",
					tt.initialHistory,
					"test-notebook",
					"Test Story",
					"Test Scene",
					tt.expression,
					tt.isCorrect,
					tt.isKnownWord,
					false, // alwaysRecord=false for practice
				)
			} else {
				gotHistory, err = cli.updateLearningHistory(
					"test-notebook",
					tt.initialHistory,
					"test-notebook",
					"Test Story",
					"Test Scene",
					tt.expression,
					tt.isCorrect,
					tt.isKnownWord,
					true, // alwaysRecord=true for qa
				)
			}
			require.NoError(t, err)

			// Find the expression
			var gotExpression *notebook.LearningHistoryExpression
			for _, history := range gotHistory {
				if history.Metadata.Title == "Test Story" {
					for _, scene := range history.Scenes {
						if scene.Metadata.Title == "Test Scene" {
							for _, expr := range scene.Expressions {
								if expr.Expression == tt.expression {
									gotExpression = &expr
									break
								}
							}
						}
					}
				}
			}

			require.NotNil(t, gotExpression, "Expression should be found in history")
			assert.Equal(t, tt.wantStatus, gotExpression.GetLatestStatus())
			assert.Equal(t, tt.wantLogs, len(gotExpression.LearnedLogs))
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
			cards := make([]*WordOccurrence, tt.cardCount)
			for i := 0; i < tt.cardCount; i++ {
				cards[i] = &WordOccurrence{
					Definition: &notebook.Note{
						Expression: "test",
						Meaning:    "meaning",
					},
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
	// Create cards with unique expressions
	cards := []*WordOccurrence{
		{Definition: &notebook.Note{Expression: "word1"}},
		{Definition: &notebook.Note{Expression: "word2"}},
		{Definition: &notebook.Note{Expression: "word3"}},
		{Definition: &notebook.Note{Expression: "word4"}},
		{Definition: &notebook.Note{Expression: "word5"}},
	}

	cli := &NotebookQuizCLI{
		cards: cards,
	}

	// Store original order
	originalOrder := make([]string, len(cards))
	for i, card := range cli.cards {
		originalOrder[i] = card.Definition.Expression
	}

	// Shuffle multiple times and check if order changes
	shuffled := false
	for i := 0; i < 10; i++ {
		cli.ShuffleCards()

		// Check if order changed
		orderChanged := false
		for j, card := range cli.cards {
			if card.Definition.Expression != originalOrder[j] {
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
	expressionSet := make(map[string]bool)
	for _, card := range cli.cards {
		expressionSet[card.Definition.Expression] = true
	}
	assert.Equal(t, 5, len(expressionSet), "All unique cards should still be present")
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
			cards := make([]*WordOccurrence, tt.initialCards)
			for i := 0; i < tt.initialCards; i++ {
				cards[i] = &WordOccurrence{
					Definition: &notebook.Note{
						Expression: "test" + string(rune('1'+i)),
					},
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
		cards              []*WordOccurrence
		learningHistories  map[string][]notebook.LearningHistory
		mockOpenAIResponse inference.AnswerMeaningsResponse
		mockOpenAIError    error
		wantErr            bool
		wantReturn         error
		wantCardsAfter     int
		validate           func(t *testing.T, cli *NotebookQuizCLI)
	}{
		{
			name:           "No more cards - returns errEnd",
			input:          "",
			cards:          []*WordOccurrence{},
			wantReturn:     errEnd,
			wantCardsAfter: 0,
		},
		{
			name:  "Correct answer - updates learning history and removes card",
			input: "test meaning\n",
			cards: []*WordOccurrence{
				{
					NotebookName: "test-notebook",
					Story: &notebook.StoryNotebook{
						Event: "Story 1",
					},
					Scene: &notebook.StoryScene{
						Title: "Scene 1",
					},
					Definition: &notebook.Note{
						Expression: "test",
						Meaning:    "test meaning",
					},
					Contexts: []WordOccurrenceContext{{Context: "This is a test", Usage: "test"}},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
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
			cards: []*WordOccurrence{
				{
					NotebookName: "test-notebook",
					Story: &notebook.StoryNotebook{
						Event: "Story 1",
					},
					Scene: &notebook.StoryScene{
						Title: "Scene 1",
					},
					Definition: &notebook.Note{
						Expression: "test",
						Meaning:    "test meaning",
					},
					Contexts: []WordOccurrenceContext{{Context: "This is a test", Usage: "test"}},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			mockOpenAIResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "test",
						Meaning:    "wrong meaning",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: false, Context: "", Reason: "A3 - unrelated: user meaning is from wrong semantic field"},
						},
					},
				},
			},
			wantCardsAfter: 0,
		},
		{
			name:  "Empty answer - marked as incorrect",
			input: "\n",
			cards: []*WordOccurrence{
				{
					NotebookName: "test-notebook",
					Story: &notebook.StoryNotebook{
						Event: "Story 1",
					},
					Scene: &notebook.StoryScene{
						Title: "Scene 1",
					},
					Definition: &notebook.Note{
						Expression: "test",
						Meaning:    "test meaning",
					},
					Contexts: []WordOccurrenceContext{{Context: "This is a test", Usage: "test"}},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			mockOpenAIResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "test",
						Meaning:    "",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: true, Context: "", Reason: "empty user input"}, // OpenAI says correct, but empty answer overrides it
						},
					},
				},
			},
			wantCardsAfter: 0,
		},
		{
			name:  "Multiple cards - processes one at a time",
			input: "test meaning\n",
			cards: []*WordOccurrence{
				{
					NotebookName: "test-notebook",
					Story: &notebook.StoryNotebook{
						Event: "Story 1",
					},
					Scene: &notebook.StoryScene{
						Title: "Scene 1",
					},
					Definition: &notebook.Note{
						Expression: "test1",
						Meaning:    "test meaning 1",
					},
					Contexts: []WordOccurrenceContext{{Context: "This is test1", Usage: "test1"}},
				},
				{
					NotebookName: "test-notebook",
					Story: &notebook.StoryNotebook{
						Event: "Story 1",
					},
					Scene: &notebook.StoryScene{
						Title: "Scene 1",
					},
					Definition: &notebook.Note{
						Expression: "test2",
						Meaning:    "test meaning 2",
					},
					Contexts: []WordOccurrenceContext{{Context: "This is test2", Usage: "test2"}},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			mockOpenAIResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "test1",
						Meaning:    "test meaning",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: true, Context: "", Reason: "synonym: meanings are equivalent"},
						},
					},
				},
			},
			wantCardsAfter: 1, // One card should remain
		},
		{
			name:  "Plural expression with singular definition - uses definition for OpenAI",
			input: "a person who has the responsibility of watching for something\n",
			cards: []*WordOccurrence{
				{
					NotebookName: "test-notebook",
					Story: &notebook.StoryNotebook{
						Event: "Story 1",
					},
					Scene: &notebook.StoryScene{
						Title: "Scene 1",
					},
					Definition: &notebook.Note{
						Expression: "lookouts",
						Definition: "lookout",
						Meaning:    "a person who has the responsibility of watching for something, especially danger, etc.",
					},
					Contexts: []WordOccurrenceContext{{Context: "They're used as lookouts", Usage: "lookout"}},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			mockOpenAIResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "lookout",
						Meaning:    "a person who has the responsibility of watching for something, especially danger, etc.",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: true, Context: "", Reason: "partial match covers main sense"},
						},
					},
				},
			},
			wantCardsAfter: 0,
		},
		{
			name:  "Card from different notebook - updates correct notebook's learning history",
			input: "test meaning\n",
			cards: []*WordOccurrence{
				{
					NotebookName: "notebook-a",
					Story: &notebook.StoryNotebook{
						Event: "Story A",
					},
					Scene: &notebook.StoryScene{
						Title: "Scene A",
					},
					Definition: &notebook.Note{
						Expression: "test",
						Meaning:    "test meaning",
					},
					Contexts: []WordOccurrenceContext{{Context: "This is a test", Usage: "test"}},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{
				"notebook-a": {},
				"notebook-b": {},
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
			validate: func(t *testing.T, cli *NotebookQuizCLI) {
				// Verify notebook-a has learning history updated
				historyA := cli.learningHistories["notebook-a"]
				require.NotEmpty(t, historyA, "notebook-a should have learning history")

				// Find the learning history for the story and expression
				found := false
				for _, history := range historyA {
					if history.Metadata.Title == "Story A" {
						for _, scene := range history.Scenes {
							if scene.Metadata.Title == "Scene A" {
								for _, expr := range scene.Expressions {
									if expr.Expression == "test" {
										found = true
										assert.NotEmpty(t, expr.LearnedLogs, "Expression should have learning logs")
										assert.Equal(t, "understood", string(expr.GetLatestStatus()), "Latest status should be understood for correct answer")
										break
									}
								}
							}
						}
					}
				}
				assert.True(t, found, "Should find the test expression in notebook-a learning history")

				// Verify notebook-b has no learning history (should remain empty)
				historyB := cli.learningHistories["notebook-b"]
				assert.Empty(t, historyB, "notebook-b should have no learning history updates")
			},
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

			// Set up CLI with mocks
			cli := &NotebookQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					learningNotesDir:  t.TempDir(),
					learningHistories: tt.learningHistories,
					dictionaryMap:     make(map[string]rapidapi.Response),
					openaiClient:      mockClient,
					stdinReader:       stdinReader,
					bold:              color.New(color.Bold),
					italic:            color.New(color.Italic),
				},
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

			// Run custom validation if provided
			if tt.validate != nil {
				tt.validate(t, cli)
			}
		})
	}
}
