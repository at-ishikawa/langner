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
	// IsBook reports whether the notebookID belongs to a full-book
	// source (Gatsby, John Tenniel, etc., loaded from
	// books_directories) rather than a study notebook (Speak English,
	// flashcards, WPME). Quiz-review is a study sheet for failed
	// vocab/origins — book chapters don't fit that mental model and
	// would drown out the study notebooks in the output.
	IsBook(notebookID string) bool
	// VocabularyForSession returns the (expression, definition) pairs
	// declared in the notebook's matching session. The YAML stores the
	// CONJUGATED form (e.g. "giving me the runaround") in the
	// expression field and the dictionary form ("give someone the
	// runaround") in the definition field. Learning history records
	// failures under whichever form the quiz prompted with — often the
	// dictionary form — so the writer needs the pair to find the
	// matching dialogue span when only the dictionary form was failed.
	VocabularyForSession(notebookID, sessionTitle string) []VocabularyPair
}

// VocabularyPair is one definition entry from the source notebook used
// to bridge learning-history failures (often recorded against the
// dictionary form) to dialogue text (which uses the conjugated form),
// and to filter the etymology concept block to concepts the failed
// word actually touches.
type VocabularyPair struct {
	// Expression is the YAML's expression field — the form as it
	// actually appears in conversation (e.g. "giving me the runaround").
	Expression string
	// Definition is the YAML's definition field — the canonical
	// dictionary form (e.g. "give someone the runaround"). Empty when
	// the YAML carries only the expression.
	Definition string
	// OriginNames lists the origin names declared under the note's
	// origin_parts (e.g. "gyne", "logos" for "gynecology"). The writer
	// expands each vocab failure's concept-filter set with these so a
	// failure on a derived word still surfaces the etymology concept
	// the origin belongs to (e.g. failing "gynecology" surfaces the
	// "woman" concept containing "gyne", not just concepts with
	// "gynecology" as a member).
	OriginNames []string
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

func (rs *readerSource) IsBook(notebookID string) bool {
	return rs.reader.IsBook(notebookID)
}

func (rs *readerSource) VocabularyForSession(notebookID, sessionTitle string) []VocabularyPair {
	// First try ReadStoryNotebooks — story/book-style notebooks have
	// the definitions already merged into each scene's Definitions
	// slice, so finding the matching Event yields every pair declared
	// for that lesson. ReadStoryNotebooks returns an error for
	// definitions-only books (WPME) but the next branch covers them.
	if stories, err := rs.reader.ReadStoryNotebooks(notebookID); err == nil {
		for _, s := range stories {
			if s.Event != sessionTitle {
				continue
			}
			var pairs []VocabularyPair
			for _, scene := range s.Scenes {
				for _, def := range scene.Definitions {
					pairs = append(pairs, VocabularyPair{
						Expression:  def.Expression,
						Definition:  def.Definition,
						OriginNames: originPartNames(def.OriginParts),
					})
				}
			}
			return pairs
		}
	}
	// Definitions-only books (WPME): walk the per-session map keyed by
	// title to collect the same pair list.
	if defs, ok := rs.reader.GetDefinitionsNotes(notebookID); ok {
		if sessionDefs, ok := defs[sessionTitle]; ok {
			var pairs []VocabularyPair
			for _, sceneNotes := range sessionDefs {
				for _, note := range sceneNotes {
					pairs = append(pairs, VocabularyPair{
						Expression:  note.Expression,
						Definition:  note.Definition,
						OriginNames: originPartNames(note.OriginParts),
					})
				}
			}
			return pairs
		}
	}
	return nil
}

// originPartNames flattens a list of origin_parts into the origin
// names declared on the YAML — the bridge used by the writer to find
// which etymology concepts a failed vocabulary word belongs to.
func originPartNames(parts []notebook.OriginPartRef) []string {
	if len(parts) == 0 {
		return nil
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p.Origin != "" {
			out = append(out, p.Origin)
		}
	}
	return out
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
	// Drop attempts on book-type notebooks (Gatsby, John Tenniel, etc.)
	// AND attempts on expressions the user has explicitly skipped for
	// the matching quiz type. Both belong in the analytics page but
	// not in a study sheet: a skipped origin won't come up in future
	// quizzes anyway, so re-reading it isn't useful — and the user
	// reported the exact case ("Anglus", "Aphrodite", … all
	// etymology_assembly skips that kept landing in quiz-review).
	filtered := detail.WrongWords[:0:0]
	for _, ww := range detail.WrongWords {
		if w.source != nil && w.source.IsBook(ww.NotebookID) {
			continue
		}
		if ww.Skipped {
			continue
		}
		filtered = append(filtered, ww)
	}
	detail.WrongWords = filtered
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
				// Enrich each failure with the YAML's expression /
				// definition pair so a failure recorded under the
				// dictionary form ("give someone the runaround") can
				// still find and bold the conjugated form ("giving me
				// the runaround") in the dialogue, and so the concept
				// filter knows which origin_parts each failed vocab
				// touches.
				pairs := source.VocabularyForSession(g.notebookID, s.title)
				enriched := enrichVocabFailures(s.vocab, pairs)
				hasConversations = writeStoryConversations(&sb,
					source.StoryConversations(g.notebookID, s.title), enriched)
				// The concept-filter set includes origin failures (the
				// origin name IS the concept member), vocab failures
				// whose names happen to be origin names (gauche the
				// word == gauche the origin), AND every origin_part of
				// a failed vocab (gynecology touches `gyne` + `logos`,
				// so the "woman" concept containing `gyne` surfaces
				// even though "gynecology" itself isn't a concept
				// member). Concepts without any matching member are
				// dropped entirely — Session 9 of WPME previously
				// dumped 9 unrelated concept tables for a single
				// origin failure on `orthos`; with the filter it
				// surfaces only the concept `orthos` actually touches.
				touched := failedConceptMemberSet(s.origin, s.vocab, pairs)
				concepts, relations, originMeanings := source.EtymologyConcepts(g.notebookID, s.title)
				writeEtymologyConcepts(&sb, concepts, relations, originMeanings, touched)
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
func writeStoryConversations(sb *strings.Builder, scenes []SourceScene, failures []failureTerm) bool {
	if len(scenes) == 0 || len(failures) == 0 {
		return false
	}
	var rendered []string
	for _, scene := range scenes {
		sceneOut := strings.Builder{}
		anyMatch := false
		for _, line := range scene.Lines {
			// Match the source's PROVEN PDF-friendly shape from
			// assets/templates/story-notebook.md.go.tmpl: each
			// conversation line is a bullet item with an italic
			// speaker. Bullet items already render bold correctly
			// in the user's Kobo PDFs (the words section uses
			// `- **expression**: meaning`), so reusing the shape
			// keeps the failed-expression highlight visible end to
			// end. Stray `*` characters in the YAML source (footnote
			// markers like "losers*") are escaped first so they
			// don't accidentally open an italic span that swallows
			// the trailing **bold** marker for the failed word.
			safe := escapeStrayAsterisks(line.Quote)
			bolded, matched := boldFailedExpressions(safe, failures)
			if matched {
				anyMatch = true
			}
			// The bolded expression inside the quote IS the visual
			// marker for a matched line — Kobo's PDF font renders
			// `**bold**` reliably but mangled the previous `✗` glyph,
			// so the line-level marker is dropped.
			if line.Speaker != "" {
				fmt.Fprintf(&sceneOut, "- _%s_: %s\n", line.Speaker, bolded)
			} else {
				fmt.Fprintf(&sceneOut, "- %s\n", bolded)
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
			// Blank line ends one bullet list, horizontal rule
			// separates scenes, blank line starts the next list.
			sb.WriteString("\n---\n\n")
		}
		sb.WriteString(r)
	}
	sb.WriteString("\n")
	return true
}

// escapeStrayAsterisks backslash-escapes any `*` in the quote that
// isn't already part of a `**…**` bold span we control. The YAML
// source data sometimes carries a single `*` as a footnote marker
// ("losers* lately:"); markdown parsers see that `*` as the start of
// an italic span and consume the next `**` for the closing marker —
// the failed-expression bold then evaporates. Escaping the stray
// before we layer our own bold makes the highlight render reliably.
func escapeStrayAsterisks(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '*' {
			b.WriteByte(s[i])
			continue
		}
		// Leave existing `**` pairs alone — boldFailedExpressions
		// only ever introduces those, never stray ones, and the
		// source data doesn't carry literal `**`.
		if i+1 < len(s) && s[i+1] == '*' {
			b.WriteByte('*')
			b.WriteByte('*')
			i++
			continue
		}
		b.WriteString(`\*`)
	}
	return b.String()
}

// failureTerm is one failure enriched for matching against dialogue.
// canonical is the form the writer tries to bold (the YAML's
// expression — i.e. the conjugated form as it actually appears in the
// dialogue — when the source notebook had a pair for this failure;
// otherwise the failure's own expression string). alias is the
// dictionary form when the YAML carries one, kept as a fallback for
// the substring path. The token fallback uses canonical's significant
// tokens so a multi-word phrase whose canonical doesn't literally
// substring-match the quote still has a chance.
type failureTerm struct {
	canonical string
	alias     string
}

// enrichVocabFailures maps each WrongWord to a failureTerm using the
// source's per-session vocabulary pairs. When the failure expression
// matches a YAML definition (the dictionary form), the YAML's
// expression (the conjugated form) becomes the canonical match target
// — so "give someone the runaround" failures still bold "giving me the
// runaround" in the dialogue. Failures with no source pair fall back
// to using their own expression as canonical.
func enrichVocabFailures(failures []analytics.WrongWord, pairs []VocabularyPair) []failureTerm {
	byDefinition := make(map[string]VocabularyPair)
	byExpression := make(map[string]VocabularyPair)
	for _, p := range pairs {
		if p.Definition != "" {
			byDefinition[strings.ToLower(strings.TrimSpace(p.Definition))] = p
		}
		if p.Expression != "" {
			byExpression[strings.ToLower(strings.TrimSpace(p.Expression))] = p
		}
	}
	out := make([]failureTerm, 0, len(failures))
	for _, f := range failures {
		key := strings.ToLower(strings.TrimSpace(f.Expression))
		if p, ok := byExpression[key]; ok {
			out = append(out, failureTerm{canonical: p.Expression, alias: p.Definition})
			continue
		}
		if p, ok := byDefinition[key]; ok {
			out = append(out, failureTerm{canonical: p.Expression, alias: p.Definition})
			continue
		}
		out = append(out, failureTerm{canonical: f.Expression})
	}
	return out
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
func boldFailedExpressions(quote string, failures []failureTerm) (string, bool) {
	type span struct{ start, end int }
	var spans []span
	lower := strings.ToLower(quote)
	for _, f := range failures {
		matched := false
		for _, candidate := range []string{f.canonical, f.alias} {
			expr := strings.TrimSpace(candidate)
			if expr == "" {
				continue
			}
			if s, e, ok := matchExpressionSpan(lower, expr); ok {
				spans = append(spans, span{s, e})
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		// Token fallback: require ALL significant tokens of the
		// canonical (or alias) to appear in the line — never just one.
		// A single-token match overshoots when the YAML expression
		// contains a proper noun or common content word (e.g.
		// "Smoothitall" or "market" inside "take Smoothitall off the
		// market") and would bold every occurrence of that token
		// across the whole dialogue. Skipped entirely for single-token
		// expressions because the exact-substring path with
		// word-boundary expansion already handles conjugation cases
		// without the risk.
		expr := strings.TrimSpace(f.canonical)
		if expr == "" {
			expr = strings.TrimSpace(f.alias)
		}
		tokens := significantTokens(expr)
		if len(tokens) < 2 {
			continue
		}
		var tokenSpans []span
		allMatched := true
		for _, tok := range tokens {
			s, e, ok := findWordStartingWith(lower, tok)
			if !ok {
				allMatched = false
				break
			}
			tokenSpans = append(tokenSpans, span{s, e})
		}
		if allMatched {
			spans = append(spans, tokenSpans...)
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
// the session carries as bullet lists (NOT tables — mdtopdf doesn't
// render bold inside table cells reliably on Kobo). For each concept
// touched by a failure, members render as `- origin (Language) —
// meaning` with the failed member bolded. Relations expand to the
// related concept's members so the reader sees what "antonym ↔
// outward" actually contains without having to scroll.
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
	// Drop concepts that don't touch any failed origin/expression.
	// A session can declare 9 concepts but only one of them is
	// relevant to today's failures — rendering all of them buried
	// the actually-failed origin in a wall of unrelated tables.
	relevant := concepts[:0:0]
	for _, c := range concepts {
		if conceptTouchesFailure(c, failedOrigins) {
			relevant = append(relevant, c)
		}
	}
	if len(relevant) == 0 {
		return
	}
	sb.WriteString("#### Concepts\n\n")
	// All concepts (not just the relevant ones) get indexed so a
	// relation can expand its endpoint inline with that concept's
	// members — the user sees "antonym → outward (out): ec, ek, ex"
	// without having to find outward's own block.
	byKey := make(map[string]notebook.Concept, len(concepts))
	for _, c := range concepts {
		byKey[c.Key] = c
	}
	relationsByKey := groupConceptRelations(relations)
	for i, c := range relevant {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(sb, "**%s — %s**\n", c.Key, c.Meaning)
		if c.Note != "" {
			fmt.Fprintf(sb, "_%s_\n", c.Note)
		}
		sb.WriteString("\n")
		for _, m := range c.Members {
			fmt.Fprintf(sb, "- %s\n", formatMemberLine(m, originMeanings, failedOrigins[m.Origin]))
		}
		for _, rel := range relationsByKey[c.Key] {
			sb.WriteString("\n")
			writeRelationBlock(sb, rel, byKey, originMeanings)
		}
	}
}

// formatMemberLine renders one concept member as the bullet body —
// "origin (Language) — meaning", with the origin name bolded when
// it matches a failed origin/expression. Language and meaning collapse
// out when missing so a bare {origin: foo} member still reads cleanly.
func formatMemberLine(m notebook.ConceptMember, originMeanings map[string]string, failed bool) string {
	name := m.Origin
	if failed {
		name = "**" + name + "**"
	}
	var sb strings.Builder
	sb.WriteString(name)
	if m.Language != "" {
		fmt.Fprintf(&sb, " (%s)", m.Language)
	}
	if meaning := originMeanings[m.Origin]; meaning != "" {
		sb.WriteString(" — " + meaning)
	}
	return sb.String()
}

// writeRelationBlock expands one relation to the other concept's
// members. When the other concept is known (declared in the same
// book), its members are listed inline as a nested bullet block
// under a header line "Antonym → **outward — out**". When the
// concept isn't found (cross-book reference, malformed YAML), the
// header still renders so the link isn't silently dropped.
func writeRelationBlock(sb *strings.Builder, rel conceptRelation, byKey map[string]notebook.Concept, originMeanings map[string]string) {
	label := titleCase(rel.Type)
	other, ok := byKey[rel.OtherKey]
	if !ok {
		fmt.Fprintf(sb, "%s %s %s\n", label, rel.Arrow, rel.OtherKey)
		return
	}
	fmt.Fprintf(sb, "%s %s **%s — %s**\n", label, rel.Arrow, other.Key, other.Meaning)
	for _, m := range other.Members {
		fmt.Fprintf(sb, "    - %s\n", formatMemberLine(m, originMeanings, false))
	}
}

// conceptRelation describes one outgoing relation from a concept,
// carrying enough context for writeRelationBlock to render it without
// re-walking the relation list. Arrow distinguishes symmetric (↔)
// from directed (→) relations.
type conceptRelation struct {
	Type     string
	Arrow    string
	OtherKey string
}

// groupConceptRelations replaces the old groupRelationsByKey: it
// keeps the structured form so the writer can look up the other
// concept's members at render time, instead of pre-formatting to a
// flat string. Symmetric "between" relations surface on both
// endpoints; directed "from/to" relations surface only on the From
// side.
func groupConceptRelations(relations []notebook.Relation) map[string][]conceptRelation {
	out := make(map[string][]conceptRelation)
	for _, r := range relations {
		if r.IsDirected() {
			if r.From != "" && r.To != "" {
				out[r.From] = append(out[r.From], conceptRelation{Type: r.Type, Arrow: "→", OtherKey: r.To})
			}
			continue
		}
		if len(r.Between) == 2 {
			a, b := r.Between[0], r.Between[1]
			if a != "" && b != "" {
				out[a] = append(out[a], conceptRelation{Type: r.Type, Arrow: "↔", OtherKey: b})
				out[b] = append(out[b], conceptRelation{Type: r.Type, Arrow: "↔", OtherKey: a})
			}
		}
	}
	return out
}

// titleCase upper-cases the first character of a kind string for the
// relation header ("antonym" → "Antonym"). Unknown kinds pass through
// unchanged.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	first := strings.ToUpper(s[:1])
	return first + s[1:]
}

// conceptTouchesFailure returns true when at least one of the concept's
// members has its origin name in the failed-origin set — i.e. the
// member is itself a failed origin, OR it's an origin_part of a failed
// vocab word (when callers expanded the set via
// failedConceptMemberSet). Concepts that don't touch any failure are
// pure context for material the user didn't miss, so dropping them
// keeps the quiz-review focused.
func conceptTouchesFailure(c notebook.Concept, failedOrigins map[string]bool) bool {
	for _, m := range c.Members {
		if failedOrigins[m.Origin] {
			return true
		}
	}
	return false
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

// failedConceptMemberSet builds the set used to filter etymology
// concepts in writeEtymologyConcepts. It is failedNameSet PLUS every
// origin name declared under the origin_parts of a failed vocab word
// (looked up via the source's VocabularyForSession pairs). A failed
// vocab whose expression isn't itself an origin name still touches the
// concepts its origin_parts belong to (e.g. "gynecology" → gyne +
// logos → the "woman" concept containing gyne surfaces).
func failedConceptMemberSet(originFailures, vocabFailures []analytics.WrongWord, pairs []VocabularyPair) map[string]bool {
	out := failedNameSet(originFailures, vocabFailures)
	if len(vocabFailures) == 0 || len(pairs) == 0 {
		return out
	}
	byExpression := make(map[string]VocabularyPair, len(pairs))
	byDefinition := make(map[string]VocabularyPair, len(pairs))
	for _, p := range pairs {
		if p.Expression != "" {
			byExpression[strings.ToLower(strings.TrimSpace(p.Expression))] = p
		}
		if p.Definition != "" {
			byDefinition[strings.ToLower(strings.TrimSpace(p.Definition))] = p
		}
	}
	for _, w := range vocabFailures {
		key := strings.ToLower(strings.TrimSpace(w.Expression))
		var pair VocabularyPair
		if p, ok := byExpression[key]; ok {
			pair = p
		} else if p, ok := byDefinition[key]; ok {
			pair = p
		} else {
			continue
		}
		for _, origin := range pair.OriginNames {
			out[origin] = true
		}
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

// writeEntry renders one wrong attempt. The failed SIDE of the entry
// (the one the user couldn't recall) is bolded — for a standard quiz
// the user was shown the word and missed the meaning, so the meaning
// gets the bold; for a reverse / freeform / etymology_assembly quiz
// the user was shown the meaning and missed the word, so the word
// gets the bold. The previous "[vocab reverse]" tag is dropped: the
// section header (Failed origins / Failed vocabularies) plus the
// bolded side together tell the reader what they missed without
// adding a separate label to scan past.
//
// suppressExample skips the Example: line when the session already
// rendered conversations (the quote was bolded inside the dialogue).
//
// Related-group rendering uses nested bullets — concept members live
// on their own indented lines under an italicised concept-label
// header, so a long concept meaning like "a mental illness hindering
// personality development" no longer crowds out the related word.
func writeEntry(sb *strings.Builder, w analytics.WrongWord, suppressExample bool) {
	expression := w.Expression
	meaning := defaultIfEmpty(w.Meaning, "—")
	if wordIsFailed(w.QuizType) {
		expression = "**" + expression + "**"
	} else {
		meaning = "**" + meaning + "**"
	}
	fmt.Fprintf(sb, "- %s: %s\n", expression, meaning)
	if !suppressExample && w.ExampleSentence != "" {
		fmt.Fprintf(sb, "    - Example: *%s*\n", w.ExampleSentence)
	}
	for _, group := range w.RelatedGroups {
		header := relatedHeader(group.Kind)
		if group.Label != "" {
			fmt.Fprintf(sb, "    - %s — _%s_\n", header, group.Label)
		} else {
			fmt.Fprintf(sb, "    - %s\n", header)
		}
		for _, m := range group.Members {
			fmt.Fprintf(sb, "        - %s\n", m)
		}
	}
	sb.WriteString("\n")
}

// wordIsFailed reports whether this quiz type tested the user's recall
// of the WORD (given the meaning). True for reverse vocab, every
// _freeform variant, and etymology assembly. False for the standard
// "see word, recall meaning" direction — in that case the MEANING is
// the failed side and gets the bold.
func wordIsFailed(quizType string) bool {
	return quizType == "reverse" ||
		quizType == "etymology_assembly" ||
		strings.HasSuffix(quizType, "_freeform")
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

