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
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateInput(tc.word, tc.meaning)

			if tc.expectError {
				assert.Error(t, err)
				assert.Equal(t, tc.errorType, err)
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

func TestFreeformQuizCLI_FindAllWordContexts(t *testing.T) {
	tests := []struct {
		name                        string
		allStories                  map[string][]notebook.StoryNotebook
		searchWord                  string
		wantContextsCount           int
		wantNotebookCount           map[string]int
		wantTotalConversationQuotes int // total number of conversation quotes found across all occurrences
	}{
		{
			name: "find word across multiple notebooks and scenes",
			allStories: map[string][]notebook.StoryNotebook{
				"vocab1": {
					{
						Event: "Unit 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Lesson 1",
								Conversations: []notebook.Conversation{
									{
										Speaker: "Speaker A",
										Quote:   "The athlete made a quick lunge forward.",
									},
								},
								Definitions: []notebook.Note{
									{
										Expression: "lunge",
										Meaning:    "sudden forward movement",
										Examples: []string{
											"He lunged at the opportunity",
										},
									},
								},
							},
							{
								Title: "Lesson 2",
								Conversations: []notebook.Conversation{
									{
										Speaker: "Speaker B",
										Quote:   "She made a sudden lunge to catch the ball.",
									},
								},
								Definitions: []notebook.Note{
									{
										Expression: "lunge",
										Meaning:    "to thrust forward suddenly",
									},
								},
							},
						},
					},
				},
				"vocab2": {
					{
						Event: "Unit 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Exercise 1",
								Conversations: []notebook.Conversation{
									{
										Speaker: "Speaker C",
										Quote:   "The fencer demonstrated a perfect lunge.",
									},
								},
								Definitions: []notebook.Note{
									{
										Expression: "lunge",
										Meaning:    "a sword thrust",
									},
								},
							},
						},
					},
					{
						Event: "Unit 2",
						Scenes: []notebook.StoryScene{
							{
								Title: "Exercise 2",
								Definitions: []notebook.Note{
									{
										Expression: "forward lunge",
										Definition: "lunge",
										Meaning:    "an aggressive forward attack",
									},
								},
							},
						},
					},
				},
			},
			searchWord:        "lunge",
			wantContextsCount: 4,
			wantNotebookCount: map[string]int{
				"vocab1": 2,
				"vocab2": 2,
			},
		},
		{
			name: "word not found returns empty",
			allStories: map[string][]notebook.StoryNotebook{
				"vocab1": {
					{
						Event: "Unit 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Lesson 1",
								Definitions: []notebook.Note{
									{
										Expression: "lunge",
										Meaning:    "sudden forward movement",
									},
								},
							},
						},
					},
				},
			},
			searchWord:        "nonexistent",
			wantContextsCount: 0,
		},
		{
			name: "case insensitive search",
			allStories: map[string][]notebook.StoryNotebook{
				"vocab1": {
					{
						Event: "Unit 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Lesson 1",
								Conversations: []notebook.Conversation{
									{
										Speaker: "Speaker A",
										Quote:   "The athlete made a quick lunge forward.",
									},
								},
								Definitions: []notebook.Note{
									{
										Expression: "lunge",
										Meaning:    "sudden forward movement",
										Examples: []string{
											"He lunged at the opportunity",
										},
									},
								},
							},
							{
								Title: "Lesson 2",
								Conversations: []notebook.Conversation{
									{
										Speaker: "Speaker B",
										Quote:   "She made a sudden lunge to catch the ball.",
									},
								},
								Definitions: []notebook.Note{
									{
										Expression: "lunge",
										Meaning:    "to thrust forward suddenly",
									},
								},
							},
						},
					},
				},
				"vocab2": {
					{
						Event: "Unit 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Exercise 1",
								Conversations: []notebook.Conversation{
									{
										Speaker: "Speaker C",
										Quote:   "The fencer demonstrated a perfect lunge.",
									},
								},
								Definitions: []notebook.Note{
									{
										Expression: "lunge",
										Meaning:    "a sword thrust",
									},
								},
							},
						},
					},
					{
						Event: "Unit 2",
						Scenes: []notebook.StoryScene{
							{
								Title: "Exercise 2",
								Definitions: []notebook.Note{
									{
										Expression: "forward lunge",
										Definition: "lunge",
										Meaning:    "an aggressive forward attack",
									},
								},
							},
						},
					},
				},
			},
			searchWord:        "LUNGE",
			wantContextsCount: 4,
		},
		{
			name: "find words in definition field",
			allStories: map[string][]notebook.StoryNotebook{
				"vocab1": {
					{
						Event: "Unit 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Phrasal Verbs",
								Definitions: []notebook.Note{
									{
										Expression: "sit out",
										Definition: "sit",
										Meaning:    "to not participate",
									},
									{
										Expression: "sit down",
										Definition: "sit",
										Meaning:    "to take a seat",
									},
								},
							},
						},
					},
				},
				"vocab2": {
					{
						Event: "Unit 2",
						Scenes: []notebook.StoryScene{
							{
								Title: "Common Verbs",
								Definitions: []notebook.Note{
									{
										Expression: "sit",
										Meaning:    "to be in a seated position",
									},
								},
							},
						},
					},
				},
			},
			searchWord:        "sit",
			wantContextsCount: 3,
		},
		{
			name: "search by expression finds contexts containing definition",
			allStories: map[string][]notebook.StoryNotebook{
				"vocab1": {
					{
						Event: "Unit 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Phrasal Verbs",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "Come here please"}, // contains "come" only, not "come along"
								},
								Definitions: []notebook.Note{
									{
										Expression: "come along",
										Definition: "come",
										Meaning:    "to arrive",
									},
								},
							},
						},
					},
				},
			},
			searchWord:                  "come along", // search by Expression
			wantContextsCount:           1,            // should find 1 WordOccurrence
			wantTotalConversationQuotes: 1,            // should find conversation containing "come"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &FreeformQuizCLI{
				allStories:    tt.allStories,
				allFlashcards: make(map[string][]notebook.FlashcardNotebook),
			}

			contexts := cli.findAllWordContexts(tt.searchWord)

			assert.Len(t, contexts, tt.wantContextsCount)

			if tt.wantNotebookCount != nil {
				notebookCount := make(map[string]int)
				for _, ctx := range contexts {
					notebookCount[ctx.NotebookName]++
				}
				for notebook, expectedCount := range tt.wantNotebookCount {
					assert.Equal(t, expectedCount, notebookCount[notebook])
				}
			}

			if tt.wantTotalConversationQuotes > 0 {
				totalQuotes := 0
				for _, ctx := range contexts {
					totalQuotes += len(ctx.Contexts)
				}
				assert.Equal(t, tt.wantTotalConversationQuotes, totalQuotes, "total conversation quotes should match")
			}
		})
	}
}

