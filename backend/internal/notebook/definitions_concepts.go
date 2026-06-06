package notebook

import (
	"fmt"
	"sort"
	"strings"
)

// definitionConceptDecl records a concept declaration from a specific
// session within a book, for error reporting.
type definitionConceptDecl struct {
	concept DefinitionConcept
	bookID  string
	session string
}

// definitionBookView aggregates the parsed concept declarations from every
// session file of a single definitions book, plus the set of expressions
// available for member resolution. Used to validate book-level invariants:
//
//   - heads are unique across the book (the head doubles as the DB key);
//   - the same head re-declared in another session agrees on meaning;
//   - every listed expression resolves to an actual Note in the book;
//   - the head is itself listed in expressions[];
//   - each expression belongs to at most one concept.
type definitionBookView struct {
	bookID string

	// expressions is the set of expression strings (and definition aliases)
	// found in this book's Notes. Used to verify concept member resolution.
	expressions map[string]bool

	// concepts indexes every concept declaration by head. Multiple entries
	// per head are expected when the same concept is referenced from more
	// than one session; the validator checks they agree on meaning.
	concepts map[string][]definitionConceptDecl
}

// newDefinitionBookView constructs the per-book view from the parsed
// Definitions slice (one entry per session) returned by NewDefinitionsMap.
// SessionTitle is propagated from each parent Definitions.Metadata onto its
// embedded DefinitionConcept entries so downstream callers (validator,
// ingestion) can report the source session.
func newDefinitionBookView(bookID string, defs []Definitions) *definitionBookView {
	view := &definitionBookView{
		bookID:      bookID,
		expressions: make(map[string]bool),
		concepts:    make(map[string][]definitionConceptDecl),
	}
	for _, def := range defs {
		session := strings.TrimSpace(def.Metadata.Title)
		if session == "" {
			session = strings.TrimSpace(def.Metadata.Notebook)
		}
		for _, scene := range def.Scenes {
			for _, note := range scene.Expressions {
				if expr := strings.TrimSpace(note.Expression); expr != "" {
					view.expressions[expr] = true
				}
				if defn := strings.TrimSpace(note.Definition); defn != "" {
					view.expressions[defn] = true
				}
			}
		}
		for _, c := range def.Concepts {
			c.SessionTitle = session
			view.concepts[c.Head] = append(view.concepts[c.Head], definitionConceptDecl{
				concept: c,
				bookID:  bookID,
				session: session,
			})
		}
	}
	return view
}

// validateDefinitionConcepts validates the new concepts: block on
// definitions session files. Warn-only while the feature matures: existing
// books without concepts pass cleanly, and a malformed concepts block does
// not block --fix or other validator passes.
func (v *Validator) validateDefinitionConcepts(result *ValidationResult) {
	if len(v.definitionsDirs) == 0 {
		return
	}
	_, raw, _, err := NewDefinitionsMap(v.definitionsDirs)
	if err != nil {
		result.AddWarning(ValidationError{
			Message: fmt.Sprintf("load definitions for concept validation: %v", err),
		})
		return
	}
	bookIDs := make([]string, 0, len(raw))
	for id := range raw {
		bookIDs = append(bookIDs, id)
	}
	sort.Strings(bookIDs)
	for _, bookID := range bookIDs {
		view := newDefinitionBookView(bookID, raw[bookID])
		v.validateBookDefinitionConcepts(view, result)
	}
}

