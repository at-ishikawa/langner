package quiz

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExampleData_DefinitionsSectionsFollowIndexOrder drives the REAL example
// config end to end (the same Service/Reader construction the server builds)
// and asserts that a definitions-only book's per-session sections come back on
// the quiz start page in the order their notebooks are declared in index.yml —
// NOT alphabetically.
//
// The book examples/definitions/session-order-demo declares its sessions as
// [Intro, UNIT ONE, UNIT TWO, UNIT THREE, UNIT FOUR, Appendix]. None of the
// titles ends in a digit, so the old trailing-integer/lexical sort scrambled
// them to [Appendix, Intro, UNIT FOUR, UNIT ONE, UNIT THREE, UNIT TWO]. This
// test FAILS on the pre-fix code (sections in alphabetical order) and PASSES
// once definitionsSectionSummaries honors the index order.
func TestExampleData_DefinitionsSectionsFollowIndexOrder(t *testing.T) {
	svc := newExampleService(t, t.TempDir())

	// includeUnstudied=true so a fresh (unseeded) learning history still counts
	// every session's words and the book surfaces with all its sections.
	summaries, err := svc.LoadNotebookSummaries(true)
	require.NoError(t, err)

	var book *NotebookSummary
	for i := range summaries {
		if summaries[i].NotebookID == "session-order-demo" {
			book = &summaries[i]
			break
		}
	}
	require.NotNil(t, book, "session-order-demo book must appear on the quiz start page")

	got := make([]string, 0, len(book.Sections))
	for _, sec := range book.Sections {
		got = append(got, sec.Title)
	}

	want := []string{"Intro", "UNIT ONE", "UNIT TWO", "UNIT THREE", "UNIT FOUR", "Appendix"}
	assert.Equal(t, want, got, "sections must follow index.yml declared order, not alphabetical")

	// Guard against a fixture that trivially passes: the declared order MUST
	// differ from the alphabetical order this book would produce pre-fix.
	alphabetical := append([]string(nil), want...)
	sort.Strings(alphabetical)
	require.NotEqual(t, alphabetical, want,
		"fixture must be non-lexical so it actually exercises the bug")
}