func TestFreeformQuizCLI_UpdateLearningHistoryRecord(t *testing.T) {
	tests := []struct {
		name            string
		initialHistory  []notebook.LearningHistory
		notebookID      string
		storyTitle      string
		sceneTitle      string
		expression      string
		isCorrect       bool
		isKnownWord     bool
		wantCount       int
		wantStatus      *notebook.LearnedStatus
		wantRecordCount *int
	}{
		{
			name:           "Add new expression to empty history",
			initialHistory: []notebook.LearningHistory{},
			notebookID:     "test-notebook",
			storyTitle:     "Story 1",
			sceneTitle:     "Scene 1",
			expression:     "test-word",
			isCorrect:      true,
			isKnownWord:    true,
			wantCount:      1,
		},
		{
			name: "Update existing expression",
			initialHistory: []notebook.LearningHistory{
				{
					Metadata: notebook.LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Story 1",
					},
					Scenes: []notebook.LearningScene{
						{
							Metadata: notebook.LearningSceneMetadata{
								Title: "Scene 1",
							},
							Expressions: []notebook.LearningHistoryExpression{
								{
									Expression:  "test-word",
									LearnedLogs: []notebook.LearningRecord{},
								},
							},
						},
					},
				},
			},
			notebookID:  "test-notebook",
			storyTitle:  "Story 1",
			sceneTitle:  "Scene 1",
			expression:  "test-word",
			isCorrect:   true,
			isKnownWord: false,
			wantCount:   1,
		},
		{
			name:           "Correct answer sets status to usable",
			initialHistory: []notebook.LearningHistory{},
			notebookID:     "test-notebook",
			storyTitle:     "Test Story",
			sceneTitle:     "Test Scene",
			expression:     "test",
			isCorrect:      true,
			isKnownWord:    false,
			wantCount:      1,
			wantStatus:     func() *notebook.LearnedStatus { s := notebook.LearnedStatus("usable"); return &s }(),
		},
		{
			name:           "Incorrect answer sets status to misunderstood",
			initialHistory: []notebook.LearningHistory{},
			notebookID:     "test-notebook",
			storyTitle:     "Test Story",
			sceneTitle:     "Test Scene",
			expression:     "test",
			isCorrect:      false,
			isKnownWord:    false,
			wantCount:      1,
			wantStatus:     func() *notebook.LearnedStatus { s := notebook.LearnedStatusMisunderstood; return &s }(),
		},
		{
			name: "Correct answer on existing word with understood status changes to usable",
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
											LearnedAt: notebook.NewDate(time.Now().AddDate(0, 0, -1)),
										},
									},
								},
							},
						},
					},
				},
			},
			notebookID:  "test-notebook",
			storyTitle:  "Test Story",
			sceneTitle:  "Test Scene",
			expression:  "test",
			isCorrect:   true,
			isKnownWord: false,
			wantCount:   1,
			wantStatus:  func() *notebook.LearnedStatus { s := notebook.LearnedStatus("usable"); return &s }(),
		},
		{
			name:           "Base form 'come along' should be recorded",
			initialHistory: []notebook.LearningHistory{},
			notebookID:     "test-notebook",
			storyTitle:     "Test Story",
			sceneTitle:     "Test Scene",
			expression:     "come along",
			isCorrect:      true,
			isKnownWord:    false,
			wantCount:      1,
		},
		{
			name:           "Full expression should be recorded",
			initialHistory: []notebook.LearningHistory{},
			notebookID:     "test-notebook",
			storyTitle:     "Test Story",
			sceneTitle:     "Test Scene",
			expression:     "comes along to fix",
			isCorrect:      true,
			isKnownWord:    false,
			wantCount:      1,
		},
		{
			name: "Duplicate status - should add new record",
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
											Status:    "usable",
											LearnedAt: notebook.NewDate(time.Now().AddDate(0, 0, -1)),
										},
									},
								},
							},
						},
					},
				},
			},
			notebookID:      "test-notebook",
			storyTitle:      "Test Story",
			sceneTitle:      "Test Scene",
			expression:      "test",
			isCorrect:       true,
			isKnownWord:     false,
			wantCount:       1,
			wantRecordCount: func() *int { c := 2; return &c }(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cli := &InteractiveQuizCLI{
				learningNotesDir: t.TempDir(),
			}
			result, err := cli.updateLearningHistoryWithQuality(
				tc.notebookID,
				tc.initialHistory,
				tc.notebookID,
				tc.storyTitle,
				tc.sceneTitle,
				tc.expression,
				tc.isCorrect,
				tc.isKnownWord,
				4,
				1000,
				notebook.QuizTypeFreeform,
			)
			require.NoError(t, err)

			assert.Len(t, result, tc.wantCount)

			// Find the expression
			var foundExpression *notebook.LearningHistoryExpression
			for _, story := range result {
				if story.Metadata.Title == tc.storyTitle {
					for _, scene := range story.Scenes {
						if scene.Metadata.Title == tc.sceneTitle {
							for _, exp := range scene.Expressions {
								if exp.Expression == tc.expression {
									foundExpression = &exp
									break
								}
							}
						}
					}
				}
			}

			require.NotNil(t, foundExpression, "Expression should exist in history")
			assert.NotEmpty(t, foundExpression.LearnedLogs)

			// Check want status if specified
			if tc.wantStatus != nil {
				assert.Equal(t, *tc.wantStatus, foundExpression.GetLatestStatus())
			}

			// Check want record count if specified
			if tc.wantRecordCount != nil {
				assert.Equal(t, *tc.wantRecordCount, len(foundExpression.LearnedLogs), "Should not add duplicate status record")
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
			wantStatus:    "usable",
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

			testStories := map[string][]notebook.StoryNotebook{
				"test-notebook": {
					{
						Event: "Test Story",
						Scenes: []notebook.StoryScene{
							{
								Title: "Test Scene",
								Conversations: []notebook.Conversation{
									{
										Speaker: "Test",
										Quote:   "This is a test word in context",
									},
								},
								Definitions: []notebook.Note{
									{
										Expression: "test",
										Meaning:    "a procedure to check something",
										Examples:   []string{"This is a test"},
									},
								},
							},
						},
					},
				},
			}

			testLearningHistories := map[string][]notebook.LearningHistory{
				"test-notebook": {
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
			}

			learningNotePath := filepath.Join(tmpDir, "test-notebook.yml")

			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					learningNotesDir:  tmpDir,
					learningHistories: testLearningHistories,
					stdoutWriter:      os.Stdout,
				},
				allStories:    testStories,
				allFlashcards: make(map[string][]notebook.FlashcardNotebook),
			}

			answer := AnswerResponse{
				Correct:    tt.answerCorrect,
				Expression: "test",
				Meaning:    "a procedure to check something",
			}

			// Simulate Run() behavior: if answer.Correct { updateLearningHistory() }
			if answer.Correct {
				wordContexts := cli.findAllWordContexts("test")
				require.Len(t, wordContexts, 1, "Should find the test word")

				needsLearning := cli.findOccurrencesNeedingLearning(wordContexts, "test")
				require.Len(t, needsLearning, 1, "Should have one occurrence needing learning")
				err := cli.updateLearningHistory(needsLearning[0], "test", answer, 4, 1000)
				require.NoError(t, err)
			}

			// Verify file saved based on expectation
			_, err := os.Stat(learningNotePath)
			if !tt.wantFileSaved {
				assert.True(t, os.IsNotExist(err), "Learning note file should not be created for incorrect answer")
				return
			}

			require.NoError(t, err, "Learning note file should be created after correct answer")

			// Read and verify the saved history
			var savedHistory []notebook.LearningHistory
			file, err := os.Open(learningNotePath)
			require.NoError(t, err)
			defer file.Close()

			err = yaml.NewDecoder(file).Decode(&savedHistory)
			require.NoError(t, err)

			// Verify the word was recorded with correct status
			gotStatus := findExpressionStatus(savedHistory, "Test Story", "Test Scene", "test")
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

func TestFreeformQuizCLI_UpdateLearningHistory(t *testing.T) {
	tests := []struct {
		name                      string
		word                      string
		numOccurrences            int
		numToUpdate               int
		wantRecordedScenes        []string
		wantNotRecordedScenes     []string
		wantRemainingNeedLearning int
	}{
		{
			name:                      "Only first correct occurrence is updated",
			word:                      "test",
			numOccurrences:            3,
			numToUpdate:               1,
			wantRecordedScenes:        []string{"Scene 1"},
			wantNotRecordedScenes:     []string{"Scene 2", "Scene 3"},
			wantRemainingNeedLearning: 2,
		},
	}

	t.Run("Update expression with empty learned_logs (Definition form)", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a story with Expression and Definition fields
		testStories := map[string][]notebook.StoryNotebook{
			"test-notebook": {
				{
					Event: "Lesson 3",
					Scenes: []notebook.StoryScene{
						{
							Title: "Scene 1",
							Conversations: []notebook.Conversation{
								{Speaker: "Lisa", Quote: "Ted would like to run some ideas by us for our new ad campaign."},
							},
							Definitions: []notebook.Note{
								{
									Expression: "run some ideas by us",
									Definition: "run some ideas by someone", // This is the Definition form (base form)
									Meaning:    "to discuss some new ideas",
								},
							},
						},
					},
				},
			},
		}

		// Create learning history with the Definition form having empty learned_logs
		testLearningHistories := map[string][]notebook.LearningHistory{
			"test-notebook": {
				{
					Metadata: notebook.LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Lesson 3",
					},
					Scenes: []notebook.LearningScene{
						{
							Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []notebook.LearningHistoryExpression{
								{
									Expression:  "run some ideas by someone", // Definition form exists with empty logs
									LearnedLogs: []notebook.LearningRecord{},
								},
							},
						},
					},
				},
			},
		}

		cli := &FreeformQuizCLI{
			InteractiveQuizCLI: &InteractiveQuizCLI{
				learningNotesDir:  tmpDir,
				learningHistories: testLearningHistories,
				stdoutWriter:      os.Stdout,
			},
			allStories: testStories,
		}

		// User types the Expression form (NOT the Definition form)
		word := "run some ideas by us"

		// Find occurrences
		wordContexts := cli.findAllWordContexts(word)
		require.Len(t, wordContexts, 1, "Should find 1 occurrence")

		// Check if it needs learning
		needsLearning := cli.findOccurrencesNeedingLearning(wordContexts, word)
		require.Len(t, needsLearning, 1, "Should need learning since learned_logs is empty")

		// Simulate correct answer
		answer := AnswerResponse{
			Correct:    true,
			Expression: word,
			Meaning:    "to discuss some new ideas",
		}

		// Update learning history
		err := cli.updateLearningHistory(needsLearning[0], word, answer, 4, 1000)
		require.NoError(t, err)

		// Read the saved history
		learningNotePath := filepath.Join(tmpDir, "test-notebook.yml")
		var savedHistory []notebook.LearningHistory
		file, err := os.Open(learningNotePath)
		require.NoError(t, err)
		defer file.Close()

		err = yaml.NewDecoder(file).Decode(&savedHistory)
		require.NoError(t, err)

		// Verify that the DEFINITION form was updated (not the Expression form)
		gotStatusDefinition := findExpressionStatus(savedHistory, "Lesson 3", "Scene 1", "run some ideas by someone")
		require.NotNil(t, gotStatusDefinition, "Definition form 'run some ideas by someone' should have status")
		assert.Equal(t, notebook.LearnedStatus("usable"), *gotStatusDefinition, "Definition form status should be 'usable'")

		// Verify that the EXPRESSION form was NOT recorded
		gotStatusExpression := findExpressionStatus(savedHistory, "Lesson 3", "Scene 1", "run some ideas by us")
		assert.Nil(t, gotStatusExpression, "Expression form 'run some ideas by us' should NOT be recorded")
	})

	t.Run("Duplicate expressions with empty and usable logs - word already mastered", func(t *testing.T) {
		// Create a story with Expression and Definition fields
		testStories := map[string][]notebook.StoryNotebook{
			"test-notebook": {
				{
					Event: "Lesson 3",
					Scenes: []notebook.StoryScene{
						{
							Title: "Scene 1",
							Conversations: []notebook.Conversation{
								{Speaker: "Lisa", Quote: "Ted would like to run some ideas by us for our new ad campaign."},
							},
							Definitions: []notebook.Note{
								{
									Expression: "run some ideas by us",
									Definition: "run some ideas by someone",
									Meaning:    "to discuss some new ideas",
								},
							},
						},
					},
				},
			},
		}

		// Create learning history with TWO expressions in the SAME scene:
		// 1. "run some ideas by someone" (Definition form) with empty learned_logs
		// 2. "run some ideas by us" (Expression form) with usable status
		// GetLogs should skip the empty-log entry and find the usable one
		testLearningHistories := map[string][]notebook.LearningHistory{
			"test-notebook": {
				{
					Metadata: notebook.LearningHistoryMetadata{
						NotebookID: "test-notebook",
						Title:      "Lesson 3",
					},
					Scenes: []notebook.LearningScene{
						{
							Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []notebook.LearningHistoryExpression{
								{
									Expression:  "run some ideas by someone",
									LearnedLogs: []notebook.LearningRecord{},
								},
								{
									Expression: "run some ideas by us",
									LearnedLogs: []notebook.LearningRecord{
										{
											Status:    "usable",
											LearnedAt: notebook.NewDate(time.Now()),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		cli := &FreeformQuizCLI{
			InteractiveQuizCLI: &InteractiveQuizCLI{
				learningHistories: testLearningHistories,
				stdoutWriter:      os.Stdout,
			},
			allStories: testStories,
		}

		// User types the Expression form
		word := "run some ideas by us"

		// Find occurrences
		wordContexts := cli.findAllWordContexts(word)
		require.Len(t, wordContexts, 1, "Should find 1 occurrence")

		// GetLogs now skips the empty-log entry and finds the usable one,
		// so the word should NOT need learning
		needsLearning := cli.findOccurrencesNeedingLearning(wordContexts, word)
		assert.Len(t, needsLearning, 0, "Should not need learning since sibling expression has usable status")
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create test data with multiple occurrences of the same word in different scenes
			scenes := make([]notebook.StoryScene, tt.numOccurrences)
			learningScenes := make([]notebook.LearningScene, tt.numOccurrences)
			for i := 0; i < tt.numOccurrences; i++ {
				sceneTitle := fmt.Sprintf("Scene %d", i+1)
				scenes[i] = notebook.StoryScene{
					Title: sceneTitle,
					Conversations: []notebook.Conversation{
						{
							Speaker: "Test",
							Quote:   fmt.Sprintf("This is a %s in scene %d", tt.word, i+1),
						},
					},
					Definitions: []notebook.Note{
						{
							Expression: tt.word,
							Meaning:    "a procedure to check something",
						},
					},
				}
				learningScenes[i] = notebook.LearningScene{
					Metadata: notebook.LearningSceneMetadata{
						Title: sceneTitle,
					},
					Expressions: []notebook.LearningHistoryExpression{},
				}
			}

			testStories := map[string][]notebook.StoryNotebook{
				"test-notebook": {
					{
						Event:  "Test Story",
						Scenes: scenes,
					},
				},
			}

			testLearningHistories := map[string][]notebook.LearningHistory{
				"test-notebook": {
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "test-notebook",
							Title:      "Test Story",
						},
						Scenes: learningScenes,
					},
				},
			}

			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					learningNotesDir:  tmpDir,
					learningHistories: testLearningHistories,
					stdoutWriter:      os.Stdout,
				},
				allStories:    testStories,
				allFlashcards: make(map[string][]notebook.FlashcardNotebook),
			}

			// Find all occurrences
			wordContexts := cli.findAllWordContexts(tt.word)
			require.Len(t, wordContexts, tt.numOccurrences, "Should find %d occurrences of '%s'", tt.numOccurrences, tt.word)

			// All need learning since there's no history
			needsLearning := cli.findOccurrencesNeedingLearning(wordContexts, tt.word)
			require.Len(t, needsLearning, tt.numOccurrences, "All %d occurrences should need learning", tt.numOccurrences)

			// Simulate correct answer - update only the specified number of occurrences
			answer := AnswerResponse{
				Correct:    true,
				Expression: tt.word,
				Meaning:    "a procedure to check something",
			}

			// Update each occurrence individually
			for i := 0; i < tt.numToUpdate; i++ {
				err := cli.updateLearningHistory(needsLearning[i], tt.word, answer, 4, 1000)
				require.NoError(t, err)
			}

			// Read the saved history
			learningNotePath := filepath.Join(tmpDir, "test-notebook.yml")
			var savedHistory []notebook.LearningHistory
			file, err := os.Open(learningNotePath)
			require.NoError(t, err)
			defer file.Close()

			err = yaml.NewDecoder(file).Decode(&savedHistory)
			require.NoError(t, err)

			// Verify expected scenes have the word recorded
			for _, sceneTitle := range tt.wantRecordedScenes {
				gotStatus := findExpressionStatus(savedHistory, "Test Story", sceneTitle, tt.word)
				require.NotNil(t, gotStatus, "%s should have '%s' recorded", sceneTitle, tt.word)
				assert.Equal(t, notebook.LearnedStatus("usable"), *gotStatus)
			}

			// Verify expected scenes do NOT have the word recorded
			for _, sceneTitle := range tt.wantNotRecordedScenes {
				gotStatus := findExpressionStatus(savedHistory, "Test Story", sceneTitle, tt.word)
				assert.Nil(t, gotStatus, "%s should NOT have '%s' recorded", sceneTitle, tt.word)
			}

			// Verify the expected number of occurrences still need learning
			needsLearning = cli.findOccurrencesNeedingLearning(wordContexts, tt.word)
			require.Len(t, needsLearning, tt.wantRemainingNeedLearning, "Expected %d occurrences to still need learning", tt.wantRemainingNeedLearning)

			// Verify the remaining occurrences are the expected ones
			for i, sceneTitle := range tt.wantNotRecordedScenes {
				assert.Equal(t, sceneTitle, needsLearning[i].Scene.Title)
			}
		})
	}
}

func TestFreeformQuizCLI_displayResult(t *testing.T) {
	tests := []struct {
		name                string
		answer              AnswerResponse
		occurrence          *WordOccurrence
		wantMeaningInOutput string
		wantNotInOutput     string
		wantContextInOutput string
		wantReasonInOutput  string
	}{
		{
			name: "Correct answer shows user's meaning and reason",
			answer: AnswerResponse{
				Correct:    true,
				Expression: "trinket",
				Meaning:    "a small ornament",
				Reason:     "partial match: user captured main sense",
			},
			occurrence: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "trinket",
					Meaning:    "a small ornament or piece of jewelry",
				},
			},
			wantMeaningInOutput: "a small ornament",
			wantReasonInOutput:  "partial match: user captured main sense",
		},
		{
			name: "Incorrect answer shows correct meaning from notebook with context",
			answer: AnswerResponse{
				Correct:    false,
				Expression: "trinket",
				Meaning:    "a container for cooking", // User's wrong answer
				Context:    "She wore a beautiful trinket on her necklace",
				Reason:     "A3 - unrelated: user said 'container for cooking' but it means 'small ornament'",
			},
			occurrence: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "trinket",
					Meaning:    "a small ornament or piece of jewelry", // Correct meaning
				},
				Scene: &notebook.StoryScene{
					Definitions: []notebook.Note{
						{Expression: "trinket", Meaning: "a small ornament or piece of jewelry"},
					},
				},
			},
			wantMeaningInOutput: "a small ornament or piece of jewelry",
			wantNotInOutput:     "a container for cooking",
			wantContextInOutput: "She wore a beautiful trinket on her necklace",
			wantReasonInOutput:  "A3 - unrelated: user said 'container for cooking' but it means 'small ornament'",
		},
		{
			name: "Correct answer with context and scene definitions",
			answer: AnswerResponse{
				Correct:    true,
				Expression: "trinket",
				Meaning:    "a small ornament",
				Context:    "She wore a beautiful {{ trinket }} on her necklace",
			},
			occurrence: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "trinket",
					Meaning:    "a small ornament or piece of jewelry",
				},
				Scene: &notebook.StoryScene{
					Definitions: []notebook.Note{
						{Expression: "trinket", Meaning: "a small ornament or piece of jewelry"},
					},
				},
			},
			wantMeaningInOutput: "a small ornament",
			wantContextInOutput: "She wore a beautiful",
		},
		{
			name: "Correct answer with context but nil scene (flashcard)",
			answer: AnswerResponse{
				Correct:    true,
				Expression: "trinket",
				Meaning:    "a small ornament",
				Context:    "She wore a beautiful trinket on her necklace",
			},
			occurrence: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "trinket",
					Meaning:    "a small ornament or piece of jewelry",
				},
			},
			wantMeaningInOutput: "a small ornament",
			wantContextInOutput: "She wore a beautiful trinket on her necklace",
		},
		{
			name: "Incorrect answer with no occurrence falls back to answer.Meaning",
			answer: AnswerResponse{
				Correct:    false,
				Expression: "test",
				Meaning:    "wrong meaning",
			},
			occurrence:          nil,
			wantMeaningInOutput: "wrong meaning",
		},
		{
			name: "Incorrect answer with empty notebook meaning falls back to answer.Meaning",
			answer: AnswerResponse{
				Correct:    false,
				Expression: "test",
				Meaning:    "wrong meaning",
			},
			occurrence: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "test",
					Meaning:    "", // Empty meaning in notebook
				},
			},
			wantMeaningInOutput: "wrong meaning",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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

			err := cli.displayResult(tc.answer, tc.occurrence)
			require.NoError(t, err)

			outputStr := buf.String()
			assert.Contains(t, outputStr, tc.wantMeaningInOutput, "Output should contain the expected meaning")

			if tc.wantNotInOutput != "" {
				assert.NotContains(t, outputStr, tc.wantNotInOutput, "Output should not contain the wrong meaning")
			}

			if tc.wantContextInOutput != "" {
				assert.Contains(t, outputStr, tc.wantContextInOutput, "Output should contain the context")
			}

			if tc.wantReasonInOutput != "" {
				assert.Contains(t, outputStr, tc.wantReasonInOutput, "Output should contain the reason")
			}
		})
	}
}

