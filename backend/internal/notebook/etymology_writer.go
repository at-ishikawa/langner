package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/assets"
	"github.com/at-ishikawa/langner/internal/pdf"
)

// EtymologyNotebookWriter handles writing etymology notebooks to various output formats
type EtymologyNotebookWriter struct {
	reader                 *Reader
	templatePath           string
	definitionsDirectories []string
	learningHistories      map[string][]LearningHistory
}

// NewEtymologyNotebookWriter creates a new EtymologyNotebookWriter
func NewEtymologyNotebookWriter(reader *Reader, templatePath string, definitionsDirectories []string, learningHistories map[string][]LearningHistory) *EtymologyNotebookWriter {
	return &EtymologyNotebookWriter{
		reader:                 reader,
		templatePath:           templatePath,
		definitionsDirectories: definitionsDirectories,
		learningHistories:      learningHistories,
	}
}

// OutputEtymologyNotebook generates markdown (and optionally PDF) output from etymology notebooks.
// It merges origins from the etymology directory with definitions from the definitions/books directory.
func (writer EtymologyNotebookWriter) OutputEtymologyNotebook(
	etymologyID string,
	outputDirectory string,
	generatePDF bool,
) error {
	etymIndex, ok := writer.reader.etymologyIndexes[etymologyID]
	if !ok {
		return fmt.Errorf("etymology notebook %s not found", etymologyID)
	}

	// Build chapters by reading each session file
	chapters, err := writer.buildChapters(etymIndex)
	if err != nil {
		return fmt.Errorf("buildChapters: %w", err)
	}

	if len(chapters) == 0 {
		return fmt.Errorf("no chapters found for etymology %s", etymologyID)
	}

	templateData := assets.EtymologyTemplate{
		Name:     etymIndex.Name,
		Chapters: chapters,
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDirectory, 0755); err != nil {
		return fmt.Errorf("os.MkdirAll(%s) > %w", outputDirectory, err)
	}

	outputFilename := strings.TrimSpace(filepath.Join(outputDirectory, etymologyID+".md"))
	output, err := os.Create(outputFilename)
	if err != nil {
		return fmt.Errorf("os.Create(%s) > %w", outputFilename, err)
	}
	defer func() {
		_ = output.Close()
	}()

	if err := assets.WriteEtymologyNotebook(output, writer.templatePath, templateData); err != nil {
		return fmt.Errorf("assets.WriteEtymologyNotebook: %w", err)
	}

	fmt.Printf("Etymology notebook written to: %s\n", outputFilename)

	if generatePDF {
		pdfPath, err := pdf.ConvertMarkdownToPDF(outputFilename)
		if err != nil {
			return fmt.Errorf("ConvertMarkdownToPDF(%s) > %w", outputFilename, err)
		}
		fmt.Printf("PDF generated at: %s\n", pdfPath)
	}

	return nil
}

// buildChapters reads etymology session files and merges origins with definitions
// to produce a list of chapters for template rendering.
func (writer EtymologyNotebookWriter) buildChapters(etymIndex EtymologyIndex) ([]assets.EtymologyChapter, error) {
	// Find the definitions directory that has matching session files
	defDir := writer.findDefinitionsDir(etymIndex)

	var chapters []assets.EtymologyChapter

	for _, nbPath := range etymIndex.NotebookPaths {
		sessionPath := filepath.Join(etymIndex.Path, nbPath)

		// Read origins from the etymology session file
		sessionOrigins, err := readSessionOrigins(sessionPath)
		if err != nil {
			return nil, fmt.Errorf("readSessionOrigins(%s): %w", sessionPath, err)
		}

		// Build origin map for resolving origin_parts meanings
		originMap := buildOriginMap(sessionOrigins)

		// Convert origins to template format, filtering by learning status.
		// Only include origins whose latest etymology log is misunderstood or
		// that have no correct answer yet (i.e., still need study).
		var templateOrigins []assets.EtymologyOriginEntry
		for _, o := range sessionOrigins {
			if !writer.originNeedsStudy(etymIndex.ID, etymIndex.Name, o.Origin) {
				continue
			}
			templateOrigins = append(templateOrigins, assets.EtymologyOriginEntry{
				Origin:   o.Origin,
				Language: o.Language,
				Meaning:  o.Meaning,
			})
		}

		// Read definitions for this session directly from the definitions file.
		// A word is hidden when every origin it references has been mastered
		// (not in the current "needs study" set built above) — once nothing in
		// a section is left to learn, the section header drops out too.
		sessionFilename := filepath.Base(nbPath)
		needsStudy := func(origin string) bool {
			return writer.originNeedsStudy(etymIndex.ID, etymIndex.Name, origin)
		}
		defChapters := readDefinitionsFileChapters(defDir, sessionFilename, originMap, needsStudy)

		if len(defChapters) > 0 {
			filteredChapters := make([]assets.EtymologyChapter, 0, len(defChapters))
			for i := range defChapters {
				// Only include origins that are referenced by words in this chapter
				defChapters[i].Origins = filterOriginsForChapter(templateOrigins, defChapters[i])
				if chapterIsEmpty(defChapters[i]) {
					continue
				}
				filteredChapters = append(filteredChapters, defChapters[i])
			}
			chapters = append(chapters, filteredChapters...)
		} else if len(templateOrigins) > 0 {
			// No definitions found; create a single chapter with just origins
			title := strings.TrimSuffix(sessionFilename, filepath.Ext(sessionFilename))
			chapters = append(chapters, assets.EtymologyChapter{
				Title:   title,
				Origins: templateOrigins,
			})
		}
	}

	return chapters, nil
}

