package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestFormatReverseQuestion(t *testing.T) {
	tests := []struct {
		name string
		card *quiz.ReverseCard
		want string
	}{
		{
			name: "With context",
			card: &quiz.ReverseCard{
				Expression: "excited",
				Meaning:    "feeling or showing enthusiasm",
				Contexts: []quiz.ReverseContext{
					{Context: "I am so excited about the trip!", MaskedContext: "I am so ______ about the trip!"},
				},
			},
			want: "Meaning: feeling or showing enthusiasm\nContext:\n  1. I am so ______ about the trip!\n\n",
		},
		{
			name: "Without context",
			card: &quiz.ReverseCard{
				Expression: "cavalry",
				Meaning:    "soldiers who fight on horseback",
				Contexts:   []quiz.ReverseContext{},
			},
			want: "Meaning: soldiers who fight on horseback\nContext: (no context available - this word may be difficult to answer)\n\n",
		},
		{
			name: "Multiple contexts",
			card: &quiz.ReverseCard{
				Expression: "thrilled",
				Meaning:    "extremely pleased and excited",
				Contexts: []quiz.ReverseContext{
					{Context: "She was thrilled to hear the news.", MaskedContext: "She was ______ to hear the news."},
					{Context: "We are thrilled about the results.", MaskedContext: "We are ______ about the results."},
				},
			},
			want: "Meaning: extremely pleased and excited\nContext:\n  1. She was ______ to hear the news.\n  2. We are ______ about the results.\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatReverseQuestion(tt.card)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReverseQuizCLI_ShuffleCards(t *testing.T) {
	cardWithContext := quiz.ReverseCard{
		Expression: "hello",
		Meaning:    "a greeting",
		Contexts:   []quiz.ReverseContext{{Context: "Hello there!", MaskedContext: "______ there!"}},
	}
	cardWithoutContext := quiz.ReverseCard{
		Expression: "world",
		Meaning:    "the earth",
		Contexts:   []quiz.ReverseContext{},
	}

	cli := &ReverseQuizCLI{
		cards: []quiz.ReverseCard{cardWithoutContext, cardWithContext},
	}

	// ShuffleCards should preserve the partition: no-context first, with-context last
	cli.ShuffleCards()

	// With only 1 card in each group, the partition should be maintained
	assert.Equal(t, 0, len(cli.cards[0].Contexts))
	assert.Greater(t, len(cli.cards[1].Contexts), 0)
}

func TestReverseQuizCLI_ListMissingContext(t *testing.T) {
	t.Run("empty cards", func(t *testing.T) {
		cli := &ReverseQuizCLI{
			InteractiveQuizCLI: &InteractiveQuizCLI{
				bold: color.New(color.Bold),
			},
			cards: []quiz.ReverseCard{},
		}
		// Should not panic
		cli.ListMissingContext()
	})

	t.Run("cards with missing context", func(t *testing.T) {
		cli := &ReverseQuizCLI{
			InteractiveQuizCLI: &InteractiveQuizCLI{
				bold: color.New(color.Bold),
			},
			cards: []quiz.ReverseCard{
				{
					NotebookName: "test-notebook",
					Expression:   "hello",
					Meaning:      "a greeting",
					Contexts:     []quiz.ReverseContext{},
				},
			},
		}
		// Should not panic
		cli.ListMissingContext()
	})
}

func TestSortReverseCardsByContextAvailability(t *testing.T) {
	cardWithContext := quiz.ReverseCard{
		Expression: "excited",
		Meaning:    "feeling enthusiasm",
		Contexts:   []quiz.ReverseContext{{Context: "I am excited!", MaskedContext: "I am ______!"}},
	}
	cardWithoutContext1 := quiz.ReverseCard{
		Expression: "lonely",
		Meaning:    "feeling alone",
		Contexts:   []quiz.ReverseContext{},
	}
	cardWithoutContext2 := quiz.ReverseCard{
		Expression: "melancholy",
		Meaning:    "deep sadness",
		Contexts:   []quiz.ReverseContext{},
	}

	tests := []struct {
		name      string
		input     []quiz.ReverseCard
		wantOrder []string
	}{
		{
			name:      "Words without context come first",
			input:     []quiz.ReverseCard{cardWithContext, cardWithoutContext1},
			wantOrder: []string{"lonely", "excited"},
		},
		{
			name:      "Multiple words without context",
			input:     []quiz.ReverseCard{cardWithContext, cardWithoutContext1, cardWithoutContext2},
			wantOrder: []string{"lonely", "melancholy", "excited"},
		},
		{
			name:      "All words have context",
			input:     []quiz.ReverseCard{cardWithContext},
			wantOrder: []string{"excited"},
		},
		{
			name:      "All words missing context",
			input:     []quiz.ReverseCard{cardWithoutContext1, cardWithoutContext2},
			wantOrder: []string{"lonely", "melancholy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sortReverseCardsByContextAvailability(tt.input)
			assert.Equal(t, len(tt.wantOrder), len(result))
			for i, expr := range tt.wantOrder {
				assert.Equal(t, expr, result[i].Expression)
			}
		})
	}
}

func TestReverseQuizCLI_ValidateAnswer(t *testing.T) {
	tests := []struct {
		name               string
		card               *quiz.ReverseCard
		answer             string
		responseTimeMs     int64
		mockResponse       inference.ValidateWordFormResponse
		mockError          error
		wantCorrect        bool
		wantQuality        int
		wantReason         string
		wantReasonContains string
	}{
		{
			name:           "same word - exact match",
			card:           &quiz.ReverseCard{Expression: "excited", Meaning: "feeling enthusiasm"},
			answer:         "excited",
			responseTimeMs: 2000,
			mockResponse:   inference.ValidateWordFormResponse{Classification: inference.ClassificationSameWord, Reason: "exact match", Quality: int(notebook.QualityCorrectFast)},
			wantCorrect:    true,
			wantQuality:    int(notebook.QualityCorrectFast),
			wantReason:     "exact match",
		},
		{
			name:           "same word - case insensitive",
			card:           &quiz.ReverseCard{Expression: "excited", Meaning: "feeling enthusiasm"},
			answer:         "EXCITED",
			responseTimeMs: 2000,
			mockResponse:   inference.ValidateWordFormResponse{Classification: inference.ClassificationSameWord, Reason: "same word different case", Quality: int(notebook.QualityCorrectFast)},
			wantCorrect:    true,
			wantQuality:    int(notebook.QualityCorrectFast),
			wantReason:     "same word different case",
		},
		{
			name:           "same word - different tense",
			card:           &quiz.ReverseCard{Expression: "run", Meaning: "to move quickly on foot"},
			answer:         "ran",
			responseTimeMs: 2000,
			mockResponse:   inference.ValidateWordFormResponse{Classification: inference.ClassificationSameWord, Reason: "different tense of the same word", Quality: int(notebook.QualityCorrectFast)},
			wantCorrect:    true,
			wantQuality:    int(notebook.QualityCorrectFast),
			wantReason:     "different tense of the same word",
		},
		{
			name:           "same word - slow response gets lower quality",
			card:           &quiz.ReverseCard{Expression: "excited", Meaning: "feeling enthusiasm"},
			answer:         "excited",
			responseTimeMs: 15000,
			mockResponse:   inference.ValidateWordFormResponse{Classification: inference.ClassificationSameWord, Reason: "exact match", Quality: int(notebook.QualityCorrectSlow)},
			wantCorrect:    true,
			wantQuality:    int(notebook.QualityCorrectSlow),
			wantReason:     "exact match",
		},
		{
			name:           "wrong classification",
			card:           &quiz.ReverseCard{Expression: "excited", Meaning: "feeling enthusiasm"},
			answer:         "apple",
			responseTimeMs: 2000,
			mockResponse:   inference.ValidateWordFormResponse{Classification: inference.ClassificationWrong, Reason: "unrelated word", Quality: 1},
			wantCorrect:    false,
			wantQuality:    int(notebook.QualityWrong),
			wantReason:     "unrelated word",
		},
		{
			name:           "empty answer",
			card:           &quiz.ReverseCard{Expression: "excited", Meaning: "feeling enthusiasm"},
			answer:         "",
			responseTimeMs: 2000,
			wantCorrect:    false,
			wantQuality:    int(notebook.QualityWrong),
			wantReason:     "empty answer",
		},
		{
			name:           "wrong answer different word",
			card:           &quiz.ReverseCard{Expression: "correct-word", Meaning: "the right word"},
			answer:         "wrong-word",
			responseTimeMs: 5000,
			mockResponse:   inference.ValidateWordFormResponse{Classification: inference.ClassificationWrong, Reason: "different word", Quality: 1},
			wantCorrect:    false,
			wantQuality:    int(notebook.QualityWrong),
			wantReason:     "different word",
		},
		{
			name:               "validation error",
			card:               &quiz.ReverseCard{Expression: "correct-word", Meaning: "the right word"},
			answer:             "some-word",
			responseTimeMs:     5000,
			mockResponse:       inference.ValidateWordFormResponse{},
			mockError:          fmt.Errorf("API error"),
			wantCorrect:        false,
			wantQuality:        int(notebook.QualityWrong),
			wantReasonContains: "validation error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mock_inference.NewMockClient(ctrl)
			// Empty answer skips the API call
			if tt.answer != "" {
				mockClient.EXPECT().
					ValidateWordForm(gomock.Any(), gomock.Any()).
					Return(tt.mockResponse, tt.mockError)
			}

			svc := quiz.NewService(config.NotebooksConfig{}, mockClient, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository("", nil), config.QuizConfig{})

			cli := &ReverseQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					openaiClient: mockClient,
				},
				svc: svc,
			}

			grade, err := cli.gradeWithSynonymRetry(context.Background(), tt.card, tt.answer, tt.responseTimeMs, false)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCorrect, grade.Correct)
			assert.Equal(t, tt.wantQuality, grade.Quality)
			if tt.wantReasonContains != "" {
				assert.Contains(t, grade.Reason, tt.wantReasonContains)
			} else {
				assert.Equal(t, tt.wantReason, grade.Reason)
			}
		})
	}
}