func TestFreeformQuizCLI_hasCorrectAnswer(t *testing.T) {
	t.Run("Duplicate expressions - first empty, second usable - returns true", func(t *testing.T) {
		// Setup: Create learning history with TWO expressions in the SAME scene:
		// 1. "run some ideas by someone" with empty learned_logs
		// 2. "run some ideas by us" with learned_logs containing status "usable"
		// GetLogs skips the empty-log entry and finds the usable one
		learningHistory := []notebook.LearningHistory{
			{
				Metadata: notebook.LearningHistoryMetadata{
					NotebookID: "test-notebook",
					Title:      "Lesson 3",
				},
				Scenes: []notebook.LearningScene{
					{
						Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []notebook.LearningHistoryExpression{
							{
								Expression:  "run some ideas by someone",
								LearnedLogs: []notebook.LearningRecord{},
							},
							{
								Expression: "run some ideas by us",
								LearnedLogs: []notebook.LearningRecord{
									{
										Status:    "usable",
										LearnedAt: notebook.NewDate(time.Now()),
									},
								},
							},
						},
					},
				},
			},
		}

		wordCtx := &WordOccurrence{
			Story: &notebook.StoryNotebook{
				Event: "Lesson 3",
			},
			Scene: &notebook.StoryScene{
				Title: "Scene 1",
			},
			Definition: &notebook.Note{
				Expression: "run some ideas by us",
				Definition: "run some ideas by someone",
				Meaning:    "to discuss some new ideas",
			},
		}

		cli := &FreeformQuizCLI{}

		word := "run some ideas by someone"

		result := cli.hasCorrectAnswer(learningHistory, wordCtx, word)

		// GetLogs now skips the empty-log entry and finds the usable status,
		// so the word is correctly identified as already having a correct answer
		assert.True(t, result, "hasCorrectAnswer should return true because GetLogs finds the usable logs from sibling expression")
	})
}

