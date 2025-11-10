package assets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