func TestReverseQuizCLI_Session(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		cards              []quiz.ReverseCard
		mockOpenAIResponse inference.ValidateWordFormResponse
		mockOpenAIError    error
		wantErr            bool
		wantReturn         error
		wantCardsAfter     int
	}{
		{
			name:           "No more cards - returns errEnd",
			input:          "",
			cards:          []quiz.ReverseCard{},
			wantReturn:     errEnd,
			wantCardsAfter: 0,
		},
		{
			name:  "Correct exact match - removes card",
			input: "excited\n",
			cards: []quiz.ReverseCard{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Expression:   "excited",
					Meaning:      "feeling enthusiasm",
					Contexts:     []quiz.ReverseContext{},
				},
			},
			mockOpenAIResponse: inference.ValidateWordFormResponse{
				Classification: inference.ClassificationSameWord,
				Reason:         "exact match",
				Quality:        int(notebook.QualityCorrect),
			},
			wantCardsAfter: 0,
		},
		{
			name:  "Wrong answer - removes card",
			input: "apple\n",
			cards: []quiz.ReverseCard{
				{
					NotebookName: "test-notebook",
					StoryTitle:   "Story 1",
					SceneTitle:   "Scene 1",
					Expression:   "excited",
					Meaning:      "feeling enthusiasm",
					Contexts:     []quiz.ReverseContext{},
				},
			},
			mockOpenAIResponse: inference.ValidateWordFormResponse{
				Classification: inference.ClassificationWrong,
				Reason:         "unrelated word",
				Quality:        int(notebook.QualityWrong),
			},
			wantCardsAfter: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			stdinReader := bufio.NewReader(strings.NewReader(tt.input))
			mockClient := mock_inference.NewMockClient(ctrl)

			// All non-empty answers go through ValidateWordForm
			if len(tt.cards) > 0 && strings.TrimSpace(tt.input) != "" {
				mockClient.EXPECT().
					ValidateWordForm(gomock.Any(), gomock.Any()).
					Return(tt.mockOpenAIResponse, tt.mockOpenAIError).
					AnyTimes()
			}

			learningNotesDir := t.TempDir()
			// Create a minimal learning history file so SaveReverseResult can work
			if len(tt.cards) > 0 {
				learningHistory := []notebook.LearningHistory{
					{
						Metadata: notebook.LearningHistoryMetadata{
							NotebookID: tt.cards[0].NotebookName,
							Title:      tt.cards[0].StoryTitle,
						},
						Scenes: []notebook.LearningScene{
							{
								Metadata: notebook.LearningSceneMetadata{Title: tt.cards[0].SceneTitle},
								Expressions: []notebook.LearningHistoryExpression{
									{
										Expression:  tt.cards[0].Expression,
										LearnedLogs: []notebook.LearningRecord{{Status: "understood"}},
									},
								},
							},
						},
					},
				}
				learningHistoryPath := filepath.Join(learningNotesDir, tt.cards[0].NotebookName+".yml")
				require.NoError(t, notebook.WriteYamlFile(learningHistoryPath, learningHistory))
			}

			notebooksConfig := config.NotebooksConfig{
				LearningNotesDirectory: learningNotesDir,
			}
			svc := quiz.NewService(notebooksConfig, mockClient, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(notebooksConfig.LearningNotesDirectory, nil), config.QuizConfig{})

			cli := &ReverseQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					learningNotesDir:  learningNotesDir,
					learningHistories: make(map[string][]notebook.LearningHistory),
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

			assert.Equal(t, tt.wantCardsAfter, len(cli.cards))
		})
	}
}

