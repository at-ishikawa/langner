package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile is a tiny helper that creates parent dirs and writes a file.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func setupMergeConceptsFixture(t *testing.T) (learningNotesDir string, definitionsDir string, bookID string) {
	t.Helper()
	root := t.TempDir()

	learningNotesDir = filepath.Join(root, "learning_notes")
	require.NoError(t, os.MkdirAll(learningNotesDir, 0o755))

	definitionsDir = filepath.Join(root, "definitions")
	bookID = "demo-book"
	bookDir := filepath.Join(definitionsDir, bookID)
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	writeFile(t, filepath.Join(bookDir, "index.yml"), `id: demo-book
notebooks:
  - ./brightness.yml
`)
	writeFile(t, filepath.Join(bookDir, "brightness.yml"), `- metadata:
    title: "Brightness Session"
  scenes:
    - metadata:
        index: 0
        title: "Bright cluster"
      expressions:
        - expression: bright
          meaning: emitting much light
        - expression: brighten
          meaning: to make or become bright
        - expression: brightness
          meaning: the quality of being bright
  concepts:
    - head: bright
      meaning: the quality or act of being bright
      expressions:
        - bright
        - brighten
        - brightness
`)
	return learningNotesDir, definitionsDir, bookID
}

func TestMergeConcepts_FoldsMembersIntoHead(t *testing.T) {
	learningNotesDir, definitionsDir, bookID := setupMergeConceptsFixture(t)

	// Three separate entries pre-merge, each with its own log.
	writeFile(t, filepath.Join(learningNotesDir, bookID+".yml"), `- metadata:
    title: "Brightness Session"
  scenes:
    - metadata:
        title: "Bright cluster"
      expressions:
        - expression: bright
          learned_logs:
            - status: understood
              learned_at: "2025-03-01T10:00:00Z"
              quality: 4
              interval_days: 14
        - expression: brighten
          learned_logs:
            - status: understood
              learned_at: "2025-03-05T10:00:00Z"
              quality: 4
              interval_days: 7
        - expression: brightness
          learned_logs:
            - status: misunderstood
              learned_at: "2025-03-02T10:00:00Z"
              quality: 1
              interval_days: 3
          skipped_at:
            reverse: "2025-02-15T10:00:00Z"
`)

	err := MergeConcepts(learningNotesDir, []string{definitionsDir}, false)
	require.NoError(t, err)

	hists, err := notebook.NewLearningHistories(learningNotesDir)
	require.NoError(t, err)
	require.Contains(t, hists, bookID)
	historyList := hists[bookID]
	require.Len(t, historyList, 1)
	require.Len(t, historyList[0].Scenes, 1)
	exprs := historyList[0].Scenes[0].Expressions

	// Only the head should remain.
	require.Len(t, exprs, 1, "non-head members must be removed")
	assert.Equal(t, "bright", exprs[0].Expression)

	// Logs are unioned newest-first.
	require.Len(t, exprs[0].LearnedLogs, 3, "head ends up with all 3 members' logs")
	assert.True(t, exprs[0].LearnedLogs[0].LearnedAt.After(exprs[0].LearnedLogs[1].LearnedAt.Time),
		"logs are sorted newest-first")

	// Newest log's interval_days is rewritten to min across members'
	// pre-merge latest intervals (14, 7, 3) -> 3.
	assert.Equal(t, 3, exprs[0].LearnedLogs[0].IntervalDays,
		"head's newest log interval_days is rewritten to min(14, 7, 3) = 3")

	// Skip from brightness propagates to the merged head's skipped_at.
	assert.Equal(t, "2025-02-15T10:00:00Z", exprs[0].SkippedAt["reverse"])
}

func TestMergeConcepts_PromotesMemberWhenHeadMissing(t *testing.T) {
	learningNotesDir, definitionsDir, bookID := setupMergeConceptsFixture(t)

	// The head has no entry yet; only a member does. The member should be
	// promoted to the head's name.
	writeFile(t, filepath.Join(learningNotesDir, bookID+".yml"), `- metadata:
    title: "Brightness Session"
  scenes:
    - metadata:
        title: "Bright cluster"
      expressions:
        - expression: brighten
          learned_logs:
            - status: understood
              learned_at: "2025-03-05T10:00:00Z"
              quality: 4
              interval_days: 7
`)

	err := MergeConcepts(learningNotesDir, []string{definitionsDir}, false)
	require.NoError(t, err)

	hists, err := notebook.NewLearningHistories(learningNotesDir)
	require.NoError(t, err)
	exprs := hists[bookID][0].Scenes[0].Expressions
	require.Len(t, exprs, 1)
	assert.Equal(t, "bright", exprs[0].Expression, "lone member is renamed to the head")
}

func TestMergeConcepts_DryRunDoesNotWrite(t *testing.T) {
	learningNotesDir, definitionsDir, bookID := setupMergeConceptsFixture(t)

	original := `- metadata:
    title: "Brightness Session"
  scenes:
    - metadata:
        title: "Bright cluster"
      expressions:
        - expression: bright
          learned_logs:
            - status: understood
              learned_at: "2025-03-01T10:00:00Z"
              interval_days: 14
        - expression: brighten
          learned_logs:
            - status: understood
              learned_at: "2025-03-05T10:00:00Z"
              interval_days: 7
`
	writeFile(t, filepath.Join(learningNotesDir, bookID+".yml"), original)

	err := MergeConcepts(learningNotesDir, []string{definitionsDir}, true)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(learningNotesDir, bookID+".yml"))
	require.NoError(t, err)
	assert.Equal(t, original, string(got), "dry-run must not modify the file")
}

func TestMergeConcepts_NoConcepts_NoOp(t *testing.T) {
	root := t.TempDir()
	learningNotesDir := filepath.Join(root, "learning_notes")
	require.NoError(t, os.MkdirAll(learningNotesDir, 0o755))
	definitionsDir := filepath.Join(root, "definitions")
	require.NoError(t, os.MkdirAll(definitionsDir, 0o755))

	// No concepts: block at all. Merge should be a no-op.
	err := MergeConcepts(learningNotesDir, []string{definitionsDir}, false)
	require.NoError(t, err)
}
