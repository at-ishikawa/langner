package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/cli"
	"github.com/at-ishikawa/langner/internal/config"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewQuizCommand(t *testing.T) {
	cmd := newQuizCommand()

	assert.Equal(t, "quiz", cmd.Use)
	assert.Equal(t, "Quiz commands for testing vocabulary knowledge", cmd.Short)
	assert.True(t, cmd.HasSubCommands())
}

func TestNewQuizFreeformCommand(t *testing.T) {
	cmd := newQuizFreeformCommand()

	assert.Equal(t, "freeform", cmd.Use)
	assert.Equal(t, "Freeform quiz where you provide both word and meaning from memory", cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestNewQuizFreeformCommand_RunE_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newQuizFreeformCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration")
}

func TestNewQuizNotebookCommand_RunE_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newQuizNotebookCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration")
}

func TestNewQuizFreeformCommand_RunE_NoAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newQuizFreeformCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

func TestNewQuizNotebookCommand_RunE_NoAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newQuizNotebookCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

func TestRunRecognitionQuiz_NotebookNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	loader, err := config.NewConfigLoader(cfgPath)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	err = runRecognitionQuiz(cfg, mockClient, "nonexistent-notebook", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunReverseQuiz_ListMissingContext(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	loader, err := config.NewConfigLoader(cfgPath)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	// listMissingContext=true will list and return nil
	err = runReverseQuiz(cfg, mockClient, "", true)
	assert.NoError(t, err)
}

func TestRunReverseQuiz_NoCardsNeedReview(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	loader, err := config.NewConfigLoader(cfgPath)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	// With empty directories, no cards need review
	err = runReverseQuiz(cfg, mockClient, "", false)
	assert.NoError(t, err)
}

func TestRunReverseQuiz_SpecificNotebook_NoCards(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	// Create a story notebook
	storiesDir := filepath.Join(tmpDir, "stories", "test-story")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(storiesDir, "index.yml"), notebook.Index{
		Kind: "story", ID: "test-story", Name: "Test Story",
		NotebookPaths: []string{"stories.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(storiesDir, "stories.yml"), []notebook.StoryNotebook{
		{
			Event: "Episode 1",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Scenes: []notebook.StoryScene{
				{
					Title: "Scene 1",
					Conversations: []notebook.Conversation{
						{Speaker: "A", Quote: "The {{ eager }} student arrived early."},
					},
					Definitions: []notebook.Note{
						{Expression: "eager", Meaning: "wanting to do something very much"},
					},
				},
			},
		},
	}))

	// Create empty learning notes for this notebook
	learningNotesDir := filepath.Join(tmpDir, "learning_notes")
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, "test-story.yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{
				NotebookID: "test-story",
				Title:      "Episode 1",
			},
		},
	}))

	loader, err := config.NewConfigLoader(cfgPath)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	// With specific notebook and empty learning notes, no cards need review
	err = runReverseQuiz(cfg, mockClient, "test-story", false)
	assert.NoError(t, err)
}

func setupTestConfigFileWithAPIKey(t *testing.T, tmpDir string) string {
	t.Helper()
	cfgPath := setupTestConfigFile(t, tmpDir)

	// Read and append OpenAI config with a fake API key
	content, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	content = append(content, []byte("openai:\n  api_key: fake-key-for-testing\n  model: gpt-4o-mini\n")...)
	require.NoError(t, os.WriteFile(cfgPath, content, 0644))
	return cfgPath
}