// chapterIsEmpty reports whether a chapter contributes nothing to the export.
// A chapter is empty when there are no origins to learn at the top of the
// chapter and no surviving words in either the top-level or section bodies.
func chapterIsEmpty(c assets.EtymologyChapter) bool {
	if len(c.Origins) > 0 || len(c.Words) > 0 {
		return false
	}
	for _, s := range c.Sections {
		if len(s.Words) > 0 {
			return false
		}
	}
	return true
}

// readSessionOrigins reads the origins from an etymology session file.
func readSessionOrigins(path string) ([]EtymologyOrigin, error) {
	// Try flat list first
	origins, flatErr := readYamlFile[[]EtymologyOrigin](path)
	if flatErr == nil {
		return origins, nil
	}

	// Try wrapped format
	wrapped, wrappedErr := readYamlFile[etymologySessionFile](path)
	if wrappedErr != nil {
		return nil, fmt.Errorf("readYamlFile(%s) > %w", path, flatErr)
	}
	return wrapped.Origins, nil
}

// buildOriginMap creates a map from origin name to its meaning for quick lookup.
func buildOriginMap(origins []EtymologyOrigin) map[string]string {
	m := make(map[string]string, len(origins))
	for _, o := range origins {
		m[o.Origin] = o.Meaning
	}
	return m
}

// findDefinitionsDir finds the definitions directory that has a matching session file
// for the etymology index. It walks all definitions directories looking for an index.yml
// with a matching book ID or matching session filenames.
func (writer EtymologyNotebookWriter) findDefinitionsDir(etymIndex EtymologyIndex) string {
	etymSessions := make(map[string]bool, len(etymIndex.NotebookPaths))
	for _, p := range etymIndex.NotebookPaths {
		etymSessions[filepath.Base(p)] = true
	}

	var found string
	for _, dir := range writer.definitionsDirectories {
		if dir == "" {
			continue
		}

		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || filepath.Base(path) != "index.yml" {
				return nil
			}

			idx, readErr := readYamlFile[definitionsIndex](path)
			if readErr != nil || idx.ID == "" {
				return nil
			}

			idxDir := filepath.Dir(path)

			// Check by ID convention
			if idx.ID == etymIndex.ID+"-vocab" || idx.ID == etymIndex.ID {
				found = idxDir
				return filepath.SkipAll
			}

			// Check if this definitions index has matching session files
			for _, nbPath := range idx.Notebooks {
				if etymSessions[filepath.Base(nbPath)] {
					found = idxDir
					return filepath.SkipAll
				}
			}

			return nil
		})

		if found != "" {
			return found
		}
	}

	return ""
}

// originNeedsStudy returns true if the origin should be included in the PDF
// based on learning history. An origin needs study if:
//   - it has no etymology learning history at all (never encountered), OR
//   - its latest EtymologyBreakdownLogs entry is misunderstood, OR
//   - it has freeform entries but no correct answers yet
//
// Origins that have been successfully mastered (latest status is understood/usable)
// and are not yet due for review are excluded.
func (writer EtymologyNotebookWriter) originNeedsStudy(etymID, nbTitle, origin string) bool {
	histories := writer.learningHistories[etymID]
	if len(histories) == 0 {
		return true // no history → include
	}

	for _, h := range histories {
		if h.Metadata.Title != nbTitle {
			continue
		}
		for _, expr := range h.Expressions {
			if !strings.EqualFold(expr.Expression, origin) {
				continue
			}
			// Found learning record for this origin.
			logs := expr.EtymologyBreakdownLogs
			if len(logs) == 0 {
				return true // no etymology logs → include
			}
			// Check latest status — if misunderstood, definitely needs study.
			latest := logs[0]
			for _, l := range logs[1:] {
				if l.LearnedAt.After(latest.LearnedAt.Time) {
					latest = l
				}
			}
			if latest.Status == LearnedStatusMisunderstood {
				return true
			}
			// Check SR: if the review interval has elapsed, needs study.
			if latest.IntervalDays > 0 {
				elapsed := int(time.Since(latest.LearnedAt.Time).Hours() / 24)
				return elapsed >= latest.IntervalDays
			}
			// Has correct answer and no interval → recently reviewed, skip.
			return false
		}
	}
	return true // not found in history → include
}

