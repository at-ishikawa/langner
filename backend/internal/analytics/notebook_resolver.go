package analytics

import (
	"context"
	"strings"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// NotebookMetadataResolver answers WrongWord metadata lookups by walking
// the source notebooks via notebook.Reader. The reader already caches
// directory indexes internally; this type adds a tiny per-call walk
// over the matching notebook's definitions, which is fine for the
// small number of wrong words a single day produces.
type NotebookMetadataResolver struct {
	reader *notebook.Reader
}

// NewNotebookMetadataResolver returns a resolver backed by the given
// reader. Pass nil to disable the lookups (the YAML repo then falls
// back to empty metadata).
func NewNotebookMetadataResolver(reader *notebook.Reader) MetadataResolver {
	if reader == nil {
		return NoMetadataResolver()
	}
	return &NotebookMetadataResolver{reader: reader}
}

// Resolve looks up the meaning and one example for the given expression.
// The expression type chooses the lookup path: etymology origins are
// pulled from etymology session files, everything else from story or
// flashcard definitions.
func (r *NotebookMetadataResolver) Resolve(_ context.Context, notebookID, expression, expressionType string) WordMetadata {
	if r == nil || r.reader == nil || notebookID == "" || expression == "" {
		return WordMetadata{}
	}
	if expressionType == notebook.LearningExpressionTypeOrigin {
		return r.resolveOrigin(notebookID, expression)
	}
	return r.resolveVocab(notebookID, expression)
}

// resolveVocab tries every notebook source a vocab definition might live in.
// In a Word-Power-Made-Easy-style setup the same notebookID can sit in
// definitions_directories (or as embedded definitions in a legacy etymology
// session) rather than in stories_directories / flashcards_directories, so
// the resolver has to walk all four. Matching is case-insensitive as a
// defensive fallback because the YAML expression can drift in case from the
// learning-history record.
func (r *NotebookMetadataResolver) resolveVocab(notebookID, expression string) WordMetadata {
	target := strings.TrimSpace(expression)

	if stories, err := r.reader.ReadStoryNotebooks(notebookID); err == nil {
		for _, s := range stories {
			for _, scene := range s.Scenes {
				if meta, ok := matchVocabNote(scene.Definitions, target); ok {
					meta.NotebookKind = "story"
					return meta
				}
			}
		}
	}
	if flashcards, err := r.reader.ReadFlashcardNotebooks(notebookID); err == nil {
		for _, fc := range flashcards {
			if meta, ok := matchVocabNote(fc.Cards, target); ok {
				meta.NotebookKind = "flashcard"
				return meta
			}
		}
	}
	// definitions_directories: definitions notebooks (the typical Word
	// Power Made Easy layout — words live in a separate definitions file
	// that the source notebook never sees directly).
	if defs, ok := r.reader.GetDefinitionsNotes(notebookID); ok {
		for _, sessionDefs := range defs {
			for _, sceneNotes := range sessionDefs {
				if meta, ok := matchVocabNote(sceneNotes, target); ok {
					// Definitions get merged into the story reader view at
					// runtime, so a "story" kind here lands the deep link on
					// /learn/{id} where the word will be highlighted.
					meta.NotebookKind = "story"
					return meta
				}
			}
		}
	}
	// Legacy etymology session files with inline `definitions:` (pre-
	// new-shape data). The notebook name on each entry is the etymology
	// index's display Name (a long-standing inconsistency in the
	// reader); look up by ID first to get that Name and then filter.
	if name := r.etymologyNotebookName(notebookID); name != "" {
		for _, def := range r.reader.ReadAllEtymologyDefinitions() {
			if def.NotebookName != name {
				continue
			}
			if !matchExpression(def.Expression, def.Definition, target) {
				continue
			}
			return WordMetadata{Meaning: def.Meaning, NotebookKind: "etymology"}
		}
	}
	return WordMetadata{}
}

// etymologyNotebookName returns the display Name for an etymology index by
// its ID. ReadAllEtymologyDefinitions tags each definition with that Name,
// so the resolver needs the name to filter back to a single notebook.
func (r *NotebookMetadataResolver) etymologyNotebookName(notebookID string) string {
	for id, idx := range r.reader.GetEtymologyIndexes() {
		if id == notebookID {
			return idx.Name
		}
	}
	return ""
}

func matchVocabNote(notes []notebook.Note, expression string) (WordMetadata, bool) {
	for _, n := range notes {
		if !matchExpression(n.Expression, n.Definition, expression) {
			continue
		}
		meta := WordMetadata{Meaning: n.Meaning}
		if len(n.Examples) > 0 {
			meta.ExampleSentence = n.Examples[0]
		}
		return meta, true
	}
	return WordMetadata{}, false
}

// matchExpression compares the target against both the canonical expression
// and the optional definition (the dictionary-form alias) field. Comparison
// is exact first, then case-insensitive to absorb stale-case learning-
// history records.
func matchExpression(expr, definition, target string) bool {
	if expr == target || definition == target {
		return true
	}
	low := strings.ToLower(target)
	return strings.ToLower(expr) == low || strings.ToLower(definition) == low
}

func (r *NotebookMetadataResolver) resolveOrigin(notebookID, expression string) WordMetadata {
	origins, err := r.reader.ReadEtymologyNotebook(notebookID)
	if err != nil {
		return WordMetadata{}
	}
	target := strings.TrimSpace(expression)
	low := strings.ToLower(target)
	for _, o := range origins {
		if o.Origin == target || strings.ToLower(o.Origin) == low {
			return WordMetadata{Meaning: o.Meaning, NotebookKind: "etymology"}
		}
	}
	return WordMetadata{}
}
