package analytics

import (
	"context"
	"regexp"
	"sort"
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
// The quiz type drives the lookup path so that words colliding between
// the vocabulary side and the etymology-origin side (e.g. "gauche" the
// English adjective and "gauche" the French origin) resolve correctly:
//
//   - etymology_* quizzes always go through resolveOrigin.
//   - vocabulary quizzes (notebook / reverse / freeform) always go
//     through resolveVocab, even when the underlying learning-history
//     record carries `type: origin` (a known cross-recording artifact
//     for words that happen to be both an English word and an origin).
//
// expressionType is used as a fallback only when the quiz type is
// missing (legacy callers / no-op resolver).
func (r *NotebookMetadataResolver) Resolve(_ context.Context, notebookID, expression, expressionType, quizType string) WordMetadata {
	if r == nil || r.reader == nil || notebookID == "" || expression == "" {
		return WordMetadata{}
	}
	if quizType != "" {
		if isEtymologyQuizType(quizType) {
			return r.resolveOrigin(notebookID, expression)
		}
		return r.resolveVocab(notebookID, expression)
	}
	if expressionType == notebook.LearningExpressionTypeOrigin {
		return r.resolveOrigin(notebookID, expression)
	}
	return r.resolveVocab(notebookID, expression)
}

// isEtymologyQuizType identifies the quiz types whose attempt records
// should resolve against the etymology side of a notebook (origin
// meaning). Vocabulary quiz types (notebook / reverse / freeform)
// always resolve against the vocabulary side. Keeping the check on the
// quiz type — not on the expression — means an English word that also
// happens to be an etymology origin still returns the English meaning
// after a vocabulary-quiz failure.
func isEtymologyQuizType(quizType string) bool {
	return strings.HasPrefix(quizType, "etymology_")
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
				if meta, note, ok := findVocabNote(scene.Definitions, target); ok {
					meta.NotebookKind = "story"
					if meta.ExampleSentence == "" {
						// Story-style notebooks (Speak English Like an American,
						// Friends, etc.) rarely carry per-note `examples:` data —
						// the in-context usage is the conversation itself. Pull
						// the first conversation quote / statement that mentions
						// the expression so the analytics card shows a real
						// usage line instead of nothing. Both forms (canonical
						// expression and the definition alias, e.g. "stuffed
						// shirts" vs "stuffed shirt") are tried so quotes that
						// use a plural / conjugated variant still match.
						meta.ExampleSentence = findUsageInScene(scene, target, lookupDefinitionAlias(scene.Definitions, target))
					}
					meta.RelatedGroups = r.computeVocabRelatedGroups(notebookID, note)
					return meta
				}
			}
		}
	}
	if flashcards, err := r.reader.ReadFlashcardNotebooks(notebookID); err == nil {
		for _, fc := range flashcards {
			if meta, note, ok := findVocabNote(fc.Cards, target); ok {
				meta.NotebookKind = "flashcard"
				meta.RelatedGroups = r.computeVocabRelatedGroups(notebookID, note)
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
				if meta, note, ok := findVocabNote(sceneNotes, target); ok {
					// Definitions get merged into the story reader view at
					// runtime, so a "story" kind here lands the deep link on
					// /learn/{id} where the word will be highlighted.
					meta.NotebookKind = "story"
					meta.RelatedGroups = r.computeVocabRelatedGroups(notebookID, note)
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

// findUsageInScene returns the first conversation quote (or, failing
// that, statement) in the scene that mentions the expression. The match
// tries, in order:
//
//   - exact (case-insensitive) substring of the expression itself
//   - exact substring of the definition alias (handles "lose one's
//     temper" stored as both expression and definition)
//   - any of the expression's significant content tokens (stopwords and
//     possessive markers stripped), to absorb conjugated / pluralised
//     forms ("losing my temper" matching "lose one's temper" via the
//     shared "temper" stem)
//
// Empty when nothing matches — the caller leaves ExampleSentence empty
// rather than fabricating a usage.
func findUsageInScene(scene notebook.StoryScene, expression, alias string) string {
	phrase := strings.ToLower(strings.TrimSpace(expression))
	aliasLow := strings.ToLower(strings.TrimSpace(alias))
	phrases := []string{phrase}
	if aliasLow != "" && aliasLow != phrase {
		phrases = append(phrases, aliasLow)
	}
	tokens := significantTokens(phrase + " " + aliasLow)
	containsAny := func(haystack string) bool {
		low := strings.ToLower(haystack)
		for _, needle := range phrases {
			if needle != "" && strings.Contains(low, needle) {
				return true
			}
		}
		for _, t := range tokens {
			if strings.Contains(low, t) {
				return true
			}
		}
		return false
	}
	for _, conv := range scene.Conversations {
		if containsAny(conv.Quote) {
			return cleanExampleSentence(conv.Quote)
		}
	}
	for _, stmt := range scene.Statements {
		if containsAny(stmt) {
			return cleanExampleSentence(stmt)
		}
	}
	return ""
}

// cleanExampleSentence strips the `{{ ... }}` highlight markers that
// MergeDefinitionsIntoNotebooks wraps around recognised expressions in
// scene statements. The markers are a rendering hint for the Learn UI;
// in the analytics card they read as noise.
func cleanExampleSentence(s string) string {
	s = strings.TrimSpace(s)
	s = exampleMarkerOpen.ReplaceAllString(s, "")
	s = exampleMarkerClose.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

var exampleMarkerOpen = regexp.MustCompile(`\{\{\s*`)
var exampleMarkerClose = regexp.MustCompile(`\s*\}\}`)

// significantTokens splits the expression into lowercase content tokens,
// drops articles / prepositions / possessive markers and anything under
// 4 characters, and returns the remainder. The list is ordered by token
// length descending so the longest (and therefore most discriminating)
// stem is tried first.
func significantTokens(phrase string) []string {
	stop := map[string]bool{
		"a": true, "an": true, "the": true, "to": true, "of": true, "in": true,
		"on": true, "at": true, "by": true, "for": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "do": true, "does": true,
		"did": true, "and": true, "or": true, "but": true, "with": true, "from": true,
		"my": true, "his": true, "her": true, "your": true, "their": true, "our": true,
		"its": true, "one's": true, "someone's": true, "one": true, "someone": true,
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range strings.Fields(phrase) {
		t := strings.Trim(strings.ToLower(raw), ".,!?;:\"'()[]{}")
		if len(t) < 4 || stop[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// lookupDefinitionAlias returns the `definition` field of the note that
// matches the expression. Used by findUsageInScene to widen substring
// search to the dictionary-form alias when the conversation uses a
// conjugated / pluralised form.
func lookupDefinitionAlias(notes []notebook.Note, expression string) string {
	for _, n := range notes {
		if matchExpression(n.Expression, n.Definition, expression) {
			return n.Definition
		}
	}
	return ""
}

// findVocabNote returns the matched Note alongside the meta so callers
// can read origin_parts (used to compute the etymology side of the
// analytics card's Related Words block). The boolean is true when a
// matching note was found.
func findVocabNote(notes []notebook.Note, expression string) (WordMetadata, notebook.Note, bool) {
	for _, n := range notes {
		if !matchExpression(n.Expression, n.Definition, expression) {
			continue
		}
		meta := WordMetadata{Meaning: n.Meaning}
		if len(n.Examples) > 0 {
			meta.ExampleSentence = n.Examples[0]
		}
		return meta, n, true
	}
	return WordMetadata{}, notebook.Note{}, false
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
			return WordMetadata{
				Meaning:       o.Meaning,
				NotebookKind:  "etymology",
				RelatedGroups: r.computeOriginRelatedGroups(notebookID, o.Origin),
			}
		}
	}
	return WordMetadata{}
}