// filterOriginsForChapter returns only the origins that are referenced by any word's origin parts,
// including words inside sections.
func filterOriginsForChapter(allOrigins []assets.EtymologyOriginEntry, chapter assets.EtymologyChapter) []assets.EtymologyOriginEntry {
	used := make(map[string]bool)
	for _, w := range chapter.Words {
		for _, op := range w.OriginParts {
			used[op.Origin] = true
		}
	}
	for _, s := range chapter.Sections {
		for _, w := range s.Words {
			for _, op := range w.OriginParts {
				used[op.Origin] = true
			}
		}
	}
	var filtered []assets.EtymologyOriginEntry
	for _, o := range allOrigins {
		if used[o.Origin] {
			filtered = append(filtered, o)
		}
	}
	return filtered
}

// readDefinitionsFileChapters reads definitions from a file matching the
// session filename in the definitions directory, and groups them into chapters
// by metadata.title.
//
// needsStudy is consulted per word: a word is dropped from the export when
// every origin it references reports false (i.e. fully mastered). Sections
// with no surviving words are dropped along with their header so the user
// doesn't see "## verto (to turn)" headings for origins they've finished.
func readDefinitionsFileChapters(defDir, sessionFilename string, originMap map[string]string, needsStudy func(origin string) bool) []assets.EtymologyChapter {
	if defDir == "" {
		return nil
	}

	defPath := filepath.Join(defDir, sessionFilename)
	definitions, err := readYamlFile[[]Definitions](defPath)
	if err != nil {
		return nil
	}

	// Group definitions by title so entries with the same session name merge into one chapter
	chapterMap := make(map[string]int) // title -> index in chapters
	var chapters []assets.EtymologyChapter
	for _, def := range definitions {
		title := def.Metadata.Title
		if title == "" {
			title = def.Metadata.Notebook
		}
		if title == "" {
			continue
		}

		var allWords []assets.EtymologyWordEntry
		var sections []assets.EtymologySection
		for _, scene := range def.Scenes {
			var sceneWords []assets.EtymologyWordEntry
			for _, note := range scene.Expressions {
				if !wordNeedsStudy(note.OriginParts, needsStudy) {
					continue
				}
				word := assets.EtymologyWordEntry{
					Expression:    note.Expression,
					Definition:    note.Definition,
					Meaning:       note.Meaning,
					Pronunciation: note.Pronunciation,
					PartOfSpeech:  note.PartOfSpeech,
					Note:          note.Note,
				}

				for _, op := range note.OriginParts {
					ref := assets.EtymologyOriginRef{
						Origin:  op.Origin,
						Meaning: originMap[op.Origin],
					}
					word.OriginParts = append(word.OriginParts, ref)
				}

				sceneWords = append(sceneWords, word)
			}
			if len(sceneWords) == 0 {
				continue
			}
			allWords = append(allWords, sceneWords...)
			if scene.Metadata.Title != "" {
				sections = append(sections, assets.EtymologySection{
					Title: scene.Metadata.Title,
					Words: sceneWords,
				})
			}
		}

		// Merge into existing chapter with the same title, or create a new one
		if idx, exists := chapterMap[title]; exists {
			chapters[idx].Words = append(chapters[idx].Words, allWords...)
			chapters[idx].Sections = append(chapters[idx].Sections, sections...)
		} else {
			chapter := assets.EtymologyChapter{
				Title: title,
			}
			if len(sections) > 0 {
				chapter.Sections = sections
			} else {
				chapter.Words = allWords
			}
			chapterMap[title] = len(chapters)
			chapters = append(chapters, chapter)
		}
	}

	return chapters
}

// wordNeedsStudy returns true when at least one of the word's origin parts
// still needs review. A word with no origin_parts is always kept (we have no
// signal to filter it on). When needsStudy is nil all words are kept (used by
// callers that don't have learning history available).
func wordNeedsStudy(originParts []OriginPartRef, needsStudy func(origin string) bool) bool {
	if needsStudy == nil || len(originParts) == 0 {
		return true
	}
	for _, op := range originParts {
		if needsStudy(op.Origin) {
			return true
		}
	}
	return false
}