// validateBookDefinitionConcepts checks book-level concept invariants on a
// single definitions book. All findings are warnings.
func (v *Validator) validateBookDefinitionConcepts(view *definitionBookView, result *ValidationResult) {
	heads := make([]string, 0, len(view.concepts))
	for h := range view.concepts {
		heads = append(heads, h)
	}
	sort.Strings(heads)

	// Per-head pass: meaning agreement, head-in-members, member resolution.
	for _, head := range heads {
		decls := view.concepts[head]
		if strings.TrimSpace(head) == "" {
			for _, d := range decls {
				result.AddWarning(ValidationError{
					File:     view.bookID,
					Location: fmt.Sprintf("session %q", d.session),
					Message:  "concept.head must be non-empty",
				})
			}
			continue
		}

		var meaning, firstSession string
		for i, d := range decls {
			if strings.TrimSpace(d.concept.Meaning) == "" {
				result.AddWarning(ValidationError{
					File:     view.bookID,
					Location: fmt.Sprintf("session %q, concept %q", d.session, head),
					Message:  "concept.meaning must be non-empty",
				})
			}
			if len(d.concept.Expressions) == 0 {
				result.AddWarning(ValidationError{
					File:     view.bookID,
					Location: fmt.Sprintf("session %q, concept %q", d.session, head),
					Message:  "concept.expressions must list at least one member",
				})
			}
			// kind=family is the only kind that triggers SR consolidation;
			// the others (synonym/antonym/visualization) are display-only.
			// Empty kind silently defaults to family (every pre-flag
			// concept assumes that semantics). We only warn on truly
			// unrecognised values so typos surface but legacy concepts
			// don't generate validator noise.
			switch d.concept.Kind {
			case "", ConceptKindFamily, ConceptKindSynonym, ConceptKindAntonym, ConceptKindVisualization:
				// known and valid (empty defaults to family)
			default:
				result.AddWarning(ValidationError{
					File:     view.bookID,
					Location: fmt.Sprintf("session %q, concept %q", d.session, head),
					Message: fmt.Sprintf(
						"concept.kind %q is not recognised; expected one of family, synonym, antonym, visualization",
						d.concept.Kind,
					),
				})
			}

			headInMembers := false
			for _, expr := range d.concept.Expressions {
				if strings.TrimSpace(expr) == head {
					headInMembers = true
					break
				}
			}
			if !headInMembers && len(d.concept.Expressions) > 0 {
				result.AddWarning(ValidationError{
					File:     view.bookID,
					Location: fmt.Sprintf("session %q, concept %q", d.session, head),
					Message:  fmt.Sprintf("concept.head %q must appear in its own expressions[]", head),
				})
			}

			for _, expr := range d.concept.Expressions {
				name := strings.TrimSpace(expr)
				if name == "" {
					continue
				}
				if !view.expressions[name] {
					result.AddWarning(ValidationError{
						File:     view.bookID,
						Location: fmt.Sprintf("session %q, concept %q", d.session, head),
						Message: fmt.Sprintf(
							"concept member %q does not match any expression declared in book %q",
							name, view.bookID,
						),
					})
				}
			}

			if i == 0 {
				meaning = d.concept.Meaning
				firstSession = d.session
				continue
			}
			if d.concept.Meaning != meaning {
				result.AddWarning(ValidationError{
					File:     view.bookID,
					Location: fmt.Sprintf("session %q, concept %q", d.session, head),
					Message: fmt.Sprintf(
						"concept.meaning %q disagrees with earlier declaration %q (session %q)",
						d.concept.Meaning, meaning, firstSession,
					),
				})
			}
		}
	}

	// Cross-concept pass: each expression must belong to at most one
	// concept. We walk in sorted-head order so error messages are stable
	// across runs (map iteration in Go is randomised).
	owners := make(map[string]string)
	for _, head := range heads {
		if strings.TrimSpace(head) == "" {
			continue
		}
		for _, d := range view.concepts[head] {
			for _, expr := range d.concept.Expressions {
				name := strings.TrimSpace(expr)
				if name == "" {
					continue
				}
				prev, exists := owners[name]
				if !exists {
					owners[name] = head
					continue
				}
				if prev != head {
					result.AddWarning(ValidationError{
						File:     view.bookID,
						Location: fmt.Sprintf("session %q, concept %q", d.session, head),
						Message: fmt.Sprintf(
							"expression %q belongs to multiple concepts %q and %q (each expression must belong to at most one)",
							name, prev, head,
						),
					})
				}
			}
		}
	}
}
