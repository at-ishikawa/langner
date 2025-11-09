package assets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStoryTemplate(t *testing.T) {
	tests := []struct {
		name         string
		templatePath string

		wantTemplateName string
		wantErr          bool

		templateData         interface{}
		wantTemplateContents string
	}{
		{
			name:             "uses fallback template when path is empty",
			templatePath:     "",
			wantTemplateName: "story-notebook.md.go.tmpl",
			templateData: struct {
				Notebooks []struct {
					Event  string
					Scenes []struct {
						Title         string
						Conversations []struct {
							Speaker string
							Quote   string
						}
						Definitions []interface{}
					}
				}
			}{
				Notebooks: []struct {
					Event  string
					Scenes []struct {
						Title         string
						Conversations []struct {
							Speaker string
							Quote   string
						}
						Definitions []interface{}
					}
				}{
					{
						Event: "Empty Path Test",
						Scenes: []struct {
							Title         string
							Conversations []struct {
								Speaker string
								Quote   string
							}
							Definitions []interface{}
						}{
							{
								Title: "Test",
								Conversations: []struct {
									Speaker string
									Quote   string
								}{
									{Speaker: "System", Quote: "Using embedded template"},
								},
								Definitions: []interface{}{},
							},
						},
					},
				},
			},
			wantTemplateContents: "\n## Empty Path Test\n\n\n\n---\n\n### Test\n\n\n- _System_: Using embedded template\n\n#### Words and phrases\n\n \n \n \n",
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
			wantTemplateName: "custom.md.go.tmpl",
			templateData: struct {
				Notebooks []struct {
					Event string
				}
			}{
				Notebooks: []struct {
					Event string
				}{
					{Event: "Test Event"},
				},
			},
			wantTemplateContents: "Filesystem Template: Test Event",
		},
		{
			name:             "uses embedded template when file doesn't exist",
			templatePath:     "/non/existent/invalid.md.go.tmpl",
			wantErr:          true,
		},
		{
			name: "uses embedded template when filesystem template is invalid",
			templatePath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				templatePath := filepath.Join(tmpDir, "invalid.md.go.tmpl")
				badContent := `Bad: {{ .Unclosed`
				err := os.WriteFile(templatePath, []byte(badContent), 0644)
				require.NoError(t, err)
				return templatePath
			}(t),
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute
			got, gotErr := ParseStoryTemplate(tt.templatePath)

			if tt.wantErr {
				require.Error(t, gotErr)
				assert.Nil(t, got)
				return // Exit early for error cases
			}

			require.NoError(t, gotErr)
			assert.NotNil(t, got)

			// Check template name
			assert.Equal(t, tt.wantTemplateName, got.Name())

			var buf bytes.Buffer
			gotErr = got.Execute(&buf, tt.templateData)
			require.NoError(t, gotErr)

			output := buf.String()
			assert.Equal(t, tt.wantTemplateContents, output)
		})
	}
}
