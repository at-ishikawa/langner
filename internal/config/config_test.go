package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigLoader(t *testing.T) {
	testCases := []struct {
		name       string
		configFile string
	}{
		{
			name:       "empty config file",
			configFile: "",
		},
		{
			name:       "config file",
			configFile: "testdata/config.yaml",
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := NewConfigLoader(tt.configFile)
			require.NoError(t, gotErr)
			assert.NotNil(t, got)
		})
	}
}

func TestConfigLoader_Load(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		want          *Config
		wantErr       string
	}{
		{
			name:          "empty config uses defaults",
			configContent: "# Empty config\n",
			want: &Config{
				Notebooks: NotebooksConfig{
					StoriesDirectories:     []string{filepath.Join("notebooks", "stories")},
					LearningNotesDirectory: filepath.Join("notebooks", "learning_notes"),
					FlashcardsDirectories:  []string{filepath.Join("notebooks", "flashcards")},
				},
				Dictionaries: DictionariesConfig{
					RapidAPI: RapidAPIConfig{
						CacheDirectory: filepath.Join("dictionaries", "rapidapi"),
					},
				},
				Outputs: OutputsConfig{
					StoryDirectory:     filepath.Join("outputs", "story"),
					FlashcardDirectory: filepath.Join("outputs", "flashcard"),
				},
				OpenAI: OpenAIConfig{
					Model: "gpt-4o-mini",
				},
			},
		},
		{
			name: "loads custom values without template",
			configContent: `notebooks:
  stories_directories: [custom/stories]
  learning_notes_directory: custom/learning
dictionaries:
  rapidapi:
    cache_directory: custom/cache
outputs:
  story_directory: custom/outputs
`,
			want: &Config{
				Notebooks: NotebooksConfig{
					StoriesDirectories:     []string{"custom/stories"},
					LearningNotesDirectory: "custom/learning",
					FlashcardsDirectories:  []string{filepath.Join("notebooks", "flashcards")},
				},
				Dictionaries: DictionariesConfig{
					RapidAPI: RapidAPIConfig{
						CacheDirectory: "custom/cache",
					},
				},
				Outputs: OutputsConfig{
					StoryDirectory:     "custom/outputs",
					FlashcardDirectory: filepath.Join("outputs", "flashcard"),
				},
				OpenAI: OpenAIConfig{
					Model: "gpt-4o-mini",
				},
			},
		},
		{
			name: "partial config merges with defaults",
			configContent: `notebooks:
  stories_directories: [partial/stories]
`,
			want: &Config{
				Notebooks: NotebooksConfig{
					StoriesDirectories:     []string{"partial/stories"},
					LearningNotesDirectory: filepath.Join("notebooks", "learning_notes"),
					FlashcardsDirectories:  []string{filepath.Join("notebooks", "flashcards")},
				},
				Dictionaries: DictionariesConfig{
					RapidAPI: RapidAPIConfig{
						CacheDirectory: filepath.Join("dictionaries", "rapidapi"),
					},
				},
				Outputs: OutputsConfig{
					StoryDirectory:     filepath.Join("outputs", "story"),
					FlashcardDirectory: filepath.Join("outputs", "flashcard"),
				},
				OpenAI: OpenAIConfig{
					Model: "gpt-4o-mini",
				},
			},
		},
		{
			name: "invalid YAML",
			configContent: `notebooks:
  stories_directories: [test]
  invalid yaml: [[[
`,
			wantErr: "configuration file found but could not be read",
		},
		{
			name: "invalid template path",
			configContent: `templates:
  story_notebook_template: /non/existent/file.tmpl
`,
			wantErr: "templates.story_notebook_template must be an existing and readable file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test directory
			tempDir := t.TempDir()

			// Create config file
			configPath := filepath.Join(tempDir, "config.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(tt.configContent), 0644))

			// Load config with explicit path
			loader, err := NewConfigLoader(configPath)
			require.NoError(t, err)

			got, err := loader.Load()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
