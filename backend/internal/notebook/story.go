package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/assets"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/pdf"
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
	Conversations []Conversation `yaml:"conversations,omitempty"`
	Statements    []string       `yaml:"statements,omitempty"`
	Type          string         `yaml:"type,omitempty"`
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
	notebookPaths := make([]string, 0)
	for _, notebookPath := range index.NotebookPaths {
		path := filepath.Join(index.Path, notebookPath)

		notebooks, err := readYamlFile[[]StoryNotebook](path)
		if err != nil {
			// Skip files that can't be parsed as story notebooks
			// (e.g., definition-only files with origin_parts)
			continue
		}

		index.Notebooks = append(index.Notebooks, notebooks)
		result = append(result, notebooks...)
		// Track notebook paths for definitions merging
		for range notebooks {
			notebookPaths = append(notebookPaths, notebookPath)
		}
	}
	reader.indexes[storyID] = index

	// Merge definitions from separate definitions files
	result = MergeDefinitionsIntoNotebooks(storyID, result, notebookPaths, reader.definitionsMap)

	return result, nil
}

// ReadAllStoryNotebooksMap reads all story notebooks and returns them as a map keyed by notebook ID
func (reader *Reader) ReadAllStoryNotebooksMap() (map[string][]StoryNotebook, error) {
	result := make(map[string][]StoryNotebook)

	for id := range reader.indexes {
		notebooks, err := reader.ReadStoryNotebooks(id)
		if err != nil {
			return nil, fmt.Errorf("ReadStoryNotebooks(%s) > %w", id, err)
		}
		result[id] = notebooks
	}

	return result, nil
}

// ConvertMarkersInText converts {{ }} markers in a text string based on conversion style
// If targetExpression is provided, only that expression will be highlighted
// definitions is a list of expressions that should be learned
func ConvertMarkersInText(text string, definitions []Note, conversionStyle ConversionStyle, targetExpression string) string {
	// Find all {{ ... }} patterns (with optional spaces)
	markerPattern := regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)
	color.NoColor = false // Force color output even in non-TTY environments
	bold := color.New(color.Bold)

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

// HighlightDefinitionsInText highlights definition expressions found in text
// without relying on {{ }} markers. It matches definition Expression (and Definition
// if set) against the text using case-insensitive matching.
// For single-word expressions, word boundary matching is used to avoid partial matches.
// For multi-word expressions, exact substring matching is used.
func HighlightDefinitionsInText(text string, definitions []Note, conversionStyle ConversionStyle) string {
	if conversionStyle == ConversionStylePlain {
		return text
	}

	color.NoColor = false
	bold := color.New(color.Bold)

	// Collect all expressions to match, longest first to avoid partial replacements
	type exprEntry struct {
		pattern *regexp.Regexp
	}
	var entries []exprEntry

	// Deduplicate and sort expressions longest-first
	seen := make(map[string]bool)
	var expressions []string
	for _, def := range definitions {
		expr := strings.TrimSpace(def.Expression)
		if expr != "" && !seen[strings.ToLower(expr)] {
			seen[strings.ToLower(expr)] = true
			expressions = append(expressions, expr)
		}
		if def.Definition != "" && !strings.EqualFold(def.Definition, expr) {
			defStr := strings.TrimSpace(def.Definition)
			if defStr != "" && !seen[strings.ToLower(defStr)] {
				seen[strings.ToLower(defStr)] = true
				expressions = append(expressions, defStr)
			}
		}
	}
	// Sort longest first so longer expressions are matched before shorter substrings
	sort.Slice(expressions, func(i, j int) bool {
		return len(expressions[i]) > len(expressions[j])
	})

	for _, expr := range expressions {
		escaped := regexp.QuoteMeta(expr)
		var patternStr string
		if strings.Contains(expr, " ") {
			// Multi-word: case-insensitive substring match
			patternStr = `(?i)` + escaped
		} else {
			// Single-word: use word boundaries to avoid partial matches.
			// Only apply \b where the expression starts/ends with a word character,
			// since \b requires a transition between word and non-word characters.
			prefix := ""
			suffix := ""
			if len(expr) > 0 && isWordChar(expr[0]) {
				prefix = `\b`
			}
			if len(expr) > 0 && isWordChar(expr[len(expr)-1]) {
				suffix = `\b`
			}
			patternStr = `(?i)` + prefix + escaped + suffix
		}
		compiled, err := regexp.Compile(patternStr)
		if err != nil {
			continue
		}
		entries = append(entries, exprEntry{pattern: compiled})
	}

	// Apply replacements sequentially, using a placeholder to prevent
	// double-highlighting of already-replaced text.
	type replacement struct {
		start int
		end   int
		text  string
	}

	for _, entry := range entries {
		var replacements []replacement
		matches := entry.pattern.FindAllStringIndex(text, -1)
		for _, loc := range matches {
			matched := text[loc[0]:loc[1]]
			// Skip if this match is already inside a highlighted region
			// Check for surrounding ** (markdown) or ANSI codes (terminal)
			if loc[0] >= 2 && text[loc[0]-2:loc[0]] == "**" {
				continue
			}
			var highlighted string
			switch conversionStyle {
			case ConversionStyleMarkdown:
				highlighted = fmt.Sprintf("**%s**", matched)
			case ConversionStyleTerminal:
				highlighted = bold.Sprint(matched)
			default:
				highlighted = matched
			}
			replacements = append(replacements, replacement{start: loc[0], end: loc[1], text: highlighted})
		}
		// Apply replacements from end to start so indices remain valid
		for i := len(replacements) - 1; i >= 0; i-- {
			r := replacements[i]
			text = text[:r.start] + r.text + text[r.end:]
		}
	}

	return text
}

