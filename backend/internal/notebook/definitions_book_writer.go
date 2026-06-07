package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/at-ishikawa/langner/internal/assets"
	"github.com/at-ishikawa/langner/internal/pdf"
)

// DefinitionsBookWriter renders a definitions-only book (e.g. Word Power
// Made Easy) to markdown / PDF. Stories, books, and flashcards each have
// their own writer because their input shapes differ; a definitions book
// has neither conversations nor a card list — its only content is the
// per-scene expression list, optionally grouped by a concepts: block.
//
// Output reuses the story template (one "definition list per scene" is
// the natural fit) with empty conversations and statements. Concept
// collapse is identical to the story writer's path: members fold into
// one row per concept, with the head's display fields and a "Family:"
// line listing every member.
type DefinitionsBookWriter struct {
	reader       *Reader
	templatePath string
}

// NewDefinitionsBookWriter constructs the writer. templatePath is the
// story-notebook template (the definitions book reuses it intentionally).
func NewDefinitionsBookWriter(reader *Reader, templatePath string) *DefinitionsBookWriter {
	return &DefinitionsBookWriter{reader: reader, templatePath: templatePath}
}

// OutputDefinitionsBook writes the markdown for a single definitions
// book and, if generatePDF is true, converts it to PDF via the same
// pipeline the story / flashcard writers use.
//
// Each Definitions session becomes one notebook entry; each scene
// becomes one section under the session header. Notes whose expression
// belongs to a concept (per the book's concepts: declarations) are
// collapsed into one row per concept; standalone notes pass through.
func (w DefinitionsBookWriter) OutputDefinitionsBook(bookID string, outputDirectory string, generatePDF bool) error {
	defs, ok := w.reader.GetDefinitionsBook(bookID)
	if !ok || len(defs) == 0 {
		return fmt.Errorf("no definitions book found for %q", bookID)
	}

	byExpr, byHead := w.reader.GetDefinitionsBookConceptInfo(bookID)

	// Build one assets.StoryNotebook per session. Scenes carry the per-
	// scene expression list as assets.StoryNote entries; the converter
	// collapses concept members within each scene the same way the
	// story writer does.
	storyNotebooks := make([]assets.StoryNotebook, 0, len(defs))
	for _, def := range defs {
		title := def.Metadata.Title
		if title == "" {
			title = def.Metadata.Notebook
		}
		assetsScenes := make([]assets.StoryScene, 0, len(def.Scenes))
		for _, scene := range def.Scenes {
			notes := collapseDefinitionConceptsForExport(scene.Expressions, byExpr, byHead)
			if len(notes) == 0 {
				continue
			}
			assetsScenes = append(assetsScenes, assets.StoryScene{
				Title:       scene.Metadata.Title,
				Definitions: notes,
			})
		}
		storyNotebooks = append(storyNotebooks, assets.StoryNotebook{
			Event:  title,
			Scenes: assetsScenes,
			Date:   def.Metadata.Date,
		})
	}

	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		return fmt.Errorf("os.MkdirAll(%s): %w", outputDirectory, err)
	}
	outputFilename := strings.TrimSpace(filepath.Join(outputDirectory, bookID+".md"))
	output, err := os.Create(outputFilename)
	if err != nil {
		return fmt.Errorf("os.Create(%s): %w", outputFilename, err)
	}
	defer func() { _ = output.Close() }()

	templateData := assets.StoryTemplate{Notebooks: storyNotebooks}
	if err := assets.WriteStoryNotebook(output, w.templatePath, templateData); err != nil {
		return fmt.Errorf("assets.WriteStoryNotebook(%s, %s): %w", outputFilename, w.templatePath, err)
	}
	fmt.Printf("Definitions book written to: %s\n", outputFilename)

	if generatePDF {
		pdfPath, err := pdf.ConvertMarkdownToPDF(outputFilename)
		if err != nil {
			return fmt.Errorf("ConvertMarkdownToPDF(%s): %w", outputFilename, err)
		}
		fmt.Printf("PDF generated at: %s\n", pdfPath)
	}
	return nil
}

// collapseDefinitionConceptsForExport converts a scene's []Note into
// assets.StoryNote entries with concept member groups collapsed to one
// row per concept. byExpression maps each member expression to its head;
// byHead carries the full concept declaration. When both maps are empty
// the behaviour is a straight 1:1 conversion.
func collapseDefinitionConceptsForExport(
	notes []Note,
	byExpression map[string]string,
	byHead map[string]DefinitionConceptInfo,
) []assets.StoryNote {
	out := make([]assets.StoryNote, 0, len(notes))
	seenHead := make(map[string]int) // head -> index in out
	memberDetails := buildConceptMemberDetails(notes, byExpression, byHead)

	for _, note := range notes {
		entry := assets.StoryNote{
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
		head, isMember := byExpression[note.Expression]
		if !isMember && note.Definition != "" {
			head, isMember = byExpression[note.Definition]
		}
		if !isMember {
			out = append(out, entry)
			continue
		}
		info := byHead[head]
		entry.ConceptHead = head
		entry.ConceptMembers = memberDetails[head]
		entry.ConceptMeaning = info.Meaning
		if existingIdx, already := seenHead[head]; already {
			// Already emitted a member; upgrade to the head's row when
			// we encounter it (more accurate display data).
			if note.Expression == head || note.Definition == head {
				out[existingIdx] = entry
			}
			continue
		}
		seenHead[head] = len(out)
		out = append(out, entry)
	}
	return out
}
