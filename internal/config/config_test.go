package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name              string
		configFile        string
		configContent     string
		useExplicitPath   bool
		wantErr           bool
		want              *Config
		wantErrorContains []string
	}{
		{
			name: "valid config file with custom values",
			configContent: `notebooks:
  stories_directory: custom/stories
  learning_notes_directory: custom/learning_notes
dictionaries:
  rapidapi:
    cache_directory: custom/dictionaries
templates:
  markdown_directory: custom/templates
outputs:
  story_directory: custom/outputs
`,
			useExplicitPath: false,
			wantErr:         false,
			want: &Config{
				Notebooks: NotebooksConfig{
					StoriesDirectory:       "custom/stories",
					LearningNotesDirectory: "custom/learning_notes",
				},
				Dictionaries: DictionariesConfig{
					RapidAPI: RapidAPIConfig{
						CacheDirectory: "custom/dictionaries",
					},
				},
				Templates: TemplatesConfig{
					MarkdownDirectory: "custom/templates",
				},
				Outputs: OutputsConfig{
					StoryDirectory: "custom/outputs",
				},
				OpenAI: OpenAIConfig{
					APIKey: "",
					Model:  "gpt-4o-mini",
				},
			},
		},
		{
			name: "invalid YAML format",
			configContent: `notebooks:
  stories_directory: custom/stories
  invalid yaml format here [[[
`,
			useExplicitPath: false,
			wantErr:         true,
			wantErrorContains: []string{
				"configuration file found but could not be read",
				"Please check the file format and permissions",
			},
		},
		{
			name: "invalid config structure uses defaults",
			configContent: `wrong_key:
  some_value: test
`,
			useExplicitPath: false,
			wantErr:         false,
			want: &Config{
				Notebooks: NotebooksConfig{
					StoriesDirectory:       filepath.Join("notebooks", "stories"),
					LearningNotesDirectory: filepath.Join("notebooks", "learning_notes"),
				},
				Dictionaries: DictionariesConfig{
					RapidAPI: RapidAPIConfig{
						CacheDirectory: filepath.Join("dictionaries", "rapidapi"),
					},
				},
				Templates: TemplatesConfig{
					MarkdownDirectory: filepath.Join("assets", "templates"),
				},
				Outputs: OutputsConfig{
					StoryDirectory: filepath.Join("outputs", "story"),
				},
				OpenAI: OpenAIConfig{
					APIKey: "",
					Model:  "gpt-4o-mini",
				},
			},
		},
		{
			name: "partial config with missing fields uses defaults",
			configContent: `notebooks:
  stories_directory: custom/stories
`,
			useExplicitPath: false,
			wantErr:         false,
			want: &Config{
				Notebooks: NotebooksConfig{
					StoriesDirectory:       "custom/stories",
					LearningNotesDirectory: filepath.Join("notebooks", "learning_notes"),
				},
				Dictionaries: DictionariesConfig{
					RapidAPI: RapidAPIConfig{
						CacheDirectory: filepath.Join("dictionaries", "rapidapi"),
					},
				},
				Templates: TemplatesConfig{
					MarkdownDirectory: filepath.Join("assets", "templates"),
				},
				Outputs: OutputsConfig{
					StoryDirectory: filepath.Join("outputs", "story"),
				},
				OpenAI: OpenAIConfig{
					APIKey: "",
					Model:  "gpt-4o-mini",
				},
			},
		},
		{
			name: "explicit config file path",
			configContent: `notebooks:
  stories_directory: explicit/stories
  learning_notes_directory: explicit/learning_notes
dictionaries:
  rapidapi:
    cache_directory: explicit/cache
templates:
  markdown_directory: explicit/templates
outputs:
  story_directory: explicit/outputs
`,
			useExplicitPath: true,
			wantErr:         false,
			want: &Config{
				Notebooks: NotebooksConfig{
					StoriesDirectory:       "explicit/stories",
					LearningNotesDirectory: "explicit/learning_notes",
				},
				Dictionaries: DictionariesConfig{
					RapidAPI: RapidAPIConfig{
						CacheDirectory: "explicit/cache",
					},
				},
				Templates: TemplatesConfig{
					MarkdownDirectory: "explicit/templates",
				},
				Outputs: OutputsConfig{
					StoryDirectory: "explicit/outputs",
				},
				OpenAI: OpenAIConfig{
					APIKey: "",
					Model:  "gpt-4o-mini",
				},
			},
		},
		{
			name: "explicit config file with config.yml name",
			configContent: `notebooks:
  stories_directory: yml/stories
  learning_notes_directory: yml/learning_notes
dictionaries:
  rapidapi:
    cache_directory: yml/cache
templates:
  markdown_directory: yml/templates
outputs:
  story_directory: yml/outputs
`,
			useExplicitPath: true,
			wantErr:         false,
			want: &Config{
				Notebooks: NotebooksConfig{
					StoriesDirectory:       "yml/stories",
					LearningNotesDirectory: "yml/learning_notes",
				},
				Dictionaries: DictionariesConfig{
					RapidAPI: RapidAPIConfig{
						CacheDirectory: "yml/cache",
					},
				},
				Templates: TemplatesConfig{
					MarkdownDirectory: "yml/templates",
				},
				Outputs: OutputsConfig{
					StoryDirectory: "yml/outputs",
				},
				OpenAI: OpenAIConfig{
					APIKey: "",
					Model:  "gpt-4o-mini",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			var configPath string
			if tt.useExplicitPath {
				configPath = filepath.Join(tempDir, "config.yml")
				err := os.WriteFile(configPath, []byte(tt.configContent), 0644)
				require.NoError(t, err)
			} else {
				if tt.configContent != "" {
					configPath = filepath.Join(tempDir, "config.yaml")
					err := os.WriteFile(configPath, []byte(tt.configContent), 0644)
					require.NoError(t, err)
				}

				originalDir, err := os.Getwd()
				require.NoError(t, err)
				defer func() {
					err := os.Chdir(originalDir)
					require.NoError(t, err)
				}()

				err = os.Chdir(tempDir)
				require.NoError(t, err)
				configPath = ""
			}

			got, err := Load(configPath)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				for _, wantMsg := range tt.wantErrorContains {
					assert.Contains(t, err.Error(), wantMsg)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got)
		})
	}
}
