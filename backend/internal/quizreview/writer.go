// Package quizreview renders a one-day "what did I get wrong" report
// per source notebook. The output mirrors the regular study notebook
// shape (top heading, per-session subsections, one entry per failed
// expression) but only carries the words / origins the user got wrong
// on the requested date — so it can be re-read alongside the original
// notebook without re-skimming material the user already knows.
package quizreview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/analytics"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/pdf"
)

// QuizReviewWriter renders the failed quiz attempts on a single day as a
// per-notebook markdown file (and optional PDF) shaped like the regular
// study notebooks: top heading, one section per source session / lesson,
// and an entry per failed origin or vocabulary with its meaning,
// example, and the concept-graph context the analytics card surfaces.
// The output is meant to be re-read alongside the original notebook so
// the user can drill exactly the words / origins they got wrong that
// day without re-reading the entire notebook.
type Writer struct {
	repo   analytics.Repository
	source SourceContent
}

// SourceContent supplies the per-session source-notebook content the
// writer interleaves with each session's failure list — the
// conversation dialogue from a story notebook, the concept declarations
// and relations from an etymology notebook. Production code wraps a
// notebook.Reader via NewReaderSource; tests pass a stub so the
// markdown rendering can be exercised without spinning up the YAML
// reader. A nil source is allowed and produces the legacy
// failure-only layout.
type SourceContent interface {
	// StoryConversations returns the dialogue scenes declared in the
	// notebook's matching session (StoryNotebook.Event == sessionTitle).
	// Each returned slice corresponds to one scene's `conversations:`
	// block; statements come back joined as a single pseudo-quote so
	// callers don't need to know about that distinction.
	StoryConversations(notebookID, sessionTitle string) []SourceScene
	// EtymologyConcepts returns the concepts + relations declared in
	// the etymology session of the same title plus a map from origin
	// name to meaning so the rendered member rows can show
	// "sinister (Latin) — left hand" without re-walking the origins.
	EtymologyConcepts(notebookID, sessionTitle string) ([]notebook.Concept, []notebook.Relation, map[string]string)
}

// SourceScene is one scene's worth of dialogue from a story notebook,
// flattened to lines so the writer can render it as a markdown
// blockquote without re-traversing speaker/quote pairs.
type SourceScene struct {
	// Title is the scene's narrative summary (typically the multi-line
	// description in YAML). Empty for flashcard-style scenes.
	Title string
	Lines []SourceLine
}

// SourceLine is one speaker + quote pair OR one statement (in which
// case Speaker is "").
type SourceLine struct {
	Speaker string
	Quote   string
}

// NewWriter constructs a writer with no source content — failures
// render as a flat list. Use NewWriterWithSource to also interleave
// source dialogue / concept blocks.
func NewWriter(repo analytics.Repository) *Writer {
	return &Writer{repo: repo}
}

// NewWriterWithSource constructs a writer that pulls per-session
// content (story conversations + etymology concepts) from the given
// SourceContent and renders it alongside the failure list. The
// repository must already be configured with its MetadataResolver so
// the WrongWords come pre-hydrated with meanings, examples and related
// groups.
func NewWriterWithSource(repo analytics.Repository, source SourceContent) *Writer {
	return &Writer{repo: repo, source: source}
}

// NewReaderSource wraps a notebook.Reader as a SourceContent so the
// CLI can hand the writer everything it needs in one shot. nil reader
// yields an empty source (no conversations, no concepts).
func NewReaderSource(reader *notebook.Reader) SourceContent {
	if reader == nil {
		return nil
	}
	return &readerSource{reader: reader}
}

type readerSource struct {
	reader *notebook.Reader
}