func TestNewReverseQuizCLI(t *testing.T) {
	tests := []struct {
		name               string
		setupFunc          func(t *testing.T) (storiesDir, flashcardsDir, learningNotesDir string)
		notebookName       string
		listMissingContext bool
		wantCardCount      int
		validate           func(t *testing.T, cli *ReverseQuizCLI)
	}{
		{
			name: "Word with no reverse logs needs review",
			setupFunc: func(t *testing.T) (string, string, string) {
				storiesDir := t.TempDir()
				learningNotesDir := t.TempDir()

				notebookDir := filepath.Join(storiesDir, "test-notebook")
				require.NoError(t, os.MkdirAll(notebookDir, 0755))

				index := notebook.Index{
					Kind:          "story",
					ID:            "test-notebook",
					Name:          "Test Notebook",
					NotebookPaths: []string{"stories.yml"},
				}
				indexPath := filepath.Join(notebookDir, "index.yml")
				require.NoError(t, notebook.WriteYamlFile(indexPath, index))

				stories := []notebook.StoryNotebook{
					{
						Event: "Story 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "I am excited about this!"},
								},
								Definitions: []notebook.Note{
									{Expression: "excited", Meaning: "feeling enthusiasm"},
								},
							},
						},
					},
				}
				storiesPath := filepath.Join(notebookDir, "stories.yml")
				require.NoError(t, notebook.WriteYamlFile(storiesPath, stories))

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
										Expression: "excited",
										LearnedLogs: []notebook.LearningRecord{
											{Status: "usable", LearnedAt: notebook.NewDate(time.Now().Add(-365 * 24 * time.Hour)), QuizType: string(notebook.QuizTypeFreeform)},
										},
									},
								},
							},
						},
					},
				}
				learningHistoryPath := filepath.Join(learningNotesDir, "test-notebook.yml")
				require.NoError(t, notebook.WriteYamlFile(learningHistoryPath, learningHistory))

				return storiesDir, "", learningNotesDir
			},
			notebookName:       "test-notebook",
			listMissingContext: false,
			wantCardCount:      1,
		},
		{
			name: "Word with recent reverse review skipped",
			setupFunc: func(t *testing.T) (string, string, string) {
				storiesDir := t.TempDir()
				learningNotesDir := t.TempDir()

				notebookDir := filepath.Join(storiesDir, "test-notebook")
				require.NoError(t, os.MkdirAll(notebookDir, 0755))

				index := notebook.Index{
					Kind:          "story",
					ID:            "test-notebook",
					Name:          "Test Notebook",
					NotebookPaths: []string{"stories.yml"},
				}
				indexPath := filepath.Join(notebookDir, "index.yml")
				require.NoError(t, notebook.WriteYamlFile(indexPath, index))

				stories := []notebook.StoryNotebook{
					{
						Event: "Story 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "I am excited about this!"},
								},
								Definitions: []notebook.Note{
									{Expression: "excited", Meaning: "feeling enthusiasm"},
								},
							},
						},
					},
				}
				storiesPath := filepath.Join(notebookDir, "stories.yml")
				require.NoError(t, notebook.WriteYamlFile(storiesPath, stories))

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
										Expression: "excited",
										LearnedLogs: []notebook.LearningRecord{
											{Status: "understood"},
										},
										ReverseLogs: []notebook.LearningRecord{
											{
												Status:       "usable",
												LearnedAt:    notebook.NewDate(time.Now()),
												IntervalDays: 7,
											},
										},
									},
								},
							},
						},
					},
				}
				learningHistoryPath := filepath.Join(learningNotesDir, "test-notebook.yml")
				require.NoError(t, notebook.WriteYamlFile(learningHistoryPath, learningHistory))

				return storiesDir, "", learningNotesDir
			},
			notebookName:       "test-notebook",
			listMissingContext: false,
			wantCardCount:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storiesDir, flashcardsDir, learningNotesDir := tt.setupFunc(t)
			dictionaryCacheDir := t.TempDir()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := mock_inference.NewMockClient(ctrl)

			var storiesDirs []string
			if storiesDir != "" {
				storiesDirs = []string{storiesDir}
			}
			var flashcardsDirs []string
			if flashcardsDir != "" {
				flashcardsDirs = []string{flashcardsDir}
			}

			cli, err := NewReverseQuizCLI(
				tt.notebookName,
				config.NotebooksConfig{
					StoriesDirectories:     storiesDirs,
					FlashcardsDirectories:  flashcardsDirs,
					LearningNotesDirectory: learningNotesDir,
				},
				dictionaryCacheDir,
				mockClient,
				tt.listMissingContext,
				config.QuizConfig{},
			)
			require.NoError(t, err)

			assert.Equal(t, tt.wantCardCount, cli.GetCardCount())

			if tt.validate != nil {
				tt.validate(t, cli)
			}
		})
	}
}

