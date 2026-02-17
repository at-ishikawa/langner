// Package testutil provides shared test helpers for creating config files and notebook fixtures.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/require"
)

// SetupTestConfig creates a minimal config file and all required directories for testing.
// Returns the path to the generated config file.
func SetupTestConfig(t *testing.T, tmpDir string) string {
	t.Helper()

	dirs := []string{
		"stories", "learning_notes", "flashcards",
		"dictionaries", "output_stories", "output_flashcards",
		"books", "definitions", "ebooks",
	}
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, d), 0755))
	}

	configContent := fmt.Sprintf(`notebooks:
  stories_directories:
    - %s
  learning_notes_directory: %s
  flashcards_directories:
    - %s
  books_directories:
    - %s
  definitions_directories:
    - %s
dictionaries:
  rapidapi:
    cache_directory: %s
outputs:
  story_directory: %s
  flashcard_directory: %s
books:
  repo_directory: %s
  repositories_file: %s
`,
		filepath.Join(tmpDir, "stories"),
		filepath.Join(tmpDir, "learning_notes"),
		filepath.Join(tmpDir, "flashcards"),
		filepath.Join(tmpDir, "books"),
		filepath.Join(tmpDir, "definitions"),
		filepath.Join(tmpDir, "dictionaries"),
		filepath.Join(tmpDir, "output_stories"),
		filepath.Join(tmpDir, "output_flashcards"),
		filepath.Join(tmpDir, "ebooks"),
		filepath.Join(tmpDir, "repos.yml"),
	)

	cfgPath := filepath.Join(tmpDir, "config.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(configContent), 0644))
	return cfgPath
}

// SetupTestConfigWithAPIKey creates a config file with a fake OpenAI API key for tests
// that require API key validation to pass.
func SetupTestConfigWithAPIKey(t *testing.T, tmpDir string) string {
	t.Helper()
	cfgPath := SetupTestConfig(t, tmpDir)

	content, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	content = append(content, []byte("openai:\n  api_key: fake-key-for-testing\n  model: gpt-4o-mini\n")...)
	require.NoError(t, os.WriteFile(cfgPath, content, 0644))
	return cfgPath
}

// StoryNotebookOption configures optional fields when creating a story notebook fixture.
type StoryNotebookOption func(*storyNotebookConfig)

type storyNotebookConfig struct {
	learningStatus notebook.LearnedStatus
}

// WithStoryLearningStatus sets the learning status for the story notebook's learning history.
func WithStoryLearningStatus(status notebook.LearnedStatus) StoryNotebookOption {
	return func(cfg *storyNotebookConfig) {
		cfg.learningStatus = status
	}
}

// CreateStoryNotebook creates a story notebook with index, story YAML, and learning history files.
// By default the learning history has a "misunderstood" status. Use WithStoryLearningStatus to override.
func CreateStoryNotebook(t *testing.T, storiesDir, learningNotesDir, id string, opts ...StoryNotebookOption) {
	t.Helper()

	cfg := storyNotebookConfig{
		learningStatus: notebook.LearnedStatusMisunderstood,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	storyDir := filepath.Join(storiesDir, id)
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(storyDir, "index.yml"), notebook.Index{
		Kind: "story", ID: id, Name: "Test Story",
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
						{Speaker: "A", Quote: "The {{ eager }} student arrived early."},
					},
					Definitions: []notebook.Note{
						{Expression: "eager", Meaning: "wanting to do something very much"},
					},
				},
			},
		},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, id+".yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: id, Title: "Episode 1"},
			Scenes: []notebook.LearningScene{
				{
					Metadata: notebook.LearningSceneMetadata{Title: "Scene 1"},
					Expressions: []notebook.LearningHistoryExpression{
						{
							Expression:  "eager",
							LearnedLogs: []notebook.LearningRecord{{Status: cfg.learningStatus, LearnedAt: notebook.NewDate(time.Now())}},
						},
					},
				},
			},
		},
	}))
}

// FlashcardNotebookOption configures optional fields when creating a flashcard notebook fixture.
type FlashcardNotebookOption func(*flashcardNotebookConfig)

type flashcardNotebookConfig struct {
	learningStatus notebook.LearnedStatus
}

// WithFlashcardLearningStatus sets the learning status for the flashcard notebook's learning history.
func WithFlashcardLearningStatus(status notebook.LearnedStatus) FlashcardNotebookOption {
	return func(cfg *flashcardNotebookConfig) {
		cfg.learningStatus = status
	}
}

// CreateFlashcardNotebook creates a flashcard notebook with index, cards YAML, and learning history files.
// By default the learning history has a "misunderstood" status. Use WithFlashcardLearningStatus to override.
func CreateFlashcardNotebook(t *testing.T, flashcardsDir, learningNotesDir, id string, opts ...FlashcardNotebookOption) {
	t.Helper()

	cfg := flashcardNotebookConfig{
		learningStatus: notebook.LearnedStatusMisunderstood,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	fcDir := filepath.Join(flashcardsDir, id)
	require.NoError(t, os.MkdirAll(fcDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(fcDir, "index.yml"), notebook.FlashcardIndex{
		ID: id, Name: "Test Flashcards",
		NotebookPaths: []string{"cards.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(fcDir, "cards.yml"), []notebook.FlashcardNotebook{
		{
			Title: "Common Words",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Cards: []notebook.Note{
				{Expression: "break the ice", Meaning: "to initiate social interaction"},
			},
		},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(learningNotesDir, id+".yml"), []notebook.LearningHistory{
		{
			Metadata: notebook.LearningHistoryMetadata{NotebookID: id, Title: "Common Words", Type: "flashcard"},
			Expressions: []notebook.LearningHistoryExpression{
				{
					Expression:  "break the ice",
					LearnedLogs: []notebook.LearningRecord{{Status: cfg.learningStatus, LearnedAt: notebook.NewDate(time.Now())}},
				},
			},
		},
	}))
}