func TestFreeformQuizCLI_session(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		allStories        map[string][]notebook.StoryNotebook
		allFlashcards     map[string][]notebook.FlashcardNotebook
		learningHistories map[string][]notebook.LearningHistory
		setupMock         func(*mock_inference.MockClient)
		wantErr           bool
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
			name:       "Word not found in context",
			input:      "unknown\ntest meaning\n",
			allStories: map[string][]notebook.StoryNotebook{},
			setupMock: func(mockClient *mock_inference.MockClient) {
				// No expectation - word not found before API call
			},
		},
		{
			name:  "Word found and already mastered - no learning needed",
			input: "test\ntest meaning\n",
			allStories: map[string][]notebook.StoryNotebook{
				"test-notebook": {
					{
						Event: "Story 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "This is a test"},
								},
								Definitions: []notebook.Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{
				"test-notebook": {
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: "test-notebook",
							Title:      "Story 1",
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{
									Title: "Scene 1",
								},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression: "test",
										LearnedLogs: []notebook.LearningRecord{
											{
												Status:    "understood",
												LearnedAt: notebook.NewDate(time.Now()),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(mockClient *mock_inference.MockClient) {
				// No expectation - already mastered, no API call needed
			},
		},
		{
			name:  "Correct answer with context mismatch - still updates learning history",
			input: "test\ntest meaning\n",
			allStories: map[string][]notebook.StoryNotebook{
				"test-notebook": {
					{
						Event: "Story 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "This is a test"},
								},
								Definitions: []notebook.Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			setupMock: func(mockClient *mock_inference.MockClient) {
				mockClient.EXPECT().
					AnswerMeanings(gomock.Any(), gomock.Any()).
					Return(inference.AnswerMeaningsResponse{
						Answers: []inference.AnswerMeaning{
							{
								Expression: "test",
								Meaning:    "test meaning",
								AnswersForContext: []inference.AnswersForContext{
									// Context doesn't match the sent context ("This is a test")
									// This simulates when OpenAI returns a modified or different context
									{Correct: true, Context: "Different context string", Reason: "correct meaning"},
								},
							},
						},
					}, nil).
					Times(1)
			},
		},
		{
			name:  "Correct answer - updates learning history",
			input: "test\ntest meaning\n",
			allStories: map[string][]notebook.StoryNotebook{
				"test-notebook": {
					{
						Event: "Story 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "This is a test"},
								},
								Definitions: []notebook.Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
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
			allStories: map[string][]notebook.StoryNotebook{
				"test-notebook": {
					{
						Event: "Story 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "This is a test"},
								},
								Definitions: []notebook.Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
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
			allStories: map[string][]notebook.StoryNotebook{
				"test-notebook": {
					{
						Event: "Story 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "This is a test"},
								},
								Definitions: []notebook.Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			setupMock: func(mockClient *mock_inference.MockClient) {
				mockClient.EXPECT().
					AnswerMeanings(gomock.Any(), gomock.Any()).
					Return(inference.AnswerMeaningsResponse{Answers: []inference.AnswerMeaning{}}, nil).
					Times(1)
			},
			wantErr: true,
		},
		{
			name:       "Word found in flashcard context",
			input:      "hello\na greeting\n",
			allStories: map[string][]notebook.StoryNotebook{},
			allFlashcards: map[string][]notebook.FlashcardNotebook{
				"test-flashcard": {
					{
						Title: "Greetings",
						Cards: []notebook.Note{
							{Expression: "hello", Meaning: "a greeting used to say hi"},
						},
					},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			setupMock: func(mockClient *mock_inference.MockClient) {
				mockClient.EXPECT().
					AnswerMeanings(gomock.Any(), gomock.Any()).
					Return(inference.AnswerMeaningsResponse{
						Answers: []inference.AnswerMeaning{
							{
								Expression: "hello",
								Meaning:    "a greeting",
								AnswersForContext: []inference.AnswersForContext{
									{Correct: true, Context: "Hello there!", Reason: "correct"},
								},
							},
						},
					}, nil).
					Times(1)
			},
		},
		{
			name:  "Incorrect answer - does not update learning history",
			input: "test\nwrong meaning\n",
			allStories: map[string][]notebook.StoryNotebook{
				"test-notebook": {
					{
						Event: "Story 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "This is a test"},
								},
								Definitions: []notebook.Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			setupMock: func(mockClient *mock_inference.MockClient) {
				mockClient.EXPECT().
					AnswerMeanings(gomock.Any(), gomock.Any()).
					Return(inference.AnswerMeaningsResponse{
						Answers: []inference.AnswerMeaning{
							{
								Expression: "test",
								Meaning:    "wrong meaning",
								AnswersForContext: []inference.AnswersForContext{
									{Correct: false, Context: "This is a test", Reason: "A3 - unrelated: meanings are from different semantic fields"},
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

			allFlashcards := tt.allFlashcards
			if allFlashcards == nil {
				allFlashcards = make(map[string][]notebook.FlashcardNotebook)
			}
			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					learningNotesDir:  t.TempDir(),
					learningHistories: tt.learningHistories,
					dictionaryMap:     make(map[string]rapidapi.Response),
					openaiClient:      mockClient,
					stdinReader:       stdinReader,
					stdoutWriter:      os.Stdout,
					bold:              color.New(color.Bold),
					italic:            color.New(color.Italic),
				},
				allStories:    tt.allStories,
				allFlashcards: allFlashcards,
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

func TestFreeformQuizCLI_UpdateLearningHistory_AlwaysRecordsDefinitionForm(t *testing.T) {
	tests := []struct {
		name                   string
		userTypedWord          string // What the user actually typed
		noteExpression         string // The Expression field in the notebook
		noteDefinition         string // The Definition field in the notebook (the canonical base form)
		wantRecordedExpression string // What should be recorded in learning notes
	}{
		{
			name:                   "User types Expression form - should record Definition form",
			userTypedWord:          "run some ideas by us",
			noteExpression:         "run some ideas by us",
			noteDefinition:         "run some ideas by someone",
			wantRecordedExpression: "run some ideas by someone", // Definition form should be recorded
		},
		{
			name:                   "User types Definition form - should record Definition form",
			userTypedWord:          "run some ideas by someone",
			noteExpression:         "run some ideas by us",
			noteDefinition:         "run some ideas by someone",
			wantRecordedExpression: "run some ideas by someone", // Definition form should be recorded
		},
		{
			name:                   "Definition field is empty - should record Expression",
			userTypedWord:          "simple word",
			noteExpression:         "simple word",
			noteDefinition:         "", // Empty Definition field
			wantRecordedExpression: "simple word", // Expression should be recorded as fallback
		},
		{
			name:                   "Case insensitive: User types definition in different case",
			userTypedWord:          "RUN SOME IDEAS BY SOMEONE",
			noteExpression:         "run some ideas by us",
			noteDefinition:         "run some ideas by someone",
			wantRecordedExpression: "run some ideas by someone", // Definition form should be recorded
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create a story with Expression and Definition fields
			testStories := map[string][]notebook.StoryNotebook{
				"test-notebook": {
					{
						Event: "Test Story",
						Scenes: []notebook.StoryScene{
							{
								Title: "Test Scene",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: fmt.Sprintf("Context for %s", tt.noteExpression)},
								},
								Definitions: []notebook.Note{
									{
										Expression: tt.noteExpression,
										Definition: tt.noteDefinition,
										Meaning:    "test meaning",
									},
								},
							},
						},
					},
				},
			}

			// Start with empty learning history
			testLearningHistories := map[string][]notebook.LearningHistory{
				"test-notebook": {},
			}

			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					learningNotesDir:  tmpDir,
					learningHistories: testLearningHistories,
					stdoutWriter:      os.Stdout,
				},
				allStories: testStories,
			}

			// Find the word occurrence
			wordContexts := cli.findAllWordContexts(tt.userTypedWord)
			require.Len(t, wordContexts, 1, "Should find 1 occurrence")

			needsLearning := cli.findOccurrencesNeedingLearning(wordContexts, tt.userTypedWord)
			require.Len(t, needsLearning, 1, "Should need learning")

			// Simulate correct answer
			answer := AnswerResponse{
				Correct:    true,
				Expression: tt.userTypedWord,
				Meaning:    "test meaning",
			}

			// Update learning history
			err := cli.updateLearningHistory(needsLearning[0], tt.userTypedWord, answer, 4, 1000)
			require.NoError(t, err)

			// Read the saved history
			learningNotePath := filepath.Join(tmpDir, "test-notebook.yml")
			var savedHistory []notebook.LearningHistory
			file, err := os.Open(learningNotePath)
			require.NoError(t, err)
			defer file.Close()

			err = yaml.NewDecoder(file).Decode(&savedHistory)
			require.NoError(t, err)

			// Verify the EXPECTED expression (Definition form if available, Expression otherwise) was recorded
			gotStatus := findExpressionStatus(savedHistory, "Test Story", "Test Scene", tt.wantRecordedExpression)
			require.NotNil(t, gotStatus, "Expression '%s' should be recorded in learning history", tt.wantRecordedExpression)
			assert.Equal(t, notebook.LearnedStatus("usable"), *gotStatus, "Status should be 'usable'")

			// Verify that the OTHER form was NOT recorded (to prevent duplicates)
			var otherForm string
			if tt.noteDefinition != "" && tt.wantRecordedExpression == tt.noteDefinition {
				// We recorded the Definition, so Expression should NOT be recorded
				otherForm = tt.noteExpression
			} else if tt.noteDefinition != "" && tt.wantRecordedExpression == tt.noteExpression {
				// We recorded the Expression (because Definition is empty), so Definition should NOT be recorded
				otherForm = tt.noteDefinition
			}

			if otherForm != "" && otherForm != tt.wantRecordedExpression {
				otherStatus := findExpressionStatus(savedHistory, "Test Story", "Test Scene", otherForm)
				assert.Nil(t, otherStatus, "The other form '%s' should NOT be recorded to avoid duplicates", otherForm)
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
								Conversations: []notebook.Conversation{{Speaker: "A", Quote: "A {{ tricky }} situation."}},
								Definitions:   []notebook.Note{{Expression: "tricky", Meaning: "difficult to deal with"}},
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
		name       string
		stories    map[string][]notebook.StoryNotebook
		flashcards map[string][]notebook.FlashcardNotebook
		want       int
	}{
		{
			name:       "empty",
			stories:    map[string][]notebook.StoryNotebook{},
			flashcards: map[string][]notebook.FlashcardNotebook{},
			want:       0,
		},
		{
			name: "story definitions counted",
			stories: map[string][]notebook.StoryNotebook{
				"test": {
					{
						Scenes: []notebook.StoryScene{
							{Definitions: []notebook.Note{{Expression: "a"}, {Expression: "b"}}},
						},
					},
				},
			},
			flashcards: map[string][]notebook.FlashcardNotebook{},
			want:       2,
		},
		{
			name:    "flashcard cards counted",
			stories: map[string][]notebook.StoryNotebook{},
			flashcards: map[string][]notebook.FlashcardNotebook{
				"test": {
					{Cards: []notebook.Note{{Expression: "a"}, {Expression: "b"}, {Expression: "c"}}},
				},
			},
			want: 3,
		},
		{
			name: "combined count",
			stories: map[string][]notebook.StoryNotebook{
				"story1": {
					{Scenes: []notebook.StoryScene{{Definitions: []notebook.Note{{Expression: "a"}}}}},
				},
			},
			flashcards: map[string][]notebook.FlashcardNotebook{
				"flash1": {
					{Cards: []notebook.Note{{Expression: "b"}}},
				},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{},
				allStories:         tt.stories,
				allFlashcards:      tt.flashcards,
			}
			assert.Equal(t, tt.want, cli.WordCount())
		})
	}
}

func TestFreeformQuizCLI_findAllWordContexts(t *testing.T) {
	tests := []struct {
		name       string
		stories    map[string][]notebook.StoryNotebook
		flashcards map[string][]notebook.FlashcardNotebook
		word       string
		wantCount  int
		validate   func(t *testing.T, result []*WordOccurrence)
	}{
		{
			name:       "empty notebooks",
			stories:    map[string][]notebook.StoryNotebook{},
			flashcards: map[string][]notebook.FlashcardNotebook{},
			word:       "hello",
			wantCount:  0,
		},
		{
			name: "finds word in story conversations",
			stories: map[string][]notebook.StoryNotebook{
				"test-story": {
					{
						Event: "Story 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "I need to study hard"},
								},
								Definitions: []notebook.Note{
									{Expression: "study", Meaning: "to learn"},
								},
							},
						},
					},
				},
			},
			flashcards: map[string][]notebook.FlashcardNotebook{},
			word:       "study",
			wantCount:  1,
			validate: func(t *testing.T, result []*WordOccurrence) {
				assert.Equal(t, "test-story", result[0].NotebookName)
				assert.NotNil(t, result[0].Story)
				assert.NotNil(t, result[0].Scene)
			},
		},
		{
			name:    "finds word in flashcards",
			stories: map[string][]notebook.StoryNotebook{},
			flashcards: map[string][]notebook.FlashcardNotebook{
				"test-flashcard": {
					{
						Title: "Common Words",
						Cards: []notebook.Note{
							{
								Expression: "break the ice",
								Meaning:    "to initiate social interaction",
								Examples:   []string{"She told a joke to break the ice."},
							},
						},
					},
				},
			},
			word:      "break the ice",
			wantCount: 1,
			validate: func(t *testing.T, result []*WordOccurrence) {
				assert.Equal(t, "test-flashcard", result[0].NotebookName)
				assert.Nil(t, result[0].Story)
				assert.Nil(t, result[0].Scene)
				assert.Equal(t, 1, len(result[0].Contexts))
				assert.Equal(t, "break the ice", result[0].Contexts[0].Usage)
			},
		},
		{
			name: "finds word in both story and flashcard",
			stories: map[string][]notebook.StoryNotebook{
				"test-story": {
					{
						Event: "Story 1",
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "Time to study"},
								},
								Definitions: []notebook.Note{
									{Expression: "study", Meaning: "to learn"},
								},
							},
						},
					},
				},
			},
			flashcards: map[string][]notebook.FlashcardNotebook{
				"test-flashcard": {
					{
						Title: "Words",
						Cards: []notebook.Note{
							{Expression: "study", Meaning: "to learn by reading"},
						},
					},
				},
			},
			word:      "study",
			wantCount: 2,
		},
		{
			name: "case insensitive match in flashcards",
			stories: map[string][]notebook.StoryNotebook{},
			flashcards: map[string][]notebook.FlashcardNotebook{
				"test": {
					{
						Title: "Words",
						Cards: []notebook.Note{
							{Expression: "Hello", Meaning: "a greeting"},
						},
					},
				},
			},
			word:      "hello",
			wantCount: 1,
		},
		{
			name:    "match via definition field in flashcard",
			stories: map[string][]notebook.StoryNotebook{},
			flashcards: map[string][]notebook.FlashcardNotebook{
				"test": {
					{
						Title: "Words",
						Cards: []notebook.Note{
							{Expression: "lost his temper", Definition: "lose one's temper", Meaning: "to become angry"},
						},
					},
				},
			},
			word:      "lose one's temper",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{},
				allStories:         tt.stories,
				allFlashcards:      tt.flashcards,
			}
			result := cli.findAllWordContexts(tt.word)
			assert.Equal(t, tt.wantCount, len(result))
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestFreeformQuizCLI_hasCorrectAnswer_FlashcardAndStory(t *testing.T) {
	tests := []struct {
		name            string
		learningHistory []notebook.LearningHistory
		wordCtx         *WordOccurrence
		word            string
		want            bool
	}{
		{
			name: "flashcard type with understood status and recent threshold - mastered",
			learningHistory: []notebook.LearningHistory{
				{
					Metadata: notebook.LearningHistoryMetadata{
						Title: "flashcards",
						Type:  "flashcard",
					},
					Expressions: []notebook.LearningHistoryExpression{
						{
							Expression: "break the ice",
							LearnedLogs: []notebook.LearningRecord{
								{Status: "understood", LearnedAt: notebook.NewDate(time.Now()), IntervalDays: 7},
							},
						},
					},
				},
			},
			wordCtx: &WordOccurrence{
				Definition: &notebook.Note{Expression: "break the ice", Meaning: "to initiate social interaction"},
			},
			word: "break the ice",
			want: true,
		},
		{
			name: "flashcard type with understood status and expired threshold - needs learning",
			learningHistory: []notebook.LearningHistory{
				{
					Metadata: notebook.LearningHistoryMetadata{
						Title: "flashcards",
						Type:  "flashcard",
					},
					Expressions: []notebook.LearningHistoryExpression{
						{
							Expression: "break the ice",
							LearnedLogs: []notebook.LearningRecord{
								{Status: "understood", LearnedAt: notebook.NewDate(time.Now().Add(-30 * 24 * time.Hour)), IntervalDays: 7},
							},
						},
					},
				},
			},
			wordCtx: &WordOccurrence{
				Definition: &notebook.Note{Expression: "break the ice", Meaning: "to initiate social interaction"},
			},
			word: "break the ice",
			want: false,
		},
		{
			name: "story type with understood status and recent threshold - mastered",
			learningHistory: []notebook.LearningHistory{
				{
					Metadata: notebook.LearningHistoryMetadata{
						Title: "Story 1",
					},
					Scenes: []notebook.LearningScene{
						{
							Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []notebook.LearningHistoryExpression{
								{
									Expression: "study",
									LearnedLogs: []notebook.LearningRecord{
										{Status: "understood", LearnedAt: notebook.NewDate(time.Now()), IntervalDays: 7},
									},
								},
							},
						},
					},
				},
			},
			wordCtx: &WordOccurrence{
				Story:      &notebook.StoryNotebook{Event: "Story 1"},
				Scene:      &notebook.StoryScene{Title: "Scene 1"},
				Definition: &notebook.Note{Expression: "study", Meaning: "to learn"},
			},
			word: "study",
			want: true,
		},
		{
			name:            "no matching learning history",
			learningHistory: []notebook.LearningHistory{},
			wordCtx: &WordOccurrence{
				Definition: &notebook.Note{Expression: "hello", Meaning: "a greeting"},
			},
			word: "hello",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &FreeformQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{},
			}
			got := cli.hasCorrectAnswer(tt.learningHistory, tt.wordCtx, tt.word)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFreeformQuizCLI_hasThresholdPassed(t *testing.T) {
	tests := []struct {
		name string
		logs []notebook.LearningRecord
		want bool
	}{
		{
			name: "empty logs - threshold passed",
			logs: []notebook.LearningRecord{},
			want: true,
		},
		{
			name: "recent log with interval - threshold not passed",
			logs: []notebook.LearningRecord{
				{
					Status:       "understood",
					LearnedAt:    notebook.NewDate(time.Now()),
					IntervalDays: 7,
				},
			},
			want: false,
		},
		{
			name: "old log with interval - threshold passed",
			logs: []notebook.LearningRecord{
				{
					Status:       "understood",
					LearnedAt:    notebook.NewDate(time.Now().Add(-10 * 24 * time.Hour)),
					IntervalDays: 7,
				},
			},
			want: true,
		},
		{
			name: "legacy log without interval - uses count-based threshold",
			logs: []notebook.LearningRecord{
				{
					Status:    "understood",
					LearnedAt: notebook.NewDate(time.Now()),
				},
			},
			want: false,
		},
		{
			name: "only learning/misunderstood logs - uses count 0 threshold",
			logs: []notebook.LearningRecord{
				{
					Status:    "",
					LearnedAt: notebook.NewDate(time.Now().Add(-100 * 24 * time.Hour)),
				},
				{
					Status:    "misunderstood",
					LearnedAt: notebook.NewDate(time.Now().Add(-50 * 24 * time.Hour)),
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &FreeformQuizCLI{}
			got := cli.hasThresholdPassed(tt.logs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFreeformQuizCLI_isExpressionMatch(t *testing.T) {
	tests := []struct {
		name    string
		expr    notebook.LearningHistoryExpression
		wordCtx *WordOccurrence
		word    string
		want    bool
	}{
		{
			name: "direct expression match",
			expr: notebook.LearningHistoryExpression{
				Expression: "break the ice",
			},
			wordCtx: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "break the ice",
					Meaning:    "to initiate social interaction",
				},
			},
			word: "break the ice",
			want: true,
		},
		{
			name: "no match",
			expr: notebook.LearningHistoryExpression{
				Expression: "different word",
			},
			wordCtx: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "break the ice",
					Meaning:    "to initiate social interaction",
				},
			},
			word: "break the ice",
			want: false,
		},
		{
			name: "match via definition field",
			expr: notebook.LearningHistoryExpression{
				Expression: "lose one's temper",
			},
			wordCtx: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "lost his temper",
					Definition: "lose one's temper",
					Meaning:    "to become angry",
				},
			},
			word: "lost his temper",
			want: true,
		},
		{
			name: "user practices definition form - expression in history matches",
			expr: notebook.LearningHistoryExpression{
				Expression: "lost his temper",
			},
			wordCtx: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "lost his temper",
					Definition: "lose one's temper",
					Meaning:    "to become angry",
				},
			},
			word: "lose one's temper",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &FreeformQuizCLI{}
			got := cli.isExpressionMatch(tt.expr, tt.wordCtx, tt.word)
			assert.Equal(t, tt.want, got)
		})
	}
}