func TestNewReverseQuizCLI_MultiLineSceneTitle(t *testing.T) {
	storiesDir := t.TempDir()
	learningNotesDir := t.TempDir()

	notebookDir := filepath.Join(storiesDir, "test-notebook")
	require.NoError(t, os.MkdirAll(notebookDir, 0755))

	index := notebook.Index{
		Kind:          "story",
		ID:            "test-notebook",
		Name:          "Test Notebook",
		NotebookPaths: []string{"stories.yml"},
	}
	indexPath := filepath.Join(notebookDir, "index.yml")
	require.NoError(t, notebook.WriteYamlFile(indexPath, index))

	multiLineSceneTitle := "Alice writes the story.\nBob reviews the draft.\nThey publish the book.\n"

	stories := []notebook.StoryNotebook{
		{
			Event: "Story 1",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Scenes: []notebook.StoryScene{
				{
					Title: multiLineSceneTitle,
					Conversations: []notebook.Conversation{
						{Speaker: "A", Quote: "I am excited about this!"},
					},
					Definitions: []notebook.Note{
						{Expression: "excited", Meaning: "feeling enthusiasm"},
					},
				},
			},
		},
	}
	storiesPath := filepath.Join(notebookDir, "stories.yml")
	require.NoError(t, notebook.WriteYamlFile(storiesPath, stories))

	learningHistory := []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{
				NotebookID: "test-notebook",
				Title:      "Story 1",
			},
			Scenes: []notebook.LearningScene{
				{
					Metadata: notebook.LearningSceneMetadata{Title: multiLineSceneTitle},
					Expressions: []notebook.LearningHistoryExpression{
						{
							Expression: "excited",
							LearnedLogs: []notebook.LearningRecord{
								{Status: notebook.LearnedStatusCanBeUsed, QuizType: string(notebook.QuizTypeFreeform)},
							},
							ReverseLogs: []notebook.LearningRecord{
								{
									Status:       "usable",
									LearnedAt:    notebook.NewDate(time.Now()),
									IntervalDays: 7,
								},
							},
						},
					},
				},
			},
		},
	}
	learningHistoryPath := filepath.Join(learningNotesDir, "test-notebook.yml")
	require.NoError(t, notebook.WriteYamlFile(learningHistoryPath, learningHistory))

	readBackHistories, err := notebook.NewLearningHistories(learningNotesDir)
	require.NoError(t, err)

	readBackHistory := readBackHistories["test-notebook"]
	t.Logf("Original scene title: %q", multiLineSceneTitle)
	t.Logf("Read back scene title: %q", readBackHistory[0].Scenes[0].Metadata.Title)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	cli, err := NewReverseQuizCLI(
		"test-notebook",
		config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			LearningNotesDirectory: learningNotesDir,
		},
		t.TempDir(),
		mockClient,
		false,
		config.QuizConfig{},
	)
	require.NoError(t, err)

	assert.Equal(t, 0, cli.GetCardCount(), "Expected 0 cards because word has recent reverse review")
}