func (rs *readerSource) StoryConversations(notebookID, sessionTitle string) []SourceScene {
	stories, err := rs.reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil
	}
	for _, s := range stories {
		if s.Event != sessionTitle {
			continue
		}
		out := make([]SourceScene, 0, len(s.Scenes))
		for _, scene := range s.Scenes {
			if len(scene.Conversations) == 0 && len(scene.Statements) == 0 {
				continue
			}
			lines := make([]SourceLine, 0, len(scene.Conversations)+len(scene.Statements))
			for _, c := range scene.Conversations {
				lines = append(lines, SourceLine{Speaker: c.Speaker, Quote: c.Quote})
			}
			for _, st := range scene.Statements {
				lines = append(lines, SourceLine{Quote: st})
			}
			out = append(out, SourceScene{Title: scene.Title, Lines: lines})
		}
		return out
	}
	return nil
}

func (rs *readerSource) EtymologyConcepts(notebookID, sessionTitle string) ([]notebook.Concept, []notebook.Relation, map[string]string) {
	conceptsBySession, relationsBySession := rs.reader.GetEtymologyConceptsBySession(notebookID)
	concepts := conceptsBySession[sessionTitle]
	relations := relationsBySession[sessionTitle]
	if len(concepts) == 0 && len(relations) == 0 {
		return nil, nil, nil
	}
	origins, _ := rs.reader.ReadEtymologyNotebook(notebookID)
	meaning := make(map[string]string, len(origins))
	for _, o := range origins {
		if o.SessionTitle != "" && o.SessionTitle != sessionTitle {
			// Track origin meanings book-wide so concept members
			// declared in a different session still render with
			// their meaning text; the session filter here is only a
			// cheap fast path.
		}
		meaning[o.Origin] = o.Meaning
	}
	return concepts, relations, meaning
}

// Output writes a single markdown file covering every notebook with
// failures on the given day. The file lands at
// <outputDirectory>/quiz-review-<YYYY-MM-DD>.md. generatePDF
// additionally writes a PDF next to it.
//
// Returns the written markdown path (empty when the day had no wrong
// attempts) and an error if the analytics fetch or file write failed.
func (w *Writer) Output(ctx context.Context, day time.Time, outputDirectory string, generatePDF bool) (string, error) {
	if outputDirectory == "" {
		return "", fmt.Errorf("output directory is empty")
	}
	detail, err := w.repo.DayDetail(ctx, day, analytics.Filters{})
	if err != nil {
		return "", fmt.Errorf("repo.DayDetail: %w", err)
	}
	if len(detail.WrongWords) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", outputDirectory, err)
	}

	dateStr := day.Format("2006-01-02")
	filename := filepath.Join(outputDirectory, "quiz-review-"+dateStr+".md")
	body := renderQuizReviewAllNotebooks(dateStr, groupByNotebook(detail.WrongWords), w.source)
	if err := os.WriteFile(filename, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}
	if generatePDF {
		if _, err := pdf.ConvertMarkdownToPDF(filename); err != nil {
			return filename, fmt.Errorf("ConvertMarkdownToPDF(%s): %w", filename, err)
		}
	}
	return filename, nil
}

// quizReviewGroup is one notebook's slice of failed attempts on the day.
// notebookTitleByID retains the first NotebookTitle the analytics
// repository surfaced — typically the notebook's display name or, when
// the learning history has no top-level name, the first session title.
type quizReviewGroup struct {
	notebookID    string
	notebookTitle string
	notebookKind  string
	sessions      []quizReviewSession
}

// quizReviewSession groups failures within one notebook by session /
// lesson title. Vocabulary entries and origin entries are kept apart so
// the rendered file has clearly-labelled "Failed origins" /
// "Failed vocabularies" subsections.
type quizReviewSession struct {
	title  string
	vocab  []analytics.WrongWord
	origin []analytics.WrongWord
}

