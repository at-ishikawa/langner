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
	repo analytics.Repository
}

// NewWriter constructs a writer. The repository must already be
// configured with its MetadataResolver so the WrongWords come
// pre-hydrated with meanings, examples and related groups (the
// quiz-review rendering reads those fields directly off the WrongWord).
func NewWriter(repo analytics.Repository) *Writer {
	return &Writer{repo: repo}
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
	body := renderQuizReviewAllNotebooks(dateStr, groupByNotebook(detail.WrongWords))
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
// notebook, sessions follow the same first-appearance order, and
// per-session subsections separate origin failures from vocabulary
// failures so the reader can drill the etymology side and the
// English-word side independently.
func renderQuizReviewAllNotebooks(date string, groups []quizReviewGroup) string {
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
			if len(s.origin) > 0 {
				sb.WriteString("#### Failed origins\n\n")
				for _, w := range s.origin {
					writeEntry(&sb, w)
				}
			}
			if len(s.vocab) > 0 {
				sb.WriteString("#### Failed vocabularies\n\n")
				for _, w := range s.vocab {
					writeEntry(&sb, w)
				}
			}
		}
	}
	return sb.String()
}

func totalEntriesAcross(groups []quizReviewGroup) int {
	var n int
	for _, g := range groups {
		n += totalEntries(g)
	}
	return n
}

// writeEntry renders one wrong attempt. Format mirrors the etymology
// notebook output so the reader's eye trains on the same shape across
// study materials: headline, italic example, related-group lines.
func writeEntry(sb *strings.Builder, w analytics.WrongWord) {
	fmt.Fprintf(sb, "- **%s** [%s]: %s\n", w.Expression, quizTypeLabel(w.QuizType), defaultIfEmpty(w.Meaning, "—"))
	if w.ExampleSentence != "" {
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

