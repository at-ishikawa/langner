package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/at-ishikawa/langner/internal/converter"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/fatih/color"
)

type StoryNotebook struct {
	Event    string       `yaml:"event"`
	Metadata Metadata     `yaml:"metadata,omitempty"`
	Date     time.Time    `yaml:"date"`
	Scenes   []StoryScene `yaml:"scenes"`
}

type Metadata struct {
	Series  string `yaml:"series"`
	Season  int    `yaml:"season"`
	Episode int    `yaml:"episode"`
}

type StoryScene struct {
	Title         string         `yaml:"scene"`
	Conversations []Conversation `yaml:"conversations"`
	Definitions   []Note         `yaml:"definitions,omitempty"`
}

type Conversation struct {
	Speaker string `yaml:"speaker"`
	Quote   string `yaml:"quote"`
}

func (reader *Reader) ReadStoryNotebooks(storyID string) ([]StoryNotebook, error) {
	index, ok := reader.indexes[storyID]
	if !ok {
		return nil, fmt.Errorf("story %s not found", storyID)
	}

	result := make([]StoryNotebook, 0)
	for _, notebookPath := range index.NotebookPaths {
		path := filepath.Join(index.path, notebookPath)

		notebooks, err := readYamlFile[[]StoryNotebook](path)
		if err != nil {
			return nil, fmt.Errorf("readYamlFile(%s) > %w", path, err)
		}

		index.Notebooks = append(index.Notebooks, notebooks)
		result = append(result, notebooks...)
	}
	reader.indexes[storyID] = index
	return result, nil
}

type StoryTemplate struct {
	Notebooks []StoryNotebook
}

// ConvertMarkersInText converts {{ }} markers in a text string based on conversion style
// If targetExpression is provided, only that expression will be highlighted
// definitions is a list of expressions that should be learned
func ConvertMarkersInText(text string, definitions []Note, conversionStyle ConversionStyle, targetExpression string) string {
	// Find all {{ ... }} patterns (with optional spaces)
	markerPattern := regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)
	bold := color.New(color.Bold)
	color.NoColor = false // Force color output even in non-TTY environments

	// Replace {{ expression }} based on conversion style and whether it needs to be learned
	return markerPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Extract the expression from {{ expression }}
		submatches := markerPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		expression := strings.TrimSpace(submatches[1])

		// Find if this expression is in the definitions and needs to be learned
		needsToLearn := false
		for _, definition := range definitions {
			if strings.EqualFold(definition.Expression, expression) {
				needsToLearn = true
				break
			}
		}

		// If doesn't need to learn, just return the plain expression
		if !needsToLearn {
			return expression
		}

		// If targetExpression is specified and this is not the target, return plain
		if targetExpression != "" && !strings.EqualFold(expression, targetExpression) {
			return expression
		}

		// Convert based on style
		switch conversionStyle {
		case ConversionStyleMarkdown:
			return fmt.Sprintf("**%s**", expression)
		case ConversionStyleTerminal:
			return bold.Sprint(expression)
		case ConversionStylePlain:
			return expression
		default:
			return expression
		}
	})
}

// ConversionStyle defines how {{ expression }} markers should be converted
type ConversionStyle int

const (
	// ConversionStyleMarkdown converts {{ expression }} to **expression** for markdown
	ConversionStyleMarkdown ConversionStyle = iota
	// ConversionStyleTerminal converts {{ expression }} to bold terminal text
	ConversionStyleTerminal
	// ConversionStylePlain removes {{ }} markers without any formatting
	ConversionStylePlain
)

// ConvertStoryNotebookMarkers converts {{ }} markers in story notebooks based on conversion style
func ConvertStoryNotebookMarkers(storyNotebooks []StoryNotebook, conversionStyle ConversionStyle) []StoryNotebook {
	result := make([]StoryNotebook, 0, len(storyNotebooks))

	for _, notebook := range storyNotebooks {
		scenes := make([]StoryScene, 0, len(notebook.Scenes))
		for _, scene := range notebook.Scenes {
			conversations := make([]Conversation, 0, len(scene.Conversations))
			for _, conv := range scene.Conversations {
				conv.Quote = ConvertMarkersInText(conv.Quote, scene.Definitions, conversionStyle, "")
				conversations = append(conversations, conv)
			}
			scene.Conversations = conversations
			scenes = append(scenes, scene)
		}
		notebook.Scenes = scenes
		result = append(result, notebook)
	}

	return result
}

func FilterStoryNotebooks(storyNotebooks []StoryNotebook, learningHistory []LearningHistory, dictionaryMap map[string]rapidapi.Response, sortDesc bool, isFlashcard bool) ([]StoryNotebook, error) {
	result := make([]StoryNotebook, 0)
	for _, notebook := range storyNotebooks {
		if len(notebook.Scenes) == 0 {
			continue
		}

		scenes := make([]StoryScene, 0)
		for _, scene := range notebook.Scenes {
			definitions := make([]Note, 0)
			for _, definition := range scene.Definitions {
				for _, h := range learningHistory {
					logs := h.GetLogs(
						notebook.Event,
						scene.Title,
						definition,
					)
					if len(logs) == 0 {
						continue
					}

					// todo: Fix this!! temporary mitigation
					definition.LearnedLogs = logs
				}

				if strings.TrimSpace(definition.Expression) == "" {
					return nil, fmt.Errorf("empty definition.Expression: %v in story %s", definition, notebook.Event)
				}
				if isFlashcard {
					if !definition.needsToLearnInFlashcard(7) {
						continue
					}
				} else {
					if !definition.needsToLearnInStory() {
						continue
					}
				}
				if err := definition.setDetails(dictionaryMap, ""); err != nil {
					return nil, fmt.Errorf("definition.setDetails() > %w", err)
				}
				definitions = append(definitions, definition)
			}
			if len(definitions) == 0 {
				continue
			}

			scene.Definitions = definitions
			if len(scene.Conversations) == 0 {
				return nil, fmt.Errorf("empty scene.Conversations: %v in story %s", scene, notebook.Event)
			}
			scenes = append(scenes, scene)
		}
		if len(scenes) == 0 {
			continue
		}

		notebook.Scenes = scenes
		result = append(result, notebook)
	}

	if sortDesc {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Date.After(result[j].Date)
		})
	} else {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Date.Before(result[j].Date)
		})
	}

	return result, nil
}

