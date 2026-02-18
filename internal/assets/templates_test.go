package assets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFlashcardNotebook(t *testing.T) {
	tests := []struct {
		name         string
		templatePath string
		templateData FlashcardTemplate
		wantErr      bool
		wantContains []string
	}{
		{
			name:         "uses fallback template with basic card",
			templatePath: "",
			templateData: FlashcardTemplate{
				Notebooks: []FlashcardNotebook{
					{
						Title:       "Daily Vocabulary",
						Description: "Common expressions",
						Date:        time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
						Cards: []FlashcardCard{
							{
								Expression:    "break the ice",
								Meaning:       "to initiate conversation",
								Pronunciation: "breyk thuh ahys",
								PartOfSpeech:  "idiom",
								Examples:      []string{"She told a joke to break the ice."},
							},
						},
					},
				},
			},
			wantContains: []string{
				"Daily Vocabulary",
				"break the ice",
				"to initiate conversation",
				"breyk thuh ahys",
				"idiom",
				"2025-06-15",
			},
		},
		{
			name:         "card with synonyms and antonyms",
			templatePath: "",
			templateData: FlashcardTemplate{
				Notebooks: []FlashcardNotebook{
					{
						Title: "Review",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Cards: []FlashcardCard{
							{
								Expression: "happy",
								Meaning:    "feeling pleasure",
								Synonyms:   []string{"joyful", "glad"},
								Antonyms:   []string{"sad", "unhappy"},
							},
						},
					},
				},
			},
			wantContains: []string{
				"happy",
				"feeling pleasure",
				"joyful",
				"sad",
			},
		},
		{
			name: "uses filesystem template when available",
			templatePath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				templatePath := filepath.Join(tmpDir, "custom-flashcard.md.go.tmpl")
				content := `Custom: {{ range .Notebooks }}{{ .Title }}{{ end }}`
				err := os.WriteFile(templatePath, []byte(content), 0644)
				require.NoError(t, err)
				return templatePath
			}(t),
			templateData: FlashcardTemplate{
				Notebooks: []FlashcardNotebook{
					{Title: "Test Cards"},
				},
			},
			wantContains: []string{"Custom: Test Cards"},
		},
		{
			name: "returns error when filesystem template is invalid",
			templatePath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				templatePath := filepath.Join(tmpDir, "invalid.md.go.tmpl")
				err := os.WriteFile(templatePath, []byte(`{{ .Unclosed`), 0644)
				require.NoError(t, err)
				return templatePath
			}(t),
			templateData: FlashcardTemplate{},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			gotErr := WriteFlashcardNotebook(&buf, tt.templatePath, tt.templateData)
			if tt.wantErr {
				require.Error(t, gotErr)
				return
			}
			require.NoError(t, gotErr)
			output := buf.String()
			for _, s := range tt.wantContains {
				assert.Contains(t, output, s)
			}
		})
	}
}

func TestWriteFlashcardNotebook_NonExistentTemplatePath(t *testing.T) {
	var buf bytes.Buffer
	err := WriteFlashcardNotebook(&buf, "/non/existent/template.tmpl", FlashcardTemplate{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template file not found")
}

func TestWriteStoryNotebook_NonExistentTemplatePath(t *testing.T) {
	var buf bytes.Buffer
	err := WriteStoryNotebook(&buf, "/non/existent/template.tmpl", StoryTemplate{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template file not found")
}

func TestWriteStoryNotebook(t *testing.T) {
	tests := []struct {
		name         string
		templatePath string

		wantErr              bool
		templateData         StoryTemplate
		wantTemplateContents string
	}{
		{
			name:         "uses fallback template when path is empty",
			templatePath: "",
			templateData: StoryTemplate{
				Notebooks: []StoryNotebook{
					{
						Event: "Empty Path Test",
						Scenes: []StoryScene{
							{
								Title: "Test",
								Conversations: []Conversation{
									{Speaker: "System", Quote: "Using embedded template for testing here."},
								},
								Definitions: []StoryNote{
									{Expression: "test phrase", Meaning: "A phrase for testing", Pronunciation: "test-phrase", PartOfSpeech: "noun"},
									{Expression: "test phrase 2", Meaning: "A phrase for testing 2"},
								},
							},
						},
					},
				},
			},
			wantTemplateContents: "\n## Empty Path Test\n\n\n\n---\n\n### Test\n\n\n- _System_: Using embedded template for testing here.\n\n#### Words and phrases\n\n\n- **test phrase** /test-phrase/ [noun]: A phrase for testing\n\n\n \n\n\n- **test phrase 2**: A phrase for testing 2\n\n\n \n\n \n \n \n",
		},
		{
			name: "uses filesystem template when available",
			templatePath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				templatePath := filepath.Join(tmpDir, "custom.md.go.tmpl")
				content := `Filesystem Template: {{ range .Notebooks }}{{ .Event }}{{ end }}`
				err := os.WriteFile(templatePath, []byte(content), 0644)
				require.NoError(t, err)
				return templatePath
			}(t),
			templateData: StoryTemplate{
				Notebooks: []StoryNotebook{
					{Event: "Test Event",
						Scenes: []StoryScene{
							{Title: "Test", Conversations: []Conversation{
								{Speaker: "System", Quote: "Using filesystem template for testing here."},
							},
							},
						},
					},
				},
			},
			wantTemplateContents: "Filesystem Template: Test Event",
		},
		{
			name: "returns error when filesystem template is invalid",
			templatePath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				templatePath := filepath.Join(tmpDir, "invalid.md.go.tmpl")
				badContent := `Bad: {{ .Unclosed`
				err := os.WriteFile(templatePath, []byte(badContent), 0644)
				require.NoError(t, err)
				return templatePath
			}(t),
			templateData: StoryTemplate{
				Notebooks: []StoryNotebook{
					{Event: "Test Event",
						Scenes: []StoryScene{
							{Title: "Test", Conversations: []Conversation{
								{Speaker: "System", Quote: "Using filesystem template for testing here."},
							}},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute
			var buf bytes.Buffer
			gotErr := WriteStoryNotebook(&buf, tt.templatePath, tt.templateData)
			if tt.wantErr {
				require.Error(t, gotErr)
				return
			}
			assert.NoError(t, gotErr)
			assert.Equal(t, tt.wantTemplateContents, buf.String())
		})
	}
}