// isWordChar returns true if the byte is a word character (letter, digit, or underscore),
// matching the behavior of \b in regular expressions.
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
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

func FilterStoryNotebooks(storyNotebooks []StoryNotebook, learningHistory []LearningHistory, dictionaryMap map[string]rapidapi.Response, sortDesc bool, includeNoCorrectAnswers bool, useSpacedRepetition bool, preserveOrder bool) ([]StoryNotebook, error) {
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

				// Skip words that are marked as skipped in learning history
				if isExpressionSkipped(learningHistory, notebook.Event, scene.Title, definition) {
					continue
				}

				// Filter out words without any correct answers if not included
				if !includeNoCorrectAnswers && !definition.hasAnyCorrectAnswer() {
					continue
				}
				if useSpacedRepetition {
					if !definition.needsToLearn() {
						continue
					}
				} else {
					if !definition.needsToLearnInNotebook() {
						continue
					}
				}
				if err := definition.SetDetails(dictionaryMap, ""); err != nil {
					return nil, fmt.Errorf("definition.SetDetails() > %w", err)
				}
				definitions = append(definitions, definition)
			}
			if len(definitions) == 0 {
				continue
			}

			scene.Definitions = definitions
			if len(scene.Conversations) == 0 && len(scene.Statements) == 0 {
				// Skip scenes with definitions but no context (e.g., definitions-only books)
				continue
			}
			scenes = append(scenes, scene)
		}
		if len(scenes) == 0 {
			continue
		}

		notebook.Scenes = scenes
		result = append(result, notebook)
	}

	if !preserveOrder {
		if sortDesc {
			sort.Slice(result, func(i, j int) bool {
				return result[i].Date.After(result[j].Date)
			})
		} else {
			sort.Slice(result, func(i, j int) bool {
				return result[i].Date.Before(result[j].Date)
			})
		}
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

	// For books, preserve index order instead of sorting by date
	preserveOrder := writer.reader.IsBook(storyID)
	notebooks, err = FilterStoryNotebooks(notebooks, learningHistory, dictionaryMap, sortDesc, true, false, preserveOrder)
	if err != nil {
		return fmt.Errorf("filterStoryNotebooks() > %w", err)
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

	// Convert notebooks to assets format with marker conversion for markdown output
	converter := newAssetsStoryConverter()
	templateData := converter.convertToAssetsStoryTemplate(notebooks)
	if err := assets.WriteStoryNotebook(output, writer.templatePath, templateData); err != nil {
		return fmt.Errorf("assets.WriteStoryNotebook(%s, %s, ) > %w", outputFilename, writer.templatePath, err)
	}

	fmt.Printf("Story notebook written to: %s\n", outputFilename)

	if generatePDF {
		pdfPath, err := pdf.ConvertMarkdownToPDF(outputFilename)
		if err != nil {
			return fmt.Errorf("ConvertMarkdownToPDF(%s) > %w", outputFilename, err)
		}
		fmt.Printf("PDF generated at: %s\n", pdfPath)
	}

	return nil
}

// Validate validates a StoryScene and its definitions against conversations and statements
func (scene *StoryScene) Validate(location string) []ValidationError {
	var errors []ValidationError

	// Check each definition appears in at least one conversation or statement
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

		// Check if expression appears in any conversation quote or statement (case-insensitive)
		found := false
		lowerExpression := strings.ToLower(expression)

		// Check conversations
		for _, conv := range scene.Conversations {
			lowerQuote := strings.ToLower(conv.Quote)
			if strings.Contains(lowerQuote, lowerExpression) {
				found = true
				break
			}
		}

		// Check statements if not already found
		if !found {
			for _, statement := range scene.Statements {
				lowerStatement := strings.ToLower(statement)
				if strings.Contains(lowerStatement, lowerExpression) {
					found = true
					break
				}
			}
		}

		// If not found at all, report error
		if !found {
			errors = append(errors, ValidationError{
				Location: defLocation,
				Message:  fmt.Sprintf("expression %q not found in any conversation quote or statement", expression),
				Suggestions: []string{
					"add the expression to a conversation quote or statement",
					"or mark it as not_used: true",
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

// ConvertToAssetsStoryTemplate converts notebook types to assets.StoryTemplate for template rendering.
func ConvertToAssetsStoryTemplate(notebooks []StoryNotebook) assets.StoryTemplate {
	return newAssetsStoryConverter().convertToAssetsStoryTemplate(notebooks)
}

type assetsStoryConverter struct {
}

func newAssetsStoryConverter() *assetsStoryConverter {
	return &assetsStoryConverter{}
}

// convertToAssetsStoryTemplate converts notebook types to assets.StoryTemplate for template rendering
// and applies marker conversion to conversation quotes based on the conversion style
func (converter assetsStoryConverter) convertToAssetsStoryTemplate(notebooks []StoryNotebook) assets.StoryTemplate {
	assetsNotebooks := make([]assets.StoryNotebook, len(notebooks))
	for i, nb := range notebooks {
		assetsNotebooks[i] = converter.convertStoryNotebook(nb)
	}
	return assets.StoryTemplate{
		Notebooks: assetsNotebooks,
	}
}

func (converter assetsStoryConverter) convertStoryNotebook(nb StoryNotebook) assets.StoryNotebook {
	assetsScenes := make([]assets.StoryScene, len(nb.Scenes))
	for i, scene := range nb.Scenes {
		assetsScenes[i] = converter.convertStoryScene(scene)
	}
	return assets.StoryNotebook{
		Event: nb.Event,
		Metadata: assets.Metadata{
			Series:  nb.Metadata.Series,
			Season:  nb.Metadata.Season,
			Episode: nb.Metadata.Episode,
		},
		Date:   nb.Date,
		Scenes: assetsScenes,
	}
}

func (converter assetsStoryConverter) convertStoryScene(scene StoryScene) assets.StoryScene {
	assetsConversations := make([]assets.Conversation, len(scene.Conversations))
	for i, conv := range scene.Conversations {
		// First strip any remaining {{ }} markers (backward compat), then highlight definitions
		convertedQuote := ConvertMarkersInText(conv.Quote, scene.Definitions, ConversionStyleMarkdown, "")
		convertedQuote = HighlightDefinitionsInText(convertedQuote, scene.Definitions, ConversionStyleMarkdown)
		assetsConversations[i] = assets.Conversation{
			Speaker: conv.Speaker,
			Quote:   convertedQuote,
		}
	}

	// Convert statements: strip markers then highlight definitions
	assetsStatements := make([]string, len(scene.Statements))
	for i, statement := range scene.Statements {
		converted := ConvertMarkersInText(statement, scene.Definitions, ConversionStyleMarkdown, "")
		assetsStatements[i] = HighlightDefinitionsInText(converted, scene.Definitions, ConversionStyleMarkdown)
	}

	assetsNotes := make([]assets.StoryNote, len(scene.Definitions))
	for i, note := range scene.Definitions {
		assetsNotes[i] = assets.StoryNote{
			Definition:    note.Definition,
			Expression:    note.Expression,
			Meaning:       note.Meaning,
			Examples:      note.Examples,
			Pronunciation: note.Pronunciation,
			PartOfSpeech:  note.PartOfSpeech,
			Origin:        note.Origin,
			Note:          note.Note,
			Synonyms:      note.Synonyms,
			Antonyms:      note.Antonyms,
			Images:        note.Images,
		}
	}

	return assets.StoryScene{
		Title:         scene.Title,
		Conversations: assetsConversations,
		Statements:    assetsStatements,
		Type:          scene.Type,
		Definitions:   assetsNotes,
	}
}

// isExpressionSkipped checks if a definition is marked as skipped in learning history.
func isExpressionSkipped(learningHistory []LearningHistory, event, sceneTitle string, def Note) bool {
	return IsExpressionSkipped(learningHistory, event, sceneTitle, def.Expression, def.Definition)
}
