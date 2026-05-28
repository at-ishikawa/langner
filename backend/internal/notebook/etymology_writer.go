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

	// Concept lookup for the paired definitions book. When non-empty,
	// readDefinitionsFileChapters collapses member entries into one row
	// per concept the same way the definitions-book writer does, so the
	// etymology PDF / markdown groups by family too.
	conceptByExpression, conceptByHead := writer.reader.GetDefinitionsBookConceptInfo(etymIndex.ID)

	var chapters []assets.EtymologyChapter

	for _, nbPath := range etymIndex.NotebookPaths {
		sessionPath := filepath.Join(etymIndex.Path, nbPath)

		// Read origins from the etymology session file
		sessionOrigins, err := readSessionOrigins(sessionPath)
		if err != nil {
			return nil, fmt.Errorf("readSessionOrigins(%s): %w", sessionPath, err)
		}

		// Read concepts + relations from the same session file. The
		// validator already loads this via loadEtymologyBookView for
		// book-wide checks; here we just want the per-session view so
		// the template can render a Concepts section.
		sessionConcepts, sessionRelations := readSessionConceptsAndRelations(sessionPath)

		// Build origin map for resolving origin_parts meanings
		originMap := buildOriginMap(sessionOrigins)

		// All origins in this session share the file's metadata.title (set by
		// readSessionOrigins). Use it as the per-session disambiguator when
		// looking up SR history.
		sessionTitle := ""
		if len(sessionOrigins) > 0 {
			sessionTitle = sessionOrigins[0].SessionTitle
		}

		// Convert origins to template format, filtering by learning status.
		// Only include origins whose latest etymology log is misunderstood or
		// that have no correct answer yet (i.e., still need study).
		var templateOrigins []assets.EtymologyOriginEntry
		for _, o := range sessionOrigins {
			if !writer.originNeedsStudy(etymIndex.ID, etymIndex.Name, o.SessionTitle, o.Origin) {
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
			return writer.originNeedsStudy(etymIndex.ID, etymIndex.Name, sessionTitle, origin)
		}
		wordHasBeenLearned := func(expression string) bool {
			return writer.expressionRecentlyLearned(etymIndex.ID, sessionTitle, expression)
		}
		// Words the user has explicitly skipped from any vocabulary
		// quiz mode (notebook / reverse / freeform) shouldn't show up
		// in the printable etymology study material either — a skip is
		// the user saying "stop studying this". Without this gate,
		// introvert / extrovert / verb (all freshly skipped) kept
		// appearing in the WPME PDF.
		wordIsSkipped := func(expression string) bool {
			return writer.expressionIsSkipped(etymIndex.ID, sessionTitle, expression)
		}
		defChapters := readDefinitionsFileChapters(defDir, sessionFilename, originMap, needsStudy, wordHasBeenLearned, wordIsSkipped, conceptByExpression, conceptByHead)

		templateConcepts := buildTemplateConcepts(sessionConcepts, sessionRelations, originMap)
		if len(defChapters) > 0 {
			filteredChapters := make([]assets.EtymologyChapter, 0, len(defChapters))
			for i := range defChapters {
				// Only include origins that are referenced by words in this chapter
				defChapters[i].Origins = filterOriginsForChapter(templateOrigins, defChapters[i])
				// Concepts are session-scoped, not chapter-scoped — attach to
				// each chapter that survived the empty-check so the user sees
				// the same Concepts block regardless of which definitions
				// title produced this chapter.
				defChapters[i].Concepts = templateConcepts
				if chapterIsEmpty(defChapters[i]) {
					continue
				}
				filteredChapters = append(filteredChapters, defChapters[i])
			}
			chapters = append(chapters, filteredChapters...)
		} else if len(templateOrigins) > 0 || len(templateConcepts) > 0 {
			// No definitions found; create a single chapter with just origins
			// and any session-declared concepts so the user still sees the
			// concept structure (gauche/sinister under leftness, etc.).
			title := strings.TrimSuffix(sessionFilename, filepath.Ext(sessionFilename))
			chapters = append(chapters, assets.EtymologyChapter{
				Title:    title,
				Origins:  templateOrigins,
				Concepts: templateConcepts,
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
// The file must use the wrapped format with a non-empty metadata.title; the
// returned origins are tagged with that title via SessionTitle.
func readSessionOrigins(path string) ([]EtymologyOrigin, error) {
	wrapped, err := readYamlFile[etymologySessionFile](path)
	if err != nil {
		return nil, fmt.Errorf("read etymology session %s: %w", path, err)
	}
	title := strings.TrimSpace(wrapped.Metadata.Title)
	if title == "" {
		return nil, fmt.Errorf("etymology session %s missing required metadata.title", path)
	}
	origins := make([]EtymologyOrigin, len(wrapped.Origins))
	for i, o := range wrapped.Origins {
		o.SessionTitle = title
		origins[i] = o
	}
	return origins, nil
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

			// The etymology and definitions notebooks share an ID after the
			// Phase 2 consolidation, so an exact match is the canonical pairing.
			if idx.ID == etymIndex.ID {
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
//   - the most recent answer (across BOTH breakdown and assembly tracks) is
//     misunderstood, OR
//   - the most recent answer's review interval has elapsed
//
// Both directions count because answering an origin in reverse counts as
// recent practice — without this combined check the PDF re-listed origins
// the user had just answered correctly in the reverse quiz earlier the same
// day.
//
// sessionTitle is the etymology session's metadata.title — origins are looked
// up under the matching scene so multi-sense origins (same string, different
// senses) are tracked independently.
func (writer EtymologyNotebookWriter) originNeedsStudy(etymID, _ /*nbTitle: unused after the migration*/, sessionTitle, origin string) bool {
	histories := writer.learningHistories[etymID]
	if len(histories) == 0 {
		return true // no history → include
	}

	// Post-migration etymology learning history is keyed by SessionTitle
	// at the top level (history.metadata.title = "Session 2") with
	// per-origin scenes underneath. The legacy code compared the
	// top-level title against the book name and the scene title against
	// the session title — both one level off, so the lookup never
	// matched and every origin defaulted to "needs study", which is why
	// the etymology PDF kept listing words the user had already learned.
	// See Validator.migrateEtymologyShape for the schema move.
	//
	// originNeedsStudy doesn't receive the per-origin SceneTitle from
	// callers, so we scan every scene's expressions under the matching
	// session and find the expression by name. Multi-sense origins (same
	// origin string, different sense) within a session share a
	// learning-history entry today, so a single match suffices.
	for _, h := range histories {
		if h.Metadata.Title != sessionTitle {
			continue
		}
		for _, scene := range h.Scenes {
			for _, expr := range scene.Expressions {
				if !strings.EqualFold(expr.Expression, origin) {
					continue
				}
				// Pick the most recent log across both etymology
				// directions. Whichever was answered last governs the
				// "is this still being studied today" decision.
				latest, found := latestEtymologyLog(expr.EtymologyBreakdownLogs, expr.EtymologyAssemblyLogs)
				if !found {
					return true // no etymology logs → include
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
	}
	return true // not found in history → include
}

// expressionRecentlyLearned returns true when the given derived word (an
// expression from a definitions notebook, e.g. "egomaniac") has a recent
// non-misunderstood log in EITHER LearnedLogs or ReverseLogs that is
// still within its SR interval. The etymology PDF uses this to suppress
// words the user has already mastered — re-reading a known word's
// definition adds noise to a study output that exists for the user to
// drill the underlying origins.
//
// sessionTitle matches the learning-history top-level title (the
// post-migration shape — see Validator.migrateEtymologyShape). The
// scene structure under the session is scanned in full because the
// caller doesn't know which scene a given expression lives in, and
// definitions-side scene titles don't necessarily mirror learning-
// history scene titles (notebook-side scenes are user-defined, e.g.
// "__index_0").
// readSessionConceptsAndRelations reads concepts: and relations: from a
// single etymology session YAML file. Returns empty slices when the
// file doesn't declare either, so callers can use len() without nil
// checks. Read errors are swallowed and treated as no-data: the
// validator at load time already reports malformed session files.
func readSessionConceptsAndRelations(path string) ([]Concept, []Relation) {
	wrapped, err := readYamlFile[etymologySessionFile](path)
	if err != nil {
		return nil, nil
	}
	return wrapped.Concepts, wrapped.Relations
}

// buildTemplateConcepts converts session-scoped Concept / Relation
// declarations into the template-friendly assets.EtymologyConcept slice
// that the etymology-notebook template renders. Relations are resolved
// for each concept so the template can print "antonym: rightness" next
// to "leftness" without traversing the relation list. Both directed
// (from/to) and symmetric (between) relation shapes are handled.
func buildTemplateConcepts(concepts []Concept, relations []Relation, originMeaning map[string]string) []assets.EtymologyConcept {
	if len(concepts) == 0 {
		return nil
	}
	relationsByConcept := make(map[string][]assets.EtymologyConceptRelation, len(concepts))
	add := func(key string, rel assets.EtymologyConceptRelation) {
		if key == "" || rel.Other == "" || rel.Other == key {
			return
		}
		relationsByConcept[key] = append(relationsByConcept[key], rel)
	}
	for _, r := range relations {
		if strings.TrimSpace(r.Type) == "" {
			continue
		}
		if r.IsDirected() {
			add(r.From, assets.EtymologyConceptRelation{Type: r.Type, Other: r.To})
			continue
		}
		if len(r.Between) == 2 {
			a, b := r.Between[0], r.Between[1]
			add(a, assets.EtymologyConceptRelation{Type: r.Type, Other: b})
			add(b, assets.EtymologyConceptRelation{Type: r.Type, Other: a})
		}
	}
	out := make([]assets.EtymologyConcept, 0, len(concepts))
	for _, c := range concepts {
		members := make([]assets.EtymologyConceptMember, 0, len(c.Members))
		for _, m := range c.Members {
			members = append(members, assets.EtymologyConceptMember{
				Origin:   m.Origin,
				Language: m.Language,
				Meaning:  originMeaning[m.Origin],
			})
		}
		out = append(out, assets.EtymologyConcept{
			Key:       c.Key,
			Meaning:   c.Meaning,
			Note:      c.Note,
			Members:   members,
			Relations: relationsByConcept[c.Key],
		})
	}
	return out
}

// expressionIsSkipped reports whether the user has explicitly skipped
// the given derived word from any vocabulary quiz mode. Mirrors the
// session-title lookup pattern in expressionRecentlyLearned (matches
// the learning-history's session entry, then walks every scene because
// scene titles aren't reliable identifiers across the
// definitions/learning-history shapes). Used to gate the etymology
// PDF / markdown so a skipped word doesn't reappear in the printable
// study material.
func (writer EtymologyNotebookWriter) expressionIsSkipped(etymID, sessionTitle, expression string) bool {
	histories := writer.learningHistories[etymID]
	if len(histories) == 0 {
		return false
	}
	for _, h := range histories {
		if h.Metadata.Title != sessionTitle {
			continue
		}
		for _, scene := range h.Scenes {
			for _, expr := range scene.Expressions {
				if !strings.EqualFold(expr.Expression, expression) {
					continue
				}
				return expr.SkippedAt.IsSkippedAny()
			}
		}
	}
	return false
}

func (writer EtymologyNotebookWriter) expressionRecentlyLearned(etymID, sessionTitle, expression string) bool {
	histories := writer.learningHistories[etymID]
	if len(histories) == 0 {
		return false
	}
	for _, h := range histories {
		if h.Metadata.Title != sessionTitle {
			continue
		}
		for _, scene := range h.Scenes {
			for _, expr := range scene.Expressions {
				if !strings.EqualFold(expr.Expression, expression) {
					continue
				}
				if hasRecentCorrectLog(expr.LearnedLogs, GetThresholdDaysFromCount(correctStreakCount(expr.LearnedLogs))) {
					return true
				}
				if hasRecentCorrectLog(expr.ReverseLogs, GetThresholdDaysFromCount(correctStreakCount(expr.ReverseLogs))) {
					return true
				}
				return false
			}
		}
	}
	return false
}

// latestEtymologyLog returns the single most-recent log across both etymology
// tracks (breakdown for standard/freeform, assembly for reverse). The bool
// is false when neither slice has any entries.
func latestEtymologyLog(breakdown, assembly []LearningRecord) (LearningRecord, bool) {
	var latest LearningRecord
	found := false
	consider := func(logs []LearningRecord) {
		for _, l := range logs {
			if !found || l.LearnedAt.After(latest.LearnedAt.Time) {
				latest = l
				found = true
			}
		}
	}
	consider(breakdown)
	consider(assembly)
	return latest, found
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
func readDefinitionsFileChapters(
	defDir, sessionFilename string,
	originMap map[string]string,
	needsStudy func(origin string) bool,
	wordHasBeenLearned func(expression string) bool,
	wordIsSkipped func(expression string) bool,
	conceptByExpression map[string]string,
	conceptByHead map[string]DefinitionConceptInfo,
) []assets.EtymologyChapter {
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
			memberDetails := buildConceptMemberDetails(scene.Expressions, conceptByExpression, conceptByHead)
			seenConceptHead := make(map[string]int) // head -> index in sceneWords
			var sceneWords []assets.EtymologyWordEntry
			for _, note := range scene.Expressions {
				if !wordNeedsStudy(note.OriginParts, needsStudy) {
					continue
				}
				// Also skip words the user already knows. Even when
				// the word's origins still need work, re-reading a
				// known word's definition doesn't help drill the
				// origin; the origins block at the top of the chapter
				// covers that on its own. Mirrors the vocabulary
				// PDF's needsToLearnInNotebook semantic: "known" iff
				// either direction has a recent non-misunderstood log
				// within its SR interval.
				if wordHasBeenLearned != nil && wordHasBeenLearned(note.Expression) {
					continue
				}
				if wordIsSkipped != nil && wordIsSkipped(note.Expression) {
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

				// Concept collapse: when this note is a concept member,
				// fold it into one EtymologyWordEntry per concept (the
				// head's row when seen, otherwise the first encountered
				// member; upgraded if the head shows up later).
				head, isMember := "", false
				if conceptByExpression != nil {
					head, isMember = conceptByExpression[note.Expression]
					if !isMember && note.Definition != "" {
						head, isMember = conceptByExpression[note.Definition]
					}
				}
				if isMember {
					info := conceptByHead[head]
					word.ConceptHead = head
					word.ConceptMeaning = info.Meaning
					word.ConceptMembers = memberDetails[head]
					if existingIdx, already := seenConceptHead[head]; already {
						if note.Expression == head || note.Definition == head {
							sceneWords[existingIdx] = word
						}
						continue
					}
					seenConceptHead[head] = len(sceneWords)
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