func TestNewQuizNotebookCommand_RunE_ReverseListMissingContext(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFileWithAPIKey(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newQuizNotebookCommand()
	cmd.SetArgs([]string{"--mode", "reverse", "--list-missing-context"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewQuizNotebookCommand_RunE_ReverseNoCards(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFileWithAPIKey(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newQuizNotebookCommand()
	cmd.SetArgs([]string{"--mode", "reverse"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewQuizNotebookCommand_RunE_NotebookNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFileWithAPIKey(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newQuizNotebookCommand()
	cmd.SetArgs([]string{"--notebook", "nonexistent"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestNewQuizNotebookCommand(t *testing.T) {
	cmd := newQuizNotebookCommand()

	assert.Equal(t, "notebook", cmd.Use)
	assert.Equal(t, "Quiz from notebooks. Use --mode to select quiz mode", cmd.Short)
	assert.NotNil(t, cmd.RunE)
	assert.NotEmpty(t, cmd.Long)

	// Verify flags
	includeFlag := cmd.Flags().Lookup("include-no-correct-answers")
	assert.NotNil(t, includeFlag)
	assert.Equal(t, "false", includeFlag.DefValue)

	notebookFlag := cmd.Flags().Lookup("notebook")
	assert.NotNil(t, notebookFlag)
	assert.Equal(t, "", notebookFlag.DefValue)
	assert.Equal(t, "n", notebookFlag.Shorthand)

	modeFlag := cmd.Flags().Lookup("mode")
	assert.NotNil(t, modeFlag)
	assert.Equal(t, "recognition", modeFlag.DefValue)
	assert.Equal(t, "m", modeFlag.Shorthand)

	listMissingFlag := cmd.Flags().Lookup("list-missing-context")
	assert.NotNil(t, listMissingFlag)
	assert.Equal(t, "false", listMissingFlag.DefValue)
}

// createStoryNotebookForQuiz creates a story notebook with learning history in tmpDir
func createStoryNotebookForQuiz(t *testing.T, tmpDir string) {
	t.Helper()
	storiesDir := filepath.Join(tmpDir, "stories", "test-story")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(storiesDir, "index.yml"), notebook.Index{
		Kind: "story", ID: "test-story", Name: "Test Story",
		NotebookPaths: []string{"stories.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(storiesDir, "stories.yml"), []notebook.StoryNotebook{
		{
			Event: "Episode 1",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Scenes: []notebook.StoryScene{
				{
					Title: "Scene 1",
					Conversations: []notebook.Conversation{
						{Speaker: "A", Quote: "The {{ eager }} student arrived early."},
					},
					Definitions: []notebook.Note{
						{Expression: "eager", Meaning: "wanting to do something very much"},
					},
				},
			},
		},
	}))
	learningNotesDir := filepath.Join(tmpDir, "learning_notes")
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, "test-story.yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: "test-story", Title: "Episode 1"},
			Scenes: []notebook.LearningScene{
				{
					Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
					Expressions: []notebook.LearningHistoryExpression{
						{
							Expression:  "eager",
							LearnedLogs: []notebook.LearningRecord{{Status: "misunderstood", LearnedAt: notebook.NewDate(time.Now())}},
						},
					},
				},
			},
		},
	}))
}

// createFlashcardNotebookForQuiz creates a flashcard notebook with learning history in tmpDir
func createFlashcardNotebookForQuiz(t *testing.T, tmpDir string) {
	t.Helper()
	flashcardDir := filepath.Join(tmpDir, "flashcards", "test-fc")
	require.NoError(t, os.MkdirAll(flashcardDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "index.yml"), notebook.FlashcardIndex{
		ID: "test-fc", Name: "Test Flashcards",
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
	learningNotesDir := filepath.Join(tmpDir, "learning_notes")
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, "test-fc.yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: "test-fc", Title: "Common Words", Type: "flashcard"},
			Expressions: []notebook.LearningHistoryExpression{
				{
					Expression:  "break the ice",
					LearnedLogs: []notebook.LearningRecord{{Status: "misunderstood", LearnedAt: notebook.NewDate(time.Now())}},
				},
			},
		},
	}))
}

func TestRunRecognitionQuiz_AllNotebooks(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	createStoryNotebookForQuiz(t, tmpDir)

	loader, err := config.NewConfigLoader(cfgPath)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	// Empty notebook name = all notebooks; with learning history this should work
	err = runRecognitionQuiz(cfg, mockClient, "", false)
	assert.NoError(t, err)
}

func TestRunRecognitionQuiz_SpecificStoryNotebook(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	createStoryNotebookForQuiz(t, tmpDir)

	loader, err := config.NewConfigLoader(cfgPath)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	err = runRecognitionQuiz(cfg, mockClient, "test-story", false)
	assert.NoError(t, err)
}

func TestRunRecognitionQuiz_SpecificFlashcardNotebook(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	// Create flashcard with "usable" status so the word doesn't need learning (0 cards, quiz exits immediately)
	flashcardDir := filepath.Join(tmpDir, "flashcards", "test-fc")
	require.NoError(t, os.MkdirAll(flashcardDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "index.yml"), notebook.FlashcardIndex{
		ID: "test-fc", Name: "Test Flashcards",
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
	learningNotesDir := filepath.Join(tmpDir, "learning_notes")
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, "test-fc.yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: "test-fc", Title: "Common Words", Type: "flashcard"},
			Expressions: []notebook.LearningHistoryExpression{
				{
					Expression:  "break the ice",
					LearnedLogs: []notebook.LearningRecord{{Status: "usable", LearnedAt: notebook.NewDate(time.Now())}},
				},
			},
		},
	}))

	loader, err := config.NewConfigLoader(cfgPath)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	err = runRecognitionQuiz(cfg, mockClient, "test-fc", false)
	// Quiz starts but returns EOF when trying to read from stdin in test environment
	if err != nil {
		assert.Contains(t, err.Error(), "EOF")
	}
}

func TestRunReverseQuiz_WithCards_SpecificNotebookName(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	// Create a story notebook with a forward-correct word needing reverse review
	storiesDir := filepath.Join(tmpDir, "stories", "test-story")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(storiesDir, "index.yml"), notebook.Index{
		Kind: "story", ID: "test-story", Name: "Test Story",
		NotebookPaths: []string{"stories.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(storiesDir, "stories.yml"), []notebook.StoryNotebook{
		{
			Event: "Episode 1",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Scenes: []notebook.StoryScene{
				{
					Title: "Scene 1",
					Conversations: []notebook.Conversation{
						{Speaker: "A", Quote: "The {{ eager }} student arrived early."},
					},
					Definitions: []notebook.Note{
						{Expression: "eager", Meaning: "wanting to do something very much"},
					},
				},
			},
		},
	}))

	learningNotesDir := filepath.Join(tmpDir, "learning_notes")
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, "test-story.yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: "test-story", Title: "Episode 1"},
			Scenes: []notebook.LearningScene{
				{
					Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
					Expressions: []notebook.LearningHistoryExpression{
						{
							Expression:  "eager",
							LearnedLogs: []notebook.LearningRecord{{Status: "understood"}},
							// No ReverseLogs - needs reverse review
						},
					},
				},
			},
		},
	}))

	loader, err := config.NewConfigLoader(cfgPath)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	// This should succeed with cards and print "Starting reverse quiz session for notebook..."
	// The quiz Run returns EOF error when trying to read from stdin in test environment
	err = runReverseQuiz(cfg, mockClient, "test-story", false)
	if err != nil {
		assert.Contains(t, err.Error(), "EOF")
	}
}