// groupByNotebook partitions the day's wrong words into notebook
// groups, preserving the relative order of first appearance (so the
// file list is stable for a given day). Within a notebook, sessions
// are sorted by first appearance and entries within a session are
// sorted by expression for a predictable diff-friendly markdown.
func groupByNotebook(wrongs []analytics.WrongWord) []quizReviewGroup {
	byID := make(map[string]int)
	var groups []quizReviewGroup
	sessionByKey := make(map[string]map[string]int)
	for _, w := range wrongs {
		idx, ok := byID[w.NotebookID]
		if !ok {
			groups = append(groups, quizReviewGroup{
				notebookID:    w.NotebookID,
				notebookTitle: w.NotebookTitle,
				notebookKind:  w.NotebookKind,
			})
			idx = len(groups) - 1
			byID[w.NotebookID] = idx
			sessionByKey[w.NotebookID] = make(map[string]int)
		}
		sessionTitle := w.NotebookTitle
		if sessionTitle == "" {
			sessionTitle = "(untitled session)"
		}
		sessions := sessionByKey[w.NotebookID]
		sIdx, ok := sessions[sessionTitle]
		if !ok {
			groups[idx].sessions = append(groups[idx].sessions, quizReviewSession{title: sessionTitle})
			sIdx = len(groups[idx].sessions) - 1
			sessions[sessionTitle] = sIdx
		}
		if isOriginQuizType(w.QuizType) {
			groups[idx].sessions[sIdx].origin = append(groups[idx].sessions[sIdx].origin, w)
		} else {
			groups[idx].sessions[sIdx].vocab = append(groups[idx].sessions[sIdx].vocab, w)
		}
	}
	for gi := range groups {
		for si := range groups[gi].sessions {
			sort.SliceStable(groups[gi].sessions[si].vocab, func(i, j int) bool {
				return groups[gi].sessions[si].vocab[i].Expression < groups[gi].sessions[si].vocab[j].Expression
			})
			sort.SliceStable(groups[gi].sessions[si].origin, func(i, j int) bool {
				return groups[gi].sessions[si].origin[i].Expression < groups[gi].sessions[si].origin[j].Expression
			})
		}
	}
	return groups
}

// renderQuizReviewAllNotebooks renders every notebook with failures on
// the day as a single markdown document. Notebooks are emitted in the
// order they first appeared in the analytics result; within each
// notebook, sessions follow the same first-appearance order. Each
// session is preceded by its source-notebook context (conversation
// dialogue for stories, concept blocks for etymology) so the reader
// gets the surrounding material a normal study notebook would carry,
// with the day's failures highlighted below.
func renderQuizReviewAllNotebooks(date string, groups []quizReviewGroup, source SourceContent) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Quiz review — %s\n\n", date)
	if total := totalEntriesAcross(groups); total > 0 {
		fmt.Fprintf(&sb, "%d wrong attempt%s across %d notebook%s.\n\n",
			total, plural(total), len(groups), plural(len(groups)))
	}
	for gi, g := range groups {
		if gi > 0 {
			sb.WriteString("\n---\n\n")
		}
		// Notebook display names aren't reliably available on the
		// WrongWord (NotebookTitle is the per-session title), so the
		// notebook ID is the only honest header here.
		fmt.Fprintf(&sb, "## %s\n\n", g.notebookID)
		if total := totalEntries(g); total > 0 {
			fmt.Fprintf(&sb, "%d wrong attempt%s across %d session%s.\n\n",
				total, plural(total), len(g.sessions), plural(len(g.sessions)))
		}
		for _, s := range g.sessions {
			fmt.Fprintf(&sb, "### %s\n\n", s.title)
			hasConversations := false
			if source != nil {
				hasConversations = writeStoryConversations(&sb,
					source.StoryConversations(g.notebookID, s.title), s.vocab)
				// Highlight matches on BOTH origin failures and vocab
				// failures: an English word can share its name with the
				// origin it descends from (`gauche` the adjective vs
				// `gauche` the French origin), and the user reading the
				// concept block should see ✗ on either case.
				highlights := failedNameSet(s.origin, s.vocab)
				concepts, relations, originMeanings := source.EtymologyConcepts(g.notebookID, s.title)
				writeEtymologyConcepts(&sb, concepts, relations, originMeanings, highlights)
			}
			if len(s.origin) > 0 {
				sb.WriteString("#### Failed origins\n\n")
				for _, w := range s.origin {
					writeEntry(&sb, w, hasConversations)
				}
			}
			if len(s.vocab) > 0 {
				sb.WriteString("#### Failed vocabularies\n\n")
				for _, w := range s.vocab {
					writeEntry(&sb, w, hasConversations)
				}
			}
		}
	}
	return sb.String()
}