func TestNewReverseQuizCLI_YAMLRoundTrip(t *testing.T) {
	tests := []struct {
		name           string
		noteExpression string
		noteDefinition string
		histExpression string
		wantCardCount  int
	}{
		{
			name:           "Simple expression match",
			noteExpression: "excited",
			noteDefinition: "",
			histExpression: "excited",
			wantCardCount:  0,
		},
		{
			name:           "Definition field - history stores Expression",
			noteExpression: "on a roll",
			noteDefinition: "be on a roll",
			histExpression: "on a roll",
			wantCardCount:  0,
		},
		{
			name:           "Definition field - history stores Definition",
			noteExpression: "on a roll",
			noteDefinition: "be on a roll",
			histExpression: "be on a roll",
			wantCardCount:  0,
		},
		{
			name:           "No match - different expression (no learning history for word)",
			noteExpression: "excited",
			noteDefinition: "",
			histExpression: "different_word",
			wantCardCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storiesDir := t.TempDir()
			learningNotesDir := t.TempDir()

			notebookDir := filepath.Join(storiesDir, "test-notebook")
			require.NoError(t, os.MkdirAll(notebookDir, 0755))

			index := notebook.Index{
				Kind:          "story",
				ID:            "test-notebook",
				Name:          "Test Notebook",
				NotebookPaths: []string{"stories.yml"},
			}
			indexPath := filepath.Join(notebookDir, "index.yml")
			require.NoError(t, notebook.WriteYamlFile(indexPath, index))

			conversationQuote := fmt.Sprintf("I am so %s about this!", tt.noteExpression)
			stories := []notebook.StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []notebook.StoryScene{
						{
							Title: "Scene 1",
							Conversations: []notebook.Conversation{
								{Speaker: "A", Quote: conversationQuote},
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
			}
			storiesPath := filepath.Join(notebookDir, "stories.yml")
			require.NoError(t, notebook.WriteYamlFile(storiesPath, stories))

			todayStr := time.Now().Format("2006-01-02")
			yamlContent := `- metadata:
    id: test-notebook
    title: Story 1
  scenes:
    - metadata:
        title: Scene 1
      expressions:
        - expression: ` + tt.histExpression + `
          learned_logs: []
          reverse_logs:
            - status: usable
              learned_at: ` + todayStr + `
              interval_days: 7
          reverse_easiness_factor: 2.5
`
			learningHistoryPath := filepath.Join(learningNotesDir, "test-notebook.yml")
			require.NoError(t, os.WriteFile(learningHistoryPath, []byte(yamlContent), 0644))

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := mock_inference.NewMockClient(ctrl)

			cli, err := NewReverseQuizCLI(
				"test-notebook",
				config.NotebooksConfig{
					StoriesDirectories:     []string{storiesDir},
					LearningNotesDirectory: learningNotesDir,
				},
				t.TempDir(),
				mockClient,
				false,
				config.QuizConfig{},
			)
			require.NoError(t, err)

			assert.Equal(t, tt.wantCardCount, cli.GetCardCount())
		})
	}
}

func TestReverseQuizCLI_FullFlow(t *testing.T) {
	storiesDir := t.TempDir()
	learningNotesDir := t.TempDir()

	notebookDir := filepath.Join(storiesDir, "test-notebook")
	require.NoError(t, os.MkdirAll(notebookDir, 0755))

	index := notebook.Index{
		Kind:          "story",
		ID:            "test-notebook",
		Name:          "Test Notebook",
		NotebookPaths: []string{"stories.yml"},
	}
	indexPath := filepath.Join(notebookDir, "index.yml")
	require.NoError(t, notebook.WriteYamlFile(indexPath, index))

	stories := []notebook.StoryNotebook{
		{
			Event: "Story 1",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Scenes: []notebook.StoryScene{
				{
					Title: "Scene 1",
					Conversations: []notebook.Conversation{
						{Speaker: "A", Quote: "I am excited about this!"},
					},
					Definitions: []notebook.Note{
						{Expression: "excited", Meaning: "feeling enthusiasm"},
					},
				},
			},
		},
	}
	storiesPath := filepath.Join(notebookDir, "stories.yml")
	require.NoError(t, notebook.WriteYamlFile(storiesPath, stories))

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
							Expression: "excited",
							LearnedLogs: []notebook.LearningRecord{
								{Status: notebook.LearnedStatusCanBeUsed, QuizType: string(notebook.QuizTypeFreeform)},
							},
						},
					},
				},
			},
		},
	}
	learningHistoryPath := filepath.Join(learningNotesDir, "test-notebook.yml")
	require.NoError(t, notebook.WriteYamlFile(learningHistoryPath, learningHistory))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	notebooksConfig := config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		LearningNotesDirectory: learningNotesDir,
	}

	cli1, err := NewReverseQuizCLI(
		"test-notebook",
		notebooksConfig,
		t.TempDir(),
		mockClient,
		false,
		config.QuizConfig{},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, cli1.GetCardCount(), "First quiz should have 1 card")

	card := cli1.cards[0]
	gradeResult := quiz.GradeResult{
		Correct: true,
		Reason:  "exact match",
		Quality: int(notebook.QualityCorrect),
	}
	err = cli1.svc.SaveReverseResult(context.Background(), card, gradeResult, 5000)
	require.NoError(t, err)

	writtenHistories, err := notebook.NewLearningHistories(learningNotesDir)
	require.NoError(t, err)
	writtenHistory := writtenHistories["test-notebook"]
	require.Len(t, writtenHistory, 1)
	require.Len(t, writtenHistory[0].Scenes, 1)
	require.Len(t, writtenHistory[0].Scenes[0].Expressions, 1)

	writtenExpr := writtenHistory[0].Scenes[0].Expressions[0]
	t.Logf("After update - Expression: %s", writtenExpr.Expression)
	t.Logf("After update - ReverseLogs count: %d", len(writtenExpr.ReverseLogs))
	if len(writtenExpr.ReverseLogs) > 0 {
		t.Logf("After update - ReverseLogs[0]: %+v", writtenExpr.ReverseLogs[0])
		t.Logf("After update - NeedsReverseReview: %v", writtenExpr.NeedsReverseReview())
	}

	cli2, err := NewReverseQuizCLI(
		"test-notebook",
		notebooksConfig,
		t.TempDir(),
		mockClient,
		false,
		config.QuizConfig{},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, cli2.GetCardCount(), "Second quiz should have 0 cards - word was answered today")
}