func TestNewQuizFreeformCommand_RunE_WithAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFileWithAPIKey(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newQuizFreeformCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	// The command reaches Run() which reads from stdin → EOF in test environment
	if err != nil {
		assert.Contains(t, err.Error(), "EOF")
	}
}

func TestNewQuizNotebookCommand_RunE_RecognitionAllNotebooks(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFileWithAPIKey(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	// Default mode is recognition, no notebook specified → all notebooks
	cmd := newQuizNotebookCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	// With empty notebooks, should succeed with 0 cards
	assert.NoError(t, err)
}

func TestNewFreeformQuizCLI_LoadsAllDirectoryTypes(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T) config.NotebooksConfig
		wantErr       bool
		wantWordCount int
	}{
		{
			name:          "loads words from book with separate definitions directory",
			wantWordCount: 1,
			setupFunc: func(t *testing.T) config.NotebooksConfig {
				t.Helper()
				booksDir := t.TempDir()
				definitionsDir := t.TempDir()
				learningNotesDir := t.TempDir()

				// Create book directory structure: books/test-book/index.yml + chapter1.yml
				bookDir := filepath.Join(booksDir, "test-book")
				require.NoError(t, os.MkdirAll(bookDir, 0755))

				bookIndex := notebook.Index{
					ID:            "test-book",
					Name:          "Test Book",
					NotebookPaths: []string{"chapter1.yml"},
				}
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(bookDir, "index.yml"), bookIndex))

				chapters := []notebook.StoryNotebook{
					{
						Event: "Chapter 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title: "Opening",
								Conversations: []notebook.Conversation{
									{Speaker: "Narrator", Quote: "The {{ arduous }} journey had just begun."},
								},
								Definitions: []notebook.Note{},
							},
						},
					},
				}
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(bookDir, "chapter1.yml"), chapters))

				// Create definitions file: definitions/test-book.yml
				definitions := []notebook.Definitions{
					{
						Metadata: notebook.DefinitionsMetadata{
							Notebook: "chapter1.yml",
						},
						Scenes: []notebook.DefinitionsScene{
							{
								Metadata: notebook.DefinitionsSceneMetadata{
									Index: 0,
								},
								Expressions: []notebook.Note{
									{
										Expression: "arduous",
										Meaning:    "involving great effort; difficult and tiring",
									},
								},
							},
						},
					},
				}
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(definitionsDir, "test-book.yml"), definitions))

				return config.NotebooksConfig{
					BooksDirectories:       []string{booksDir},
					DefinitionsDirectories: []string{definitionsDir},
					LearningNotesDirectory: learningNotesDir,
				}
			},
		},
		{
			name:          "loads words from story directory",
			wantWordCount: 1,
			setupFunc: func(t *testing.T) config.NotebooksConfig {
				t.Helper()
				storiesDir := t.TempDir()
				learningNotesDir := t.TempDir()

				// Create story directory structure: stories/test-story/index.yml + stories.yml
				storyDir := filepath.Join(storiesDir, "test-story")
				require.NoError(t, os.MkdirAll(storyDir, 0755))

				storyIndex := notebook.Index{
					Kind:          "story",
					ID:            "test-story",
					Name:          "Test Story",
					NotebookPaths: []string{"stories.yml"},
				}
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(storyDir, "index.yml"), storyIndex))

				stories := []notebook.StoryNotebook{
					{
						Event: "Episode 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "That puzzle was really {{ tricky }}."},
								},
								Definitions: []notebook.Note{
									{
										Expression: "tricky",
										Meaning:    "difficult to deal with; requiring skill or caution",
									},
								},
							},
						},
					},
				}
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(storyDir, "stories.yml"), stories))

				return config.NotebooksConfig{
					StoriesDirectories:     []string{storiesDir},
					LearningNotesDirectory: learningNotesDir,
				}
			},
		},
		{
			name:          "loads words from flashcard directory",
			wantWordCount: 1,
			setupFunc: func(t *testing.T) config.NotebooksConfig {
				t.Helper()
				flashcardsDir := t.TempDir()
				learningNotesDir := t.TempDir()

				// Create flashcard directory structure: flashcards/test-flashcard/index.yml + cards.yml
				flashcardDir := filepath.Join(flashcardsDir, "test-flashcard")
				require.NoError(t, os.MkdirAll(flashcardDir, 0755))

				flashcardIndex := notebook.FlashcardIndex{
					ID:            "test-flashcard",
					Name:          "Test Flashcards",
					NotebookPaths: []string{"cards.yml"},
				}
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "index.yml"), flashcardIndex))

				cards := []notebook.FlashcardNotebook{
					{
						Title: "Common Words",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Cards: []notebook.Note{
							{
								Expression: "break the ice",
								Meaning:    "to initiate social interaction in an awkward situation",
								Examples:   []string{"She told a joke to break the ice at the party."},
							},
						},
					},
				}
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "cards.yml"), cards))

				return config.NotebooksConfig{
					FlashcardsDirectories:  []string{flashcardsDir},
					LearningNotesDirectory: learningNotesDir,
				}
			},
		},
		{
			name:          "loads words from all directory types combined",
			wantWordCount: 3,
			setupFunc: func(t *testing.T) config.NotebooksConfig {
				t.Helper()
				booksDir := t.TempDir()
				definitionsDir := t.TempDir()
				storiesDir := t.TempDir()
				flashcardsDir := t.TempDir()
				learningNotesDir := t.TempDir()

				// Book
				bookDir := filepath.Join(booksDir, "test-book")
				require.NoError(t, os.MkdirAll(bookDir, 0755))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(bookDir, "index.yml"), notebook.Index{
					ID:            "test-book",
					Name:          "Test Book",
					NotebookPaths: []string{"chapter1.yml"},
				}))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(bookDir, "chapter1.yml"), []notebook.StoryNotebook{
					{
						Event: "Chapter 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title: "Opening",
								Conversations: []notebook.Conversation{
									{Speaker: "Narrator", Quote: "The {{ arduous }} journey had just begun."},
								},
							},
						},
					},
				}))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(definitionsDir, "test-book.yml"), []notebook.Definitions{
					{
						Metadata: notebook.DefinitionsMetadata{Notebook: "chapter1.yml"},
						Scenes: []notebook.DefinitionsScene{
							{
								Metadata:    notebook.DefinitionsSceneMetadata{Index: 0},
								Expressions: []notebook.Note{{Expression: "arduous", Meaning: "involving great effort"}},
							},
						},
					},
				}))

				// Story
				storyDir := filepath.Join(storiesDir, "test-story")
				require.NoError(t, os.MkdirAll(storyDir, 0755))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(storyDir, "index.yml"), notebook.Index{
					Kind:          "story",
					ID:            "test-story",
					Name:          "Test Story",
					NotebookPaths: []string{"stories.yml"},
				}))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(storyDir, "stories.yml"), []notebook.StoryNotebook{
					{
						Event: "Episode 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []notebook.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []notebook.Conversation{
									{Speaker: "A", Quote: "That puzzle was really {{ tricky }}."},
								},
								Definitions: []notebook.Note{
									{Expression: "tricky", Meaning: "difficult to deal with"},
								},
							},
						},
					},
				}))

				// Flashcard
				flashcardDir := filepath.Join(flashcardsDir, "test-flashcard")
				require.NoError(t, os.MkdirAll(flashcardDir, 0755))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "index.yml"), notebook.FlashcardIndex{
					ID:            "test-flashcard",
					Name:          "Test Flashcards",
					NotebookPaths: []string{"cards.yml"},
				}))
				require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "cards.yml"), []notebook.FlashcardNotebook{
					{
						Title: "Common Words",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Cards: []notebook.Note{
							{
								Expression: "break the ice",
								Meaning:    "to initiate social interaction",
								Examples:   []string{"She told a joke to break the ice."},
							},
						},
					},
				}))

				return config.NotebooksConfig{
					BooksDirectories:       []string{booksDir},
					DefinitionsDirectories: []string{definitionsDir},
					StoriesDirectories:     []string{storiesDir},
					FlashcardsDirectories:  []string{flashcardsDir},
					LearningNotesDirectory: learningNotesDir,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notebooksConfig := tt.setupFunc(t)
			dictionaryCacheDir := t.TempDir()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := mock_inference.NewMockClient(ctrl)

			got, err := cli.NewFreeformQuizCLI(
				notebooksConfig,
				dictionaryCacheDir,
				mockClient,
			)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantWordCount, got.WordCount(), "loaded word count should match expected")
		})
	}
}
