package cli

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
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

func TestMaskWordInContext(t *testing.T) {
	tests := []struct {
		name       string
		context    string
		expression string
		definition string
		usage      string
		want       string
	}{
		{
			name:       "Simple word replacement",
			context:    "I am so excited about the trip!",
			expression: "excited",
			definition: "",
			usage:      "",
			want:       "I am so ______ about the trip!",
		},
		{
			name:       "Case insensitive replacement",
			context:    "The EXCITED child ran around.",
			expression: "excited",
			definition: "",
			usage:      "",
			want:       "The ______ child ran around.",
		},
		{
			name:       "Multiple occurrences",
			context:    "He was excited, and she was excited too.",
			expression: "excited",
			definition: "",
			usage:      "",
			want:       "He was ______, and she was ______ too.",
		},
		{
			name:       "With definition different from expression",
			context:    "Please take off your shoes.",
			expression: "remove",
			definition: "take off",
			usage:      "",
			want:       "Please ______ your shoes.",
		},
		{
			name:       "Remove markers and mask",
			context:    "I am so {{ excited }} about the trip!",
			expression: "excited",
			definition: "",
			usage:      "",
			want:       "I am so ______ about the trip!",
		},
		{
			name:       "Word boundary - don't mask partial matches",
			context:    "The excitement was palpable.",
			expression: "excite",
			definition: "",
			usage:      "",
			want:       "The excitement was palpable.",
		},
		{
			name:       "Word boundary - mask exact word",
			context:    "Don't excite the dog.",
			expression: "excite",
			definition: "",
			usage:      "",
			want:       "Don't ______ the dog.",
		},
		{
			name:       "Multi-word expression - exact match with expression",
			context:    "But you never give my friends the time of day.",
			expression: "give my friends the time of day",
			definition: "give someone the time of day",
			usage:      "",
			want:       "But you never ______.",
		},
		{
			name:       "Usage field helps mask actual form",
			context:    "She gave them the cold shoulder.",
			expression: "give the cold shoulder",
			definition: "",
			usage:      "gave them the cold shoulder",
			want:       "She ______.",
		},
		{
			name:       "Reflexive pronoun - expression matches context",
			context:    "Don't make a fool of yourself, Nicole.",
			expression: "make a fool of yourself",
			definition: "make a fool of oneself",
			usage:      "",
			want:       "Don't ______, Nicole.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskWordInContext(tt.context, tt.expression, tt.definition, tt.usage)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatReverseQuestion(t *testing.T) {
	tests := []struct {
		name     string
		card     *WordOccurrence
		expected string
	}{
		{
			name: "With context",
			card: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "excited",
					Meaning:    "feeling or showing enthusiasm",
				},
				Contexts: []WordOccurrenceContext{
					{Context: "I am so excited about the trip!", Usage: "excited"},
				},
			},
			expected: "Meaning: feeling or showing enthusiasm\nContext:\n  1. I am so ______ about the trip!\n\n",
		},
		{
			name: "Without context",
			card: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "cavalry",
					Meaning:    "soldiers who fight on horseback",
				},
				Contexts: []WordOccurrenceContext{},
			},
			expected: "Meaning: soldiers who fight on horseback\n\n",
		},
		{
			name: "Multiple contexts",
			card: &WordOccurrence{
				Definition: &notebook.Note{
					Expression: "thrilled",
					Meaning:    "extremely pleased and excited",
				},
				Contexts: []WordOccurrenceContext{
					{Context: "She was thrilled to hear the news.", Usage: "thrilled"},
					{Context: "We are thrilled about the results.", Usage: "thrilled"},
				},
			},
			expected: "Meaning: extremely pleased and excited\nContext:\n  1. She was ______ to hear the news.\n  2. We are ______ about the results.\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatReverseQuestion(tt.card)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestExtractReverseQuizCards(t *testing.T) {
	tests := []struct {
		name               string
		notebookName       string
		stories            []notebook.StoryNotebook
		learningHistory    []notebook.LearningHistory
		listMissingContext bool
		wantCount          int
		validate           func(t *testing.T, cards []*WordOccurrence)
	}{
		{
			name:         "All words need review (no history)",
			notebookName: "test-notebook",
			stories: []notebook.StoryNotebook{
				{
					Event: "Story 1",
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
			},
			learningHistory:    []notebook.LearningHistory{},
			listMissingContext: false,
			wantCount:          1,
		},
		{
			name:         "Word with recent reverse review skipped",
			notebookName: "test-notebook",
			stories: []notebook.StoryNotebook{
				{
					Event: "Story 1",
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
			},
			learningHistory: []notebook.LearningHistory{
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
			},
			listMissingContext: false,
			wantCount:          0, // Skipped because reviewed recently
		},
		{
			name:         "List missing context - exclude words with context",
			notebookName: "test-notebook",
			stories: []notebook.StoryNotebook{
				{
					Event: "Story 1",
					Scenes: []notebook.StoryScene{
						{
							Title: "Scene 1",
							Conversations: []notebook.Conversation{
								{Speaker: "A", Quote: "I am excited about this!"},
							},
							Definitions: []notebook.Note{
								{Expression: "excited", Meaning: "feeling enthusiasm"},
								{Expression: "lonely", Meaning: "feeling alone"}, // No conversation contains this
							},
						},
					},
				},
			},
			learningHistory:    []notebook.LearningHistory{},
			listMissingContext: true,
			wantCount:          1, // Only "lonely" which has no context
			validate: func(t *testing.T, cards []*WordOccurrence) {
				assert.Equal(t, "lonely", cards[0].Definition.Expression)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := extractReverseQuizCards(tt.notebookName, tt.stories, tt.learningHistory, tt.listMissingContext)
			assert.Equal(t, tt.wantCount, len(cards))
			if tt.validate != nil {
				tt.validate(t, cards)
			}
		})
	}
}

func TestReverseQuizCLI_CalculateQuality(t *testing.T) {
	cli := &ReverseQuizCLI{}

	tests := []struct {
		name           string
		responseTimeMs int64
		isRetry        bool
		want           int
	}{
		{
			name:           "Fast response - Q5",
			responseTimeMs: 2000,
			isRetry:        false,
			want:           int(notebook.QualityCorrectFast),
		},
		{
			name:           "Normal response - Q4",
			responseTimeMs: 5000,
			isRetry:        false,
			want:           int(notebook.QualityCorrect),
		},
		{
			name:           "Slow response - Q3",
			responseTimeMs: 15000,
			isRetry:        false,
			want:           int(notebook.QualityCorrectSlow),
		},
		{
			name:           "Retry always Q3",
			responseTimeMs: 1000,
			isRetry:        true,
			want:           int(notebook.QualityCorrectSlow),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cli.calculateQuality(tt.responseTimeMs, tt.isRetry)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReverseQuizCLI_ValidateAnswer_ExactMatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_inference.NewMockClient(ctrl)

	cli := &ReverseQuizCLI{
		InteractiveQuizCLI: &InteractiveQuizCLI{
			openaiClient: mockClient,
		},
	}

	card := &WordOccurrence{
		Definition: &notebook.Note{
			Expression: "excited",
			Meaning:    "feeling enthusiasm",
		},
		Contexts: []WordOccurrenceContext{},
	}

	// Exact match should not call OpenAI
	isCorrect, quality, reason, err := cli.validateAnswer(context.Background(), card, "excited", 2000, false)
	require.NoError(t, err)
	assert.True(t, isCorrect)
	assert.Equal(t, int(notebook.QualityCorrectFast), quality)
	assert.Equal(t, "exact match", reason)
}

func TestReverseQuizCLI_ValidateAnswer_CaseInsensitive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_inference.NewMockClient(ctrl)

	cli := &ReverseQuizCLI{
		InteractiveQuizCLI: &InteractiveQuizCLI{
			openaiClient: mockClient,
		},
	}

	card := &WordOccurrence{
		Definition: &notebook.Note{
			Expression: "excited",
			Meaning:    "feeling enthusiasm",
		},
		Contexts: []WordOccurrenceContext{},
	}

	// Case insensitive match should not call OpenAI
	isCorrect, quality, reason, err := cli.validateAnswer(context.Background(), card, "EXCITED", 2000, false)
	require.NoError(t, err)
	assert.True(t, isCorrect)
	assert.Equal(t, int(notebook.QualityCorrectFast), quality)
	assert.Equal(t, "exact match", reason)
}

func TestReverseQuizCLI_ValidateAnswer_DefinitionMatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_inference.NewMockClient(ctrl)

	cli := &ReverseQuizCLI{
		InteractiveQuizCLI: &InteractiveQuizCLI{
			openaiClient: mockClient,
		},
	}

	card := &WordOccurrence{
		Definition: &notebook.Note{
			Expression: "remove",
			Definition: "take off",
			Meaning:    "to remove something",
		},
		Contexts: []WordOccurrenceContext{},
	}

	// When Definition is set, GetExpression() returns Definition ("take off")
	// So typing "take off" is an exact match, not a definition match
	isCorrect, quality, reason, err := cli.validateAnswer(context.Background(), card, "take off", 2000, false)
	require.NoError(t, err)
	assert.True(t, isCorrect)
	assert.Equal(t, int(notebook.QualityCorrectFast), quality)
	assert.Equal(t, "exact match", reason)

	// Typing the Expression ("remove") should also work via the expression field check
	isCorrect, quality, reason, err = cli.validateAnswer(context.Background(), card, "remove", 2000, false)
	require.NoError(t, err)
	assert.True(t, isCorrect)
	assert.Equal(t, int(notebook.QualityCorrectFast), quality)
	assert.Equal(t, "matches expression", reason)
}

func TestReverseQuizCLI_ValidateAnswer_SameWord(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_inference.NewMockClient(ctrl)
	mockClient.EXPECT().
		ValidateWordForm(gomock.Any(), gomock.Any()).
		Return(inference.ValidateWordFormResponse{
			Classification: inference.ClassificationSameWord,
			Reason:         "different tense of the same word",
		}, nil)

	cli := &ReverseQuizCLI{
		InteractiveQuizCLI: &InteractiveQuizCLI{
			openaiClient: mockClient,
		},
	}

	card := &WordOccurrence{
		Definition: &notebook.Note{
			Expression: "run",
			Meaning:    "to move quickly on foot",
		},
		Contexts: []WordOccurrenceContext{},
	}

	// "ran" is a different form of "run"
	isCorrect, quality, reason, err := cli.validateAnswer(context.Background(), card, "ran", 2000, false)
	require.NoError(t, err)
	assert.True(t, isCorrect)
	assert.Equal(t, int(notebook.QualityCorrectFast), quality)
	assert.Equal(t, "different tense of the same word", reason)
}

func TestReverseQuizCLI_ValidateAnswer_Wrong(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_inference.NewMockClient(ctrl)
	mockClient.EXPECT().
		ValidateWordForm(gomock.Any(), gomock.Any()).
		Return(inference.ValidateWordFormResponse{
			Classification: inference.ClassificationWrong,
			Reason:         "unrelated word",
		}, nil)

	cli := &ReverseQuizCLI{
		InteractiveQuizCLI: &InteractiveQuizCLI{
			openaiClient: mockClient,
		},
	}

	card := &WordOccurrence{
		Definition: &notebook.Note{
			Expression: "excited",
			Meaning:    "feeling enthusiasm",
		},
		Contexts: []WordOccurrenceContext{},
	}

	isCorrect, quality, reason, err := cli.validateAnswer(context.Background(), card, "apple", 2000, false)
	require.NoError(t, err)
	assert.False(t, isCorrect)
	assert.Equal(t, int(notebook.QualityWrong), quality)
	assert.Equal(t, "unrelated word", reason)
}

func TestReverseQuizCLI_ValidateAnswer_EmptyAnswer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_inference.NewMockClient(ctrl)

	cli := &ReverseQuizCLI{
		InteractiveQuizCLI: &InteractiveQuizCLI{
			openaiClient: mockClient,
		},
	}

	card := &WordOccurrence{
		Definition: &notebook.Note{
			Expression: "excited",
			Meaning:    "feeling enthusiasm",
		},
		Contexts: []WordOccurrenceContext{},
	}

	// Empty answer should not call OpenAI
	isCorrect, quality, reason, err := cli.validateAnswer(context.Background(), card, "", 2000, false)
	require.NoError(t, err)
	assert.False(t, isCorrect)
	assert.Equal(t, int(notebook.QualityWrong), quality)
	assert.Equal(t, "empty answer", reason)
}

func TestReverseQuizCLI_Session(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		cards              []*WordOccurrence
		learningHistories  map[string][]notebook.LearningHistory
		mockOpenAIResponse inference.ValidateWordFormResponse
		mockOpenAIError    error
		wantErr            bool
		wantReturn         error
		wantCardsAfter     int
	}{
		{
			name:           "No more cards - returns errEnd",
			input:          "",
			cards:          []*WordOccurrence{},
			wantReturn:     errEnd,
			wantCardsAfter: 0,
		},
		{
			name:  "Correct exact match - updates history and removes card",
			input: "excited\n",
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
						Expression: "excited",
						Meaning:    "feeling enthusiasm",
					},
					Contexts: []WordOccurrenceContext{},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			wantCardsAfter:    0,
		},
		{
			name:  "Wrong answer - updates history and removes card",
			input: "apple\n",
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
						Expression: "excited",
						Meaning:    "feeling enthusiasm",
					},
					Contexts: []WordOccurrenceContext{},
				},
			},
			learningHistories: map[string][]notebook.LearningHistory{},
			mockOpenAIResponse: inference.ValidateWordFormResponse{
				Classification: inference.ClassificationWrong,
				Reason:         "unrelated word",
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

			// Set expectation only if we have cards and need OpenAI validation
			if len(tt.cards) > 0 {
				// Check if exact match - if not, OpenAI will be called
				userAnswer := strings.TrimSpace(tt.input)
				if len(tt.cards) > 0 && !strings.EqualFold(userAnswer, tt.cards[0].Definition.Expression) {
					mockClient.EXPECT().
						ValidateWordForm(gomock.Any(), gomock.Any()).
						Return(tt.mockOpenAIResponse, tt.mockOpenAIError).
						AnyTimes()
				}
			}

			cli := &ReverseQuizCLI{
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

				// Create learning history without reverse logs
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
											{Status: "usable", LearnedAt: notebook.NewDate(time.Now())},
										},
										// No ReverseLogs - should need review
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

				// Create learning history with recent reverse logs
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
				storiesDirs,
				flashcardsDirs,
				learningNotesDir,
				dictionaryCacheDir,
				mockClient,
				tt.listMissingContext,
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
	// This test verifies that scene titles with newlines are properly normalized
	// when comparing between notebook and learning history
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

	// Scene title with newlines (similar to real data)
	multiLineSceneTitle := "Ted always writes the songs.\nBut now Amber wants to write too.\nShe sings her new song.\n"

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

	// Create learning history with the SAME multi-line scene title
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

	// Read back the learning history to simulate what happens in real usage
	readBackHistories, err := notebook.NewLearningHistories(learningNotesDir)
	require.NoError(t, err)

	// Debug: print what was read back
	readBackHistory := readBackHistories["test-notebook"]
	t.Logf("Original scene title: %q", multiLineSceneTitle)
	t.Logf("Read back scene title: %q", readBackHistory[0].Scenes[0].Metadata.Title)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	cli, err := NewReverseQuizCLI(
		"test-notebook",
		[]string{storiesDir},
		nil,
		learningNotesDir,
		t.TempDir(),
		mockClient,
		false,
	)
	require.NoError(t, err)

	// The word should be skipped because it has recent reverse review
	assert.Equal(t, 0, cli.GetCardCount(), "Expected 0 cards because word has recent reverse review")
}

