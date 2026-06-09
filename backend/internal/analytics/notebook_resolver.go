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

func (r *NotebookMetadataResolver) resolveVocab(notebookID, expression string) WordMetadata {
	// Stories carry definitions inside scenes; flashcards carry them on the
	// notebook directly. Try both because the same notebook ID can resolve
	// in either index depending on how the source declares it.
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
	return WordMetadata{}
}

func matchVocabNote(notes []notebook.Note, expression string) (WordMetadata, bool) {
	for _, n := range notes {
		if n.Expression != expression && n.Definition != expression {
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

func (r *NotebookMetadataResolver) resolveOrigin(notebookID, expression string) WordMetadata {
	origins, err := r.reader.ReadEtymologyNotebook(notebookID)
	if err != nil {
		return WordMetadata{}
	}
	target := strings.TrimSpace(expression)
	for _, o := range origins {
		if o.Origin != target {
			continue
		}
		return WordMetadata{Meaning: o.Meaning, NotebookKind: "etymology"}
	}
	return WordMetadata{}
}
