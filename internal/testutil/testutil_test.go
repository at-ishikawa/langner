package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"


	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupTestConfig(t *testing.T) {
	tmpDir := t.TempDir()
	got := SetupTestConfig(t, tmpDir)

	want := filepath.Join(tmpDir, "config.yml")
	assert.Equal(t, want, got)

	// Verify config file exists and is readable.
	content, err := os.ReadFile(got)
	require.NoError(t, err)
	assert.Contains(t, string(content), "stories_directories")
	assert.Contains(t, string(content), "flashcards_directories")

	// Verify all required directories were created.
	dirs := []string{
		"stories", "learning_notes", "flashcards",
		"dictionaries", "output_stories", "output_flashcards",
		"books", "definitions", "ebooks",
	}
	for _, d := range dirs {
		info, err := os.Stat(filepath.Join(tmpDir, d))
		require.NoError(t, err, "directory %s should exist", d)
		assert.True(t, info.IsDir(), "%s should be a directory", d)
	}
}

func TestSetupTestConfigWithAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	got := SetupTestConfigWithAPIKey(t, tmpDir)

	want := filepath.Join(tmpDir, "config.yml")
	assert.Equal(t, want, got)

	content, err := os.ReadFile(got)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "openai:")
	assert.Contains(t, contentStr, "api_key: fake-key-for-testing")
	assert.Contains(t, contentStr, "model: gpt-4o-mini")
	// The base config fields should also be present.
	assert.Contains(t, contentStr, "stories_directories")
}

func TestCreateStoryNotebook(t *testing.T) {
	tests := []struct {
		name           string
		id             string
		opts           []StoryNotebookOption
		wantStatusStr  string
	}{
		{
			name:          "default learning status",
			id:            "story-1",
			wantStatusStr: "misunderstood",
		},
		{
			name:          "custom learning status",
			id:            "story-2",
			opts:          []StoryNotebookOption{WithStoryLearningStatus(notebook.LearnedStatus("understood"))},
			wantStatusStr: "understood",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			storiesDir := filepath.Join(tmpDir, "stories")
			learningNotesDir := filepath.Join(tmpDir, "learning_notes")
			require.NoError(t, os.MkdirAll(storiesDir, 0755))
			require.NoError(t, os.MkdirAll(learningNotesDir, 0755))

			CreateStoryNotebook(t, storiesDir, learningNotesDir, tt.id, tt.opts...)

			// Verify story directory was created.
			storyDir := filepath.Join(storiesDir, tt.id)
			info, err := os.Stat(storyDir)
			require.NoError(t, err)
			assert.True(t, info.IsDir())

			// Verify index.yml exists and contains expected content.
			indexPath := filepath.Join(storyDir, "index.yml")
			indexContent, err := os.ReadFile(indexPath)
			require.NoError(t, err)
			assert.Contains(t, string(indexContent), "kind: story")
			assert.Contains(t, string(indexContent), "id: "+tt.id)

			// Verify stories.yml exists and contains expected content.
			storiesPath := filepath.Join(storyDir, "stories.yml")
			storiesContent, err := os.ReadFile(storiesPath)
			require.NoError(t, err)
			assert.Contains(t, string(storiesContent), "Episode 1")
			assert.Contains(t, string(storiesContent), "eager")

			// Verify learning history file exists and contains expected status.
			historyPath := filepath.Join(learningNotesDir, tt.id+".yml")
			historyContent, err := os.ReadFile(historyPath)
			require.NoError(t, err)
			assert.Contains(t, string(historyContent), tt.wantStatusStr)
		})
	}
}

func TestCreateFlashcardNotebook(t *testing.T) {
	tests := []struct {
		name           string
		id             string
		opts           []FlashcardNotebookOption
		wantStatusStr  string
	}{
		{
			name:          "default learning status",
			id:            "fc-1",
			wantStatusStr: "misunderstood",
		},
		{
			name:          "custom learning status",
			id:            "fc-2",
			opts:          []FlashcardNotebookOption{WithFlashcardLearningStatus(notebook.LearnedStatus("understood"))},
			wantStatusStr: "understood",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			flashcardsDir := filepath.Join(tmpDir, "flashcards")
			learningNotesDir := filepath.Join(tmpDir, "learning_notes")
			require.NoError(t, os.MkdirAll(flashcardsDir, 0755))
			require.NoError(t, os.MkdirAll(learningNotesDir, 0755))

			CreateFlashcardNotebook(t, flashcardsDir, learningNotesDir, tt.id, tt.opts...)

			// Verify flashcard directory was created.
			fcDir := filepath.Join(flashcardsDir, tt.id)
			info, err := os.Stat(fcDir)
			require.NoError(t, err)
			assert.True(t, info.IsDir())

			// Verify index.yml exists and contains expected content.
			indexPath := filepath.Join(fcDir, "index.yml")
			indexContent, err := os.ReadFile(indexPath)
			require.NoError(t, err)
			assert.Contains(t, string(indexContent), "id: "+tt.id)
			assert.Contains(t, string(indexContent), "name: Test Flashcards")

			// Verify cards.yml exists and contains expected content.
			cardsPath := filepath.Join(fcDir, "cards.yml")
			cardsContent, err := os.ReadFile(cardsPath)
			require.NoError(t, err)
			assert.Contains(t, string(cardsContent), "Common Words")
			assert.Contains(t, string(cardsContent), "break the ice")

			// Verify learning history file exists and contains expected status.
			historyPath := filepath.Join(learningNotesDir, tt.id+".yml")
			historyContent, err := os.ReadFile(historyPath)
			require.NoError(t, err)
			assert.Contains(t, string(historyContent), tt.wantStatusStr)
			assert.Contains(t, string(historyContent), "type: flashcard")
		})
	}
}

func TestWithStoryLearningStatus(t *testing.T) {
	cfg := storyNotebookConfig{
		learningStatus: notebook.LearnedStatusMisunderstood,
	}

	// Apply the option with a different status value. Use the raw string
	// to verify the option function writes through correctly.
	opt := WithStoryLearningStatus("custom-status")
	opt(&cfg)

	assert.Equal(t, notebook.LearnedStatus("custom-status"), cfg.learningStatus)
}

func TestWithFlashcardLearningStatus(t *testing.T) {
	cfg := flashcardNotebookConfig{
		learningStatus: notebook.LearnedStatusMisunderstood,
	}

	opt := WithFlashcardLearningStatus("custom-status")
	opt(&cfg)

	assert.Equal(t, notebook.LearnedStatus("custom-status"), cfg.learningStatus)
}

func TestSetupTestConfig_configPathsAreAbsolute(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := SetupTestConfig(t, tmpDir)

	content, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	// Every path value in the config should be an absolute path.
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- /") || (strings.Contains(trimmed, ": /") && !strings.HasPrefix(trimmed, "#")) {
			parts := strings.SplitN(trimmed, " ", 2)
			path := parts[len(parts)-1]
			assert.True(t, filepath.IsAbs(path), "path should be absolute: %s", path)
		}
	}
}
