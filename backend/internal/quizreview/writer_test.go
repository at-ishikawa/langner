package quizreview

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/analytics"
)

// stubRepo lets the writer tests inject a fixed DayDetail without
// going through the YAML repository — keeps the tests focused on the
// per-notebook split and rendering rather than the YAML format.
type stubRepo struct {
	detail analytics.DayDetail
	err    error
}

func (s *stubRepo) DailySummaries(context.Context, int, analytics.Filters) ([]analytics.DailySummary, error) {
	return nil, nil
}
func (s *stubRepo) DayDetail(context.Context, time.Time, analytics.Filters) (analytics.DayDetail, error) {
	return s.detail, s.err
}
func (s *stubRepo) WordHistory(context.Context, analytics.WordRef) (analytics.WordHistory, error) {
	return analytics.WordHistory{}, nil
}

func TestWriter_SingleFileWithEveryNotebook(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-16")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "word-power-made-easy",
					NotebookTitle: "Session 3",
					Expression:    "gauche",
					QuizType:      "notebook",
					Meaning:       "clumsy, tactless, especially in social situations",
					NotebookKind:  "story",
					RelatedGroups: []analytics.RelatedGroup{
						{Kind: "concept", Label: "clumsy in social situations", Members: []string{"gaucherie"}},
						{Kind: "antonym", Label: "rightness — right", Members: []string{"dexter (Latin) — right hand", "droit (French) — right hand"}},
					},
				},
				{
					NotebookID:    "word-power-made-easy",
					NotebookTitle: "Session 3",
					Expression:    "logos",
					QuizType:      "etymology_breakdown",
					Meaning:       "science, study",
					NotebookKind:  "etymology",
				},
				{
					NotebookID:    "word-power-made-easy",
					NotebookTitle: "Session 5",
					Expression:    "obstetrics",
					QuizType:      "notebook",
					Meaning:       "the medical specialty dealing with childbirth",
					NotebookKind:  "story",
				},
				{
					NotebookID:      "more-speak-english-like-an-american",
					NotebookTitle:   "LESSON 7: CINDY ASKS MARK TO GET BACK TOGETHER",
					Expression:      "stuffed shirt",
					QuizType:        "notebook",
					Meaning:         "a self-important formal person",
					ExampleSentence: "I've dated a lot of losers lately: stuffed shirts, two-timers — you get the picture.",
					NotebookKind:    "story",
				},
			},
		},
	}
	writer := NewWriter(repo)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "quiz-review-2026-06-16.md"), written,
		"single combined file lives directly under the output directory, named with the date")

	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)

	// Top-of-file summary covers every notebook.
	assert.Contains(t, out, "# Quiz review — 2026-06-16")
	assert.Contains(t, out, "4 wrong attempts across 2 notebooks.",
		"top summary counts every wrong attempt across every notebook on the day")

	// Each notebook is a top-level section, in first-appearance order.
	assert.Contains(t, out, "## word-power-made-easy")
	assert.Contains(t, out, "## more-speak-english-like-an-american")
	assert.Less(t, indexOf(out, "## word-power-made-easy"), indexOf(out, "## more-speak-english-like-an-american"),
		"notebooks render in first-appearance order — WPME comes first because its first wrong attempt was first in the day detail")

	// Per-notebook summary nested under each H2.
	assert.Contains(t, out, "3 wrong attempts across 2 sessions.",
		"per-notebook summary covers the entries inside that notebook only")
	assert.Contains(t, out, "1 wrong attempt across 1 session.",
		"the second notebook's summary exercises the singular pluralisation path")

	// Sessions sit one level deeper (### instead of ##).
	assert.Contains(t, out, "### Session 3")
	assert.Contains(t, out, "### Session 5")
	assert.Contains(t, out, "### LESSON 7: CINDY ASKS MARK TO GET BACK TOGETHER")

	// Failed-origins and failed-vocabularies blocks pushed to ####.
	assert.Contains(t, out, "#### Failed origins")
	assert.Contains(t, out, "#### Failed vocabularies")
	assert.Contains(t, out, "- **logos** [etymology breakdown]: science, study")
	assert.Contains(t, out, "- **gauche** [vocab]: clumsy, tactless, especially in social situations")
	assert.Contains(t, out, "    - Same sense (clumsy in social situations): gaucherie")
	assert.Contains(t, out, "    - Antonym (rightness — right): dexter (Latin) — right hand, droit (French) — right hand")

	// Notebook sections are separated by a horizontal rule so a reader
	// scrolling through the file gets a clear cut between notebooks.
	assert.Contains(t, out, "\n---\n", "horizontal rule separates each notebook section")

	// Stuffed-shirt example renders italic in the speak-english section.
	assert.Contains(t, out,
		"    - Example: *I've dated a lot of losers lately: stuffed shirts, two-timers — you get the picture.*",
		"example sentence still renders inside the entry body as italic")
}

// TestWriter_NoWrongAttemptsReturnsEmpty pins the no-op for days with no
// activity: no file is written, no error is raised. The CLI surfaces a
// friendly "nothing to write" line off the empty result.
func TestWriter_NoWrongAttemptsReturnsEmpty(t *testing.T) {
	repo := &stubRepo{detail: analytics.DayDetail{}}
	writer := NewWriter(repo)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), time.Now(), tmpDir, false)
	require.NoError(t, err)
	assert.Empty(t, written)
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no file should be created when there is nothing to write")
}

// TestWriter_RejectsEmptyOutputDirectory guards against silently
// writing to the working directory when the config omits the output
// path.
func TestWriter_RejectsEmptyOutputDirectory(t *testing.T) {
	writer := NewWriter(&stubRepo{detail: analytics.DayDetail{WrongWords: []analytics.WrongWord{{NotebookID: "x"}}}})
	_, err := writer.Output(context.Background(), time.Now(), "", false)
	require.Error(t, err)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