// writeStoryConversations renders only the scenes that contain a
// failed expression, marks each matching line with a ✗ prefix, and
// bolds the failed expression inside the quote. Scenes without any
// failure are dropped — a full lesson's dialogue is too much to wade
// through when the user only failed one word in it. Returns true when
// at least one scene was written, so the caller can suppress the
// downstream Example: line in the vocab entry (the conversation block
// already showed it).
func writeStoryConversations(sb *strings.Builder, scenes []SourceScene, failures []analytics.WrongWord) bool {
	if len(scenes) == 0 || len(failures) == 0 {
		return false
	}
	var rendered []string
	for _, scene := range scenes {
		sceneOut := strings.Builder{}
		anyMatch := false
		for _, line := range scene.Lines {
			bolded, matched := boldFailedExpressions(line.Quote, failures)
			marker := ""
			if matched {
				marker = "✗ "
				anyMatch = true
			}
			if line.Speaker != "" {
				fmt.Fprintf(&sceneOut, "> %s**%s:** %s\n", marker, line.Speaker, bolded)
			} else {
				fmt.Fprintf(&sceneOut, "> %s%s\n", marker, bolded)
			}
		}
		if anyMatch {
			rendered = append(rendered, sceneOut.String())
		}
	}
	if len(rendered) == 0 {
		return false
	}
	sb.WriteString("#### Conversations\n\n")
	for i, r := range rendered {
		if i > 0 {
			// Blank line between scenes ends the previous blockquote
			// and starts the next as a fresh block — cleaner than the
			// empty `>` line the previous version used.
			sb.WriteString("\n")
		}
		sb.WriteString(r)
	}
	sb.WriteString("\n")
	return true
}

// boldFailedExpressions returns the quote with each failed expression
// it contains wrapped in **bold** markers, and a flag indicating
// whether any failure matched. Matching tries, per failure:
//
//  1. exact case-insensitive substring of the full expression, anchored
//     at a word boundary on the left and expanded to the next word
//     boundary on the right ("scrimp" in "scrimping" bolds the whole
//     "scrimping"; "stuffed shirt" in "stuffed shirts" bolds "stuffed
//     shirts"). Anchoring to the left boundary prevents stray matches
//     like "take" inside "mistake".
//  2. the longest content-token of the expression as a whole-word
//     match, when (1) misses ("drum up business" → bolds "business"
//     in "drum up a lot of business"). The token must start at a word
//     boundary, again to keep "take" out of "mistake".
//
// Overlapping spans are merged so multiple failures that share the
// same word don't produce nested `****…****`.
func boldFailedExpressions(quote string, failures []analytics.WrongWord) (string, bool) {
	type span struct{ start, end int }
	var spans []span
	lower := strings.ToLower(quote)
	for _, f := range failures {
		expr := strings.TrimSpace(f.Expression)
		if expr == "" {
			continue
		}
		if s, e, ok := matchExpressionSpan(lower, expr); ok {
			spans = append(spans, span{s, e})
			continue
		}
		for _, tok := range significantTokens(expr) {
			if s, e, ok := findWordStartingWith(lower, tok); ok {
				spans = append(spans, span{s, e})
				break
			}
		}
	}
	if len(spans) == 0 {
		return quote, false
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	merged := []span{spans[0]}
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.start <= last.end {
			if s.end > last.end {
				last.end = s.end
			}
			continue
		}
		merged = append(merged, s)
	}
	out := quote
	for i := len(merged) - 1; i >= 0; i-- {
		s := merged[i]
		out = out[:s.start] + "**" + out[s.start:s.end] + "**" + out[s.end:]
	}
	return out, true
}