type StoryNotebookWriter struct {
	reader       *Reader
	templatePath string
}

func NewStoryNotebookWriter(reader *Reader, templatePath string) *StoryNotebookWriter {
	return &StoryNotebookWriter{
		reader:       reader,
		templatePath: templatePath,
	}
}

func (writer StoryNotebookWriter) OutputStoryNotebooks(
	storyID string,
	dictionaryMap map[string]rapidapi.Response,
	learningHistories map[string][]LearningHistory,
	sortDesc bool,
	outputDirectory string,
	generatePDF bool,
) error {
	notebooks, err := writer.reader.ReadStoryNotebooks(storyID)
	if err != nil {
		return fmt.Errorf("readStoryNotebooks() > %w", err)
	}
	if len(notebooks) == 0 {
		return fmt.Errorf("no story notebooks found for %s", storyID)
	}
	learningHistory := learningHistories[storyID]

	notebooks, err = FilterStoryNotebooks(notebooks, learningHistory, dictionaryMap, sortDesc, false)
	if err != nil {
		return fmt.Errorf("filterStoryNotebooks() > %w", err)
	}

	// Convert markers to markdown for textbook output
	notebooks = ConvertStoryNotebookMarkers(notebooks, ConversionStyleMarkdown)

	fileName := "story-marp.md.go.tmpl"
	tmpl, err := template.New(fileName).
		Funcs(template.FuncMap{
			"join": strings.Join,
		}).
		ParseFiles(
			writer.templatePath,
		)
	if err != nil {
		return fmt.Errorf("template.ParseFiles() > %w", err)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDirectory, 0755); err != nil {
		return fmt.Errorf("os.MkdirAll(%s) > %w", outputDirectory, err)
	}

	// Generate output filename from story ID
	outputFilename := strings.TrimSpace(filepath.Join(outputDirectory, storyID+".md"))

	output, err := os.Create(outputFilename)
	if err != nil {
		return fmt.Errorf("os.Create(%s) > %w", outputFilename, err)
	}
	defer func() {
		_ = output.Close()
	}()

	if err := tmpl.Execute(output, StoryTemplate{
		Notebooks: notebooks,
	}); err != nil {
		return fmt.Errorf("tmpl.Execute() > %w", err)
	}

	fmt.Printf("Story notebook written to: %s\n", outputFilename)

	if generatePDF {
		pdfPath, err := converter.ConvertMarkdownToPDF(outputFilename)
		if err != nil {
			return fmt.Errorf("converter.ConvertMarkdownToPDF(%s) > %w", outputFilename, err)
		}
		fmt.Printf("PDF generated at: %s\n", pdfPath)
	}

	return nil
}

// Validate validates a StoryScene and its definitions against conversations
func (scene *StoryScene) Validate(location string) []ValidationError {
	var errors []ValidationError

	// Check each definition appears in at least one conversation
	for defIdx, def := range scene.Definitions {
		// Skip expressions marked as not_used
		if def.NotUsed {
			continue
		}

		expression := strings.TrimSpace(def.Expression)
		if expression == "" {
			continue
		}

		defLocation := fmt.Sprintf("%s -> definition[%d]: %s", location, defIdx, expression)

		// Check if expression appears in any conversation quote (case-insensitive)
		foundWithMarker := false
		foundWithoutMarker := false
		lowerExpression := strings.ToLower(expression)

		for _, conv := range scene.Conversations {
			lowerQuote := strings.ToLower(conv.Quote)

			// Check for {{ expression }} marker (case-insensitive)
			if strings.Contains(lowerQuote, fmt.Sprintf("{{ %s }}", lowerExpression)) ||
				strings.Contains(lowerQuote, fmt.Sprintf("{{%s}}", lowerExpression)) {
				foundWithMarker = true
				break
			}

			// Check case-insensitive in plain text (without markers)
			if strings.Contains(lowerQuote, lowerExpression) {
				foundWithoutMarker = true
			}
		}

		// If not found at all, report error
		if !foundWithMarker && !foundWithoutMarker {
			errors = append(errors, ValidationError{
				Location: defLocation,
				Message:  fmt.Sprintf("expression %q not found in any conversation quote", expression),
				Suggestions: []string{
					"add the expression to a conversation quote",
					"or mark it as not_used: true",
				},
			})
		} else if foundWithoutMarker && !foundWithMarker {
			// Found without marker - report as error
			errors = append(errors, ValidationError{
				Location: defLocation,
				Message:  fmt.Sprintf("expression %q found in conversation but missing {{ }} markers", expression),
				Suggestions: []string{
					"run highlight-expressions command to add {{ }} markers",
				},
			})
		}
	}

	return errors
}

// Validate validates a StoryNotebook
func (notebook *StoryNotebook) Validate(location string) []ValidationError {
	var errors []ValidationError

	for sceneIdx, scene := range notebook.Scenes {
		sceneLocation := fmt.Sprintf("%s -> scene[%d]: %s", location, sceneIdx, scene.Title)
		if sceneErrors := scene.Validate(sceneLocation); len(sceneErrors) > 0 {
			errors = append(errors, sceneErrors...)
		}
	}

	return errors
}