func TestNewReverseQuizCLI_YAMLRoundTrip(t *testing.T) {
	tests := []struct {
		name           string
		noteExpression string // Expression field in notebook
		noteDefinition string // Definition field in notebook (optional)
		histExpression string // expression field in learning history
		wantCardCount  int
	}{
		{
			name:           "Simple expression match",
			noteExpression: "excited",
			noteDefinition: "",
			histExpression: "excited",
			wantCardCount:  0, // Should be skipped
		},
		{
			name:           "Definition field - history stores Expression",
			noteExpression: "on a roll",
			noteDefinition: "be on a roll",
			histExpression: "on a roll", // Stored using Expression field
			wantCardCount:  0,           // Should match via Expression
		},
		{
			name:           "Definition field - history stores Definition",
			noteExpression: "on a roll",
			noteDefinition: "be on a roll",
			histExpression: "be on a roll", // Stored using Definition field
			wantCardCount:  0,              // Should match via Definition
		},
		{
			name:           "No match - different expression",
			noteExpression: "excited",
			noteDefinition: "",
			histExpression: "different_word",
			wantCardCount:  1, // Should NOT be skipped
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

			stories := []notebook.StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []notebook.StoryScene{
						{
							Title: "Scene 1",
							Conversations: []notebook.Conversation{
								{Speaker: "A", Quote: "Test conversation"},
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
				[]string{storiesDir},
				nil,
				learningNotesDir,
				t.TempDir(),
				mockClient,
				false,
			)
			require.NoError(t, err)

			assert.Equal(t, tt.wantCardCount, cli.GetCardCount())
		})
	}
}

func TestReverseQuizCLI_FullFlow(t *testing.T) {
	// This test simulates the complete production flow:
	// 1. Load quiz with a word that has no reverse_logs
	// 2. Answer the word correctly (which calls updateReverseHistory)
	// 3. Create a new quiz instance
	// 4. Verify the word is now skipped

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

	// Create empty learning history (no reverse_logs yet)
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
							Expression:  "excited",
							LearnedLogs: []notebook.LearningRecord{},
							// No ReverseLogs - word should be shown
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

	// Create first quiz - should have 1 card (no reverse_logs)
	cli1, err := NewReverseQuizCLI(
		"test-notebook",
		[]string{storiesDir},
		nil,
		learningNotesDir,
		t.TempDir(),
		mockClient,
		false,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, cli1.GetCardCount(), "First quiz should have 1 card")

	// Simulate answering correctly by directly calling updateReverseHistory
	card := cli1.cards[0]
	err = cli1.updateReverseHistory(card, true, int(notebook.QualityCorrect), 5000)
	require.NoError(t, err)

	// Debug: Check what was written
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

	// Create second quiz - should have 0 cards (word was answered today)
	cli2, err := NewReverseQuizCLI(
		"test-notebook",
		[]string{storiesDir},
		nil,
		learningNotesDir,
		t.TempDir(),
		mockClient,
		false,
	)
	require.NoError(t, err)
	assert.Equal(t, 0, cli2.GetCardCount(), "Second quiz should have 0 cards - word was answered today")
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Simple string",
			input: "Scene 1",
			want:  "Scene 1",
		},
		{
			name:  "String with newlines",
			input: "Line 1\nLine 2\nLine 3\n",
			want:  "Line 1 Line 2 Line 3",
		},
		{
			name:  "String with multiple spaces",
			input: "Line 1   Line 2",
			want:  "Line 1 Line 2",
		},
		{
			name:  "String with leading/trailing whitespace",
			input: "  Scene 1  ",
			want:  "Scene 1",
		},
		{
			name:  "Complex multi-line string",
			input: "Ted always writes the songs.\nBut now Amber wants to write too.\nShe sings her new song.\n",
			want:  "Ted always writes the songs. But now Amber wants to write too. She sings her new song.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTitle(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