// matchExpressionSpan finds the first occurrence of expression in the
// lowercased quote that starts at a word boundary, then expands the
// right edge to the end of the surrounding word. Used so "scrimp"
// matches "scrimping" (becomes "scrimping" span) but "take" in "take
// the plunge" doesn't accidentally match "take" inside "mistake".
func matchExpressionSpan(lowerQuote, expression string) (int, int, bool) {
	exprLower := strings.ToLower(expression)
	if exprLower == "" {
		return 0, 0, false
	}
	pos := 0
	for {
		idx := strings.Index(lowerQuote[pos:], exprLower)
		if idx < 0 {
			return 0, 0, false
		}
		abs := pos + idx
		if abs > 0 && isWordChar(lowerQuote[abs-1]) {
			pos = abs + 1
			continue
		}
		end := abs + len(exprLower)
		for end < len(lowerQuote) && isWordChar(lowerQuote[end]) {
			end++
		}
		return abs, end, true
	}
}

// findWordStartingWith returns the byte range covering the first word
// whose lowercased root starts with the token. Used by the token
// fallback so "drum" matches "drum" / "drumming" but not "eardrum",
// and "take" never wanders inside "mistake".
func findWordStartingWith(lowerQuote, token string) (int, int, bool) {
	pos := 0
	for {
		idx := strings.Index(lowerQuote[pos:], token)
		if idx < 0 {
			return 0, 0, false
		}
		abs := pos + idx
		if abs > 0 && isWordChar(lowerQuote[abs-1]) {
			pos = abs + 1
			continue
		}
		end := abs + len(token)
		for end < len(lowerQuote) && isWordChar(lowerQuote[end]) {
			end++
		}
		return abs, end, true
	}
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '\''
}

// significantTokens picks out the content words of a multi-word
// expression — stopwords / articles / possessive markers stripped,
// tokens under four characters dropped. Returns the longest first so
// boldFailedExpressions tries the most discriminating token before
// giving up. Mirrors the analytics resolver's helper of the same name;
// duplicated to keep this package independent of the analytics
// internals.
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

// writeEtymologyConcepts renders the concept declarations + relations
// the session carries, marking any member whose origin appears in the
// failedOrigins set with a ✗ prefix so the user's eye lands on the
// origin they got wrong inside the otherwise-static concept block.
func writeEtymologyConcepts(
	sb *strings.Builder,
	concepts []notebook.Concept,
	relations []notebook.Relation,
	originMeanings map[string]string,
	failedOrigins map[string]bool,
) {
	if len(concepts) == 0 && len(relations) == 0 {
		return
	}
	sb.WriteString("#### Concepts\n\n")
	relationsByKey := groupRelationsByKey(relations)
	for _, c := range concepts {
		fmt.Fprintf(sb, "**%s — %s**\n\n", c.Key, c.Meaning)
		if c.Note != "" {
			fmt.Fprintf(sb, "_%s_\n\n", c.Note)
		}
		if len(c.Members) > 0 {
			sb.WriteString("| Member | Language | Meaning |\n")
			sb.WriteString("|---|---|---|\n")
			for _, m := range c.Members {
				marker := ""
				if failedOrigins[m.Origin] {
					marker = "✗ "
				}
				lang := orDash(m.Language)
				meaning := orDash(originMeanings[m.Origin])
				fmt.Fprintf(sb, "| %s%s | %s | %s |\n", marker, m.Origin, lang, meaning)
			}
			sb.WriteString("\n")
		}
		if rels := relationsByKey[c.Key]; len(rels) > 0 {
			sb.WriteString("Relations: ")
			for i, r := range rels {
				if i > 0 {
					sb.WriteString("; ")
				}
				sb.WriteString(r)
			}
			sb.WriteString("\n\n")
		}
	}
}

