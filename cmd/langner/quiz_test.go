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