func TestReverseQuizCLI_DisplayResult(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	tests := []struct {
		name            string
		card            *quiz.ReverseCard
		userAnswer      string
		grade           quiz.GradeResult
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "correct answer",
			card: &quiz.ReverseCard{
				Expression: "excited",
				Meaning:    "feeling enthusiasm",
				Contexts:   []quiz.ReverseContext{},
			},
			userAnswer: "excited",
			grade: quiz.GradeResult{
				Correct: true,
				Reason:  "exact match",
			},
			wantContains: []string{
				"Correct!",
				"excited",
				"feeling enthusiasm",
				"Reason: exact match",
			},
		},
		{
			name: "incorrect answer",
			card: &quiz.ReverseCard{
				Expression: "excited",
				Meaning:    "feeling enthusiasm",
				Contexts:   []quiz.ReverseContext{},
			},
			userAnswer: "apple",
			grade: quiz.GradeResult{
				Correct: false,
				Reason:  "unrelated word",
			},
			wantContains: []string{
				"Incorrect",
				"excited",
				"feeling enthusiasm",
				"You answered: apple",
				"Reason: unrelated word",
			},
		},
		{
			name: "correct answer with context",
			card: &quiz.ReverseCard{
				Expression: "excited",
				Meaning:    "feeling enthusiasm",
				Contexts: []quiz.ReverseContext{
					{Context: "I am so excited about the trip!", MaskedContext: "I am so ______ about the trip!"},
				},
			},
			userAnswer: "excited",
			grade: quiz.GradeResult{
				Correct: true,
			},
			wantContains: []string{
				"Correct!",
				"Context:",
				"I am so excited about the trip!",
			},
		},
		{
			name: "correct answer without reason",
			card: &quiz.ReverseCard{
				Expression: "hello",
				Meaning:    "a greeting",
				Contexts:   []quiz.ReverseContext{},
			},
			userAnswer: "hello",
			grade: quiz.GradeResult{
				Correct: true,
			},
			wantNotContains: []string{"Reason:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &ReverseQuizCLI{
				InteractiveQuizCLI: &InteractiveQuizCLI{
					bold:   color.New(color.Bold),
					italic: color.New(color.Italic),
				},
			}

			old := os.Stdout
			oldColorOutput := color.Output
			r, w, _ := os.Pipe()
			os.Stdout = w
			color.Output = w

			cli.displayReverseResult(tt.card, tt.userAnswer, tt.grade)

			w.Close()
			os.Stdout = old
			color.Output = oldColorOutput

			var buf strings.Builder
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			for _, want := range tt.wantContains {
				assert.Contains(t, output, want)
			}
			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, output, notWant)
			}
		})
	}
}