// groupRelationsByKey turns the flat relation list into a per-concept
// view of outgoing edges, each rendered as "<type> → <other concept>".
// Symmetric relations surface on both endpoints so a reader looking at
// either side of an antonym pair sees the link.
func groupRelationsByKey(relations []notebook.Relation) map[string][]string {
	out := make(map[string][]string)
	for _, r := range relations {
		if r.IsDirected() {
			if r.From != "" && r.To != "" {
				out[r.From] = append(out[r.From], fmt.Sprintf("%s → %s", r.Type, r.To))
			}
			continue
		}
		if len(r.Between) == 2 {
			a, b := r.Between[0], r.Between[1]
			if a != "" && b != "" {
				out[a] = append(out[a], fmt.Sprintf("%s ↔ %s", r.Type, b))
				out[b] = append(out[b], fmt.Sprintf("%s ↔ %s", r.Type, a))
			}
		}
	}
	return out
}

// failedNameSet collects every failed expression name (origin-side or
// vocab-side) so the concept block can mark matching member rows. Vocab
// failures are included because an English word can share its name with
// its etymology origin (`gauche` the adjective vs `gauche` the French
// origin) — the user should see ✗ on either case.
func failedNameSet(originFailures, vocabFailures []analytics.WrongWord) map[string]bool {
	out := make(map[string]bool, len(originFailures)+len(vocabFailures))
	for _, w := range originFailures {
		out[w.Expression] = true
	}
	for _, w := range vocabFailures {
		out[w.Expression] = true
	}
	return out
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func totalEntriesAcross(groups []quizReviewGroup) int {
	var n int
	for _, g := range groups {
		n += totalEntries(g)
	}
	return n
}

// writeEntry renders one wrong attempt. Format mirrors the etymology
// notebook output: headline, optional italic example, related-group
// lines. When suppressExample is true (the session has rendered
// conversations that already carry the same quote) the Example: line
// is dropped — otherwise it duplicates the bolded line in the
// conversation block above.
func writeEntry(sb *strings.Builder, w analytics.WrongWord, suppressExample bool) {
	fmt.Fprintf(sb, "- **%s** [%s]: %s\n", w.Expression, quizTypeLabel(w.QuizType), defaultIfEmpty(w.Meaning, "—"))
	if !suppressExample && w.ExampleSentence != "" {
		fmt.Fprintf(sb, "    - Example: *%s*\n", w.ExampleSentence)
	}
	for _, group := range w.RelatedGroups {
		header := relatedHeader(group.Kind)
		if group.Label != "" {
			header = fmt.Sprintf("%s (%s)", header, group.Label)
		}
		fmt.Fprintf(sb, "    - %s: %s\n", header, strings.Join(group.Members, ", "))
	}
	sb.WriteString("\n")
}

// isOriginQuizType matches every etymology-side quiz (breakdown /
// assembly / freeform) so an origin failure sorts under "Failed
// origins" rather than alongside English vocabulary words. Mirrors the
// analytics resolver's dispatch.
func isOriginQuizType(quizType string) bool {
	return strings.HasPrefix(quizType, "etymology_")
}

// quizTypeLabel maps the internal quiz_type string to a friendly label
// the reader can scan. Unknown / future quiz types pass through as-is
// so the file never hides a failure.
func quizTypeLabel(qt string) string {
	switch qt {
	case "notebook":
		return "vocab"
	case "reverse":
		return "vocab reverse"
	case "freeform":
		return "vocab freeform"
	case "etymology_breakdown":
		return "etymology breakdown"
	case "etymology_assembly":
		return "etymology assembly"
	case "etymology_freeform":
		return "etymology freeform"
	default:
		return qt
	}
}

// relatedHeader maps a RelatedGroup.Kind to the bullet header used in
// the markdown output. The kind strings come straight from the
// notebooks' relation type field plus two built-in kinds ("concept",
// "origin_family") emitted by the analytics resolver; unrecognised
// kinds fall through verbatim so a new relation type from the YAML
// schema still surfaces.
func relatedHeader(kind string) string {
	switch kind {
	case "concept":
		return "Same sense"
	case "origin_family":
		return "Same origin family"
	default:
		return strings.Title(strings.ReplaceAll(kind, "_", " "))
	}
}

func totalEntries(g quizReviewGroup) int {
	var n int
	for _, s := range g.sessions {
		n += len(s.vocab) + len(s.origin)
	}
	return n
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func defaultIfEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

