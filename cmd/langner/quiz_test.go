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
	"github.com/at-ishikawa/langner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewQuizFreeformCommand_RunE_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

	cmd := newQuizFreeformCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration")
}

func TestNewQuizNotebookCommand_RunE_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

	cmd := newQuizNotebookCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration")
}

func TestNewQuizFreeformCommand_RunE_NoAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	cmd := newQuizFreeformCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

func TestNewQuizNotebookCommand_RunE_NoAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	cmd := newQuizNotebookCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

func TestRunRecognitionQuiz_NotebookNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

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

func TestRunReverseQuiz(t *testing.T) {
	tests := []struct {
		name                string
		setupNotebooks      bool
		notebookName        string
		listMissingContext  bool
		learningStatus      string
		wantErr             bool
		wantErrContains     string
	}{
		{
			name:               "list missing context",
			listMissingContext: true,
		},
		{
			name: "no cards need review",
		},
		{
			name:           "specific notebook with no cards",
			setupNotebooks: true,
			notebookName:   "test-story",
		},
		{
			name:           "with cards needing reverse review",
			setupNotebooks: true,
			notebookName:   "test-story",
			learningStatus: "understood",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfgPath := testutil.SetupTestConfig(t, tmpDir)
			setConfigFile(t, cfgPath)

			if tt.setupNotebooks {
				var opts []testutil.StoryNotebookOption
				if tt.learningStatus != "" {
					opts = append(opts, testutil.WithStoryLearningStatus(notebook.LearnedStatus(tt.learningStatus)))
				}
				testutil.CreateStoryNotebook(t, filepath.Join(tmpDir, "stories"), filepath.Join(tmpDir, "learning_notes"), "test-story", opts...)
			}

			loader, err := config.NewConfigLoader(cfgPath)
			require.NoError(t, err)
			cfg, err := loader.Load()
			require.NoError(t, err)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := mock_inference.NewMockClient(ctrl)

			err = runReverseQuiz(cfg, mockClient, tt.notebookName, tt.listMissingContext)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrContains != "" {
					assert.Contains(t, err.Error(), tt.wantErrContains)
				}
				return
			}
			// Some tests may get EOF from stdin reading in test environment
			if err != nil {
				assert.Contains(t, err.Error(), "EOF")
			}
		})
	}
}

func TestRunRecognitionQuiz_AllNotebooks(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	testutil.CreateStoryNotebook(t, filepath.Join(tmpDir, "stories"), filepath.Join(tmpDir, "learning_notes"), "test-story")

	loader, err := config.NewConfigLoader(cfgPath)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_inference.NewMockClient(ctrl)

	err = runRecognitionQuiz(cfg, mockClient, "", false)
	assert.NoError(t, err)
}

func TestRunRecognitionQuiz_SpecificStoryNotebook(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	testutil.CreateStoryNotebook(t, filepath.Join(tmpDir, "stories"), filepath.Join(tmpDir, "learning_notes"), "test-story")

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
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	testutil.CreateFlashcardNotebook(t, filepath.Join(tmpDir, "flashcards"), filepath.Join(tmpDir, "learning_notes"), "test-fc",
		testutil.WithFlashcardLearningStatus("usable"))

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

func TestNewQuizNotebookCommand_RunE_WithAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfigWithAPIKey(t, tmpDir)
	setConfigFile(t, cfgPath)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "reverse list missing context",
			args: []string{"--mode", "reverse", "--list-missing-context"},
		},
		{
			name: "reverse no cards",
			args: []string{"--mode", "reverse"},
		},
		{
			name:    "notebook not found",
			args:    []string{"--notebook", "nonexistent"},
			wantErr: "not found",
		},
		{
			name: "recognition all notebooks",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newQuizNotebookCommand()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestNewQuizFreeformCommand_RunE_WithAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfigWithAPIKey(t, tmpDir)
	setConfigFile(t, cfgPath)

	cmd := newQuizFreeformCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	// The command reaches Run() which reads from stdin -> EOF in test environment
	if err != nil {
		assert.Contains(t, err.Error(), "EOF")
	}
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