func TestNewReverseQuizCLI_AllNotebooks(t *testing.T) {
	storiesDir := t.TempDir()
	flashcardsDir := t.TempDir()
	learningNotesDir := t.TempDir()
	dictionaryCacheDir := t.TempDir()

	// Create story notebook manually with an old "understood" status so it passes spaced repetition
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
					Conversations: []notebook.Conversation{{Speaker: "A", Quote: "The eager student arrived early."}},
					Definitions:   []notebook.Note{{Expression: "eager", Meaning: "wanting to do something very much"}},
				},
			},
		},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, "test-story.yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: "test-story", Title: "Episode 1"},
			Scenes: []notebook.LearningScene{
				{
					Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
					Expressions: []notebook.LearningHistoryExpression{
						{
							Expression:  "eager",
							LearnedLogs: []notebook.LearningRecord{{Status: notebook.LearnedStatusCanBeUsed, LearnedAt: notebook.NewDate(time.Now().Add(-30 * 24 * time.Hour)), QuizType: string(notebook.QuizTypeFreeform)}},
						},
					},
				},
			},
		},
	}))

	// Create flashcard notebook manually with an old "understood" status
	fcDir := filepath.Join(flashcardsDir, "test-fc")
	require.NoError(t, os.MkdirAll(fcDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(fcDir, "index.yml"), notebook.FlashcardIndex{
		ID: "test-fc", Name: "Test Flashcards",
		NotebookPaths: []string{"cards.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(fcDir, "cards.yml"), []notebook.FlashcardNotebook{
		{
			Title: "Common Words",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Cards: []notebook.Note{{Expression: "break the ice", Meaning: "to initiate social interaction"}},
		},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, "test-fc.yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: "test-fc", Title: "Common Words", Type: "flashcard"},
			Expressions: []notebook.LearningHistoryExpression{
				{
					Expression:  "break the ice",
					LearnedLogs: []notebook.LearningRecord{{Status: notebook.LearnedStatusCanBeUsed, LearnedAt: notebook.NewDate(time.Now().Add(-30 * 24 * time.Hour)), QuizType: string(notebook.QuizTypeFreeform)}},
				},
			},
		},
	}))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	cli, err := NewReverseQuizCLI(
		"",
		config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			FlashcardsDirectories:  []string{flashcardsDir},
			LearningNotesDirectory: learningNotesDir,
		},
		dictionaryCacheDir,
		mockClient,
		false,
		config.QuizConfig{},
	)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, cli.GetCardCount(), 1)
}

func TestNewReverseQuizCLI_FlashcardNotebook(t *testing.T) {
	flashcardsDir := t.TempDir()
	learningNotesDir := t.TempDir()
	dictionaryCacheDir := t.TempDir()

	fcDir := filepath.Join(flashcardsDir, "my-fc")
	require.NoError(t, os.MkdirAll(fcDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(fcDir, "index.yml"), notebook.FlashcardIndex{
		ID: "my-fc", Name: "Test Flashcards",
		NotebookPaths: []string{"cards.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(fcDir, "cards.yml"), []notebook.FlashcardNotebook{
		{
			Title: "Common Words",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Cards: []notebook.Note{{Expression: "break the ice", Meaning: "to initiate social interaction"}},
		},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, "my-fc.yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: "my-fc", Title: "Common Words", Type: "flashcard"},
			Expressions: []notebook.LearningHistoryExpression{
				{
					Expression:  "break the ice",
					LearnedLogs: []notebook.LearningRecord{{Status: notebook.LearnedStatusCanBeUsed, LearnedAt: notebook.NewDate(time.Now().Add(-30 * 24 * time.Hour)), QuizType: string(notebook.QuizTypeFreeform)}},
				},
			},
		},
	}))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	cli, err := NewReverseQuizCLI(
		"my-fc",
		config.NotebooksConfig{
			FlashcardsDirectories:  []string{flashcardsDir},
			LearningNotesDirectory: learningNotesDir,
		},
		dictionaryCacheDir,
		mockClient,
		false,
		config.QuizConfig{},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, cli.GetCardCount())
}

