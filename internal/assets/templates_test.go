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
						Definitions []struct {
							Expression    string
							Definition    string
							Pronunciation string
							PartOfSpeech  string
							Meaning       string
							Examples      []string
							Origin        string
							Synonyms      []string
							Antonyms      []string
							Images        []string
						}
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
						Definitions []struct {
							Expression    string
							Definition    string
							Pronunciation string
							PartOfSpeech  string
							Meaning       string
							Examples      []string
							Origin        string
							Synonyms      []string
							Antonyms      []string
							Images        []string
						}
					}
				}{
					{
						Event: "Fallback Event",
						Scenes: []struct {
							Title         string
							Conversations []struct {
								Speaker string
								Quote   string
							}
							Definitions []struct {
								Expression    string
								Definition    string
								Pronunciation string
								PartOfSpeech  string
								Meaning       string
								Examples      []string
								Origin        string
								Synonyms      []string
								Antonyms      []string
								Images        []string
							}
						}{
							{
								Title: "Test Scene",
								Conversations: []struct {
									Speaker string
									Quote   string
								}{
									{Speaker: "Alice", Quote: "Hello World"},
								},
								Definitions: []struct {
									Expression    string
									Definition    string
									Pronunciation string
									PartOfSpeech  string
									Meaning       string
									Examples      []string
									Origin        string
									Synonyms      []string
									Antonyms      []string
									Images        []string
								}{
									{
										Expression:    "greeting",
										Definition:    "a polite word or sign of welcome",
										Pronunciation: "gree-ting",
										PartOfSpeech:  "noun",
										Meaning:       "a polite word",
										Examples:      []string{"Good morning!"},
										Origin:        "Old English",
										Synonyms:      []string{"hello", "hi"},
										Antonyms:      []string{"farewell"},
										Images:        []string{},
									},
								},
							},
						},
					},
				},
			},
			// Note: The template has trailing spaces on empty lines due to template formatting
			wantTemplateContents: "\n## Fallback Event\n\n\n\n---\n\n### Test Scene\n\n\n- _Alice_: Hello World\n\n#### Words and phrases\n\n\n- **a polite word or sign of welcome** /gree-ting/ [noun]: a polite word\n    - Examples:\n        - Good morning!\n\n    - Origin: Old English\n\n\n    - Synonyms: hello, hi\n\n\n    - Antonyms: farewell\n \n\n \n \n \n",
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
						Event: "Using Fallback",
						Scenes: []struct {
							Title         string
							Conversations []struct {
								Speaker string
								Quote   string
							}
							Definitions []interface{}
						}{
							{
								Title: "Scene",
								Conversations: []struct {
									Speaker string
									Quote   string
								}{
									{Speaker: "Bob", Quote: "Fallback works"},
								},
								Definitions: []interface{}{},
							},
						},
					},
				},
			},
			// Note: The template has trailing spaces on empty lines due to template formatting
			wantTemplateContents: "\n## Using Fallback\n\n\n\n---\n\n### Scene\n\n\n- _Bob_: Fallback works\n\n#### Words and phrases\n\n \n \n \n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute
			got, gotErr := ParseStoryTemplate(tt.templatePath)
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