func TestNewReverseQuizCLI_NotebookNotFound(t *testing.T) {
	storiesDir := t.TempDir()
	learningNotesDir := t.TempDir()
	dictionaryCacheDir := t.TempDir()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	_, err := NewReverseQuizCLI(
		"nonexistent",
		config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			LearningNotesDirectory: learningNotesDir,
		},
		dictionaryCacheDir,
		mockClient,
		false,
		config.QuizConfig{},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestNewReverseQuizCLI_NoLearningNote(t *testing.T) {
	storiesDir := t.TempDir()
	learningNotesDir := t.TempDir()
	dictionaryCacheDir := t.TempDir()

	notebookDir := filepath.Join(storiesDir, "test-story")
	require.NoError(t, os.MkdirAll(notebookDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(notebookDir, "index.yml"), notebook.Index{
		Kind: "story", ID: "test-story", Name: "Test Story",
		NotebookPaths: []string{"stories.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(notebookDir, "stories.yml"), []notebook.StoryNotebook{
		{
			Event: "Story 1",
			Scenes: []notebook.StoryScene{
				{
					Title: "Scene 1",
					Conversations: []notebook.Conversation{
						{Speaker: "A", Quote: "She is eager to learn."},
					},
					Definitions: []notebook.Note{{Expression: "eager", Meaning: "wanting to do something"}},
				},
			},
		},
	}))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	// No learning history file exists, so the word has no correct answers and won't appear
	cli, err := NewReverseQuizCLI(
		"test-story",
		config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			LearningNotesDirectory: learningNotesDir,
		},
		dictionaryCacheDir,
		mockClient,
		false,
		config.QuizConfig{},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, cli.GetCardCount())
}

func TestNewReverseQuizCLI_ListMissingContext(t *testing.T) {
	flashcardsDir := t.TempDir()
	learningNotesDir := t.TempDir()
	dictionaryCacheDir := t.TempDir()

	fcDir := filepath.Join(flashcardsDir, "test-fc")
	require.NoError(t, os.MkdirAll(fcDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(fcDir, "index.yml"), notebook.FlashcardIndex{
		ID: "test-fc", Name: "Test FC",
		NotebookPaths: []string{"cards.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(fcDir, "cards.yml"), []notebook.FlashcardNotebook{
		{
			Title: "Unit 1",
			Cards: []notebook.Note{
				{Expression: "abstruse", Meaning: "difficult to understand"},
				{Expression: "break the ice", Meaning: "to initiate interaction", Examples: []string{"She told a joke to break the ice."}},
			},
		},
	}))

	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, "test-fc.yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: "test-fc", Title: "Unit 1", Type: "flashcard"},
			Expressions: []notebook.LearningHistoryExpression{
				{Expression: "abstruse", LearnedLogs: []notebook.LearningRecord{{Status: notebook.LearnedStatusCanBeUsed, LearnedAt: notebook.NewDate(time.Now().Add(-30 * 24 * time.Hour)), QuizType: string(notebook.QuizTypeFreeform)}}},
				{Expression: "break the ice", LearnedLogs: []notebook.LearningRecord{{Status: notebook.LearnedStatusCanBeUsed, LearnedAt: notebook.NewDate(time.Now().Add(-30 * 24 * time.Hour)), QuizType: string(notebook.QuizTypeFreeform)}}},
			},
		},
	}))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	cli, err := NewReverseQuizCLI(
		"test-fc",
		config.NotebooksConfig{
			FlashcardsDirectories:  []string{flashcardsDir},
			LearningNotesDirectory: learningNotesDir,
		},
		dictionaryCacheDir,
		mockClient,
		true,
		config.QuizConfig{},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, cli.GetCardCount())
	cli.ListMissingContext()
}

func TestReverseQuizCLI_ValidateAnswer_SynonymRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	firstCall := mockClient.EXPECT().ValidateWordForm(gomock.Any(), gomock.Any()).Return(inference.ValidateWordFormResponse{
		Classification: inference.ClassificationSynonym,
		Reason:         "similar meaning",
		Quality:        int(notebook.QualityCorrect),
	}, nil)

	mockClient.EXPECT().ValidateWordForm(gomock.Any(), gomock.Any()).Return(inference.ValidateWordFormResponse{
		Classification: inference.ClassificationSameWord,
		Reason:         "exact match",
		Quality:        int(notebook.QualityCorrectFast),
	}, nil).After(firstCall)

	retryInput := "correct-word\n"
	stdinReader := bufio.NewReader(strings.NewReader(retryInput))

	svc := quiz.NewService(config.NotebooksConfig{}, mockClient, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository("", nil), config.QuizConfig{})

	cli := &ReverseQuizCLI{
		InteractiveQuizCLI: &InteractiveQuizCLI{
			openaiClient: mockClient,
			stdinReader:  stdinReader,
			bold:         color.New(color.Bold),
			italic:       color.New(color.Italic),
		},
		svc: svc,
	}

	card := &quiz.ReverseCard{
		Expression: "correct-word",
		Meaning:    "the right word",
		Contexts: []quiz.ReverseContext{
			{Context: "This is the context.", MaskedContext: "This is the ______."},
		},
	}

	grade, err := cli.gradeWithSynonymRetry(context.Background(), card, "synonym-word", 5000, false)
	assert.NoError(t, err)
	assert.True(t, grade.Correct)
	assert.Equal(t, int(notebook.QualityCorrectSlow), grade.Quality)
	assert.Equal(t, "exact match", grade.Reason)
}
