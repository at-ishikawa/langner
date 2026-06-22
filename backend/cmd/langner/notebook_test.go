package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortFlag_Set(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    SortFlag
		wantErr bool
	}{
		{
			name:  "descending",
			value: "desc",
			want:  SortDescending,
		},
		{
			name:  "ascending",
			value: "asc",
			want:  SortAscending,
		},
		{
			name:    "invalid value",
			value:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var flag SortFlag
			err := flag.Set(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid value")
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, flag)
		})
	}
}

func TestSortFlag_String(t *testing.T) {
	tests := []struct {
		name string
		flag *SortFlag
		want string
	}{
		{
			name: "descending",
			flag: func() *SortFlag { f := SortDescending; return &f }(),
			want: "desc",
		},
		{
			name: "ascending",
			flag: func() *SortFlag { f := SortAscending; return &f }(),
			want: "asc",
		},
		{
			name: "nil pointer",
			flag: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.flag.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSortFlag_Type(t *testing.T) {
	flag := SortDescending
	assert.Equal(t, "SortFlag", flag.Type())
}

func TestNewNotebookCommand_Stories_RunE(t *testing.T) {
	skipIfNoDB(t)
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	testutil.CreateStoryNotebook(t, filepath.Join(tmpDir, "stories"), filepath.Join(tmpDir, "learning_notes"), "test-story")

	cmd := newNotebookCommand()
	cmd.SetArgs([]string{"stories", "test-story"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewNotebookCommand_Flashcards_RunE(t *testing.T) {
	skipIfNoDB(t)
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	testutil.CreateFlashcardNotebook(t, filepath.Join(tmpDir, "flashcards"), filepath.Join(tmpDir, "learning_notes"), "test-fc")

	cmd := newNotebookCommand()
	cmd.SetArgs([]string{"flashcards", "test-fc"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewNotebookCommand_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

	tests := []struct {
		name string
		args []string
	}{
		{name: "stories", args: []string{"stories", "test-id"}},
		{name: "flashcards", args: []string{"flashcards", "test-id"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newNotebookCommand()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "configuration")
		})
	}
}

func TestNewNotebookCommand_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	tests := []struct {
		name string
		args []string
	}{
		{name: "stories", args: []string{"stories", "nonexistent"}},
		{name: "flashcards", args: []string{"flashcards", "nonexistent"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newNotebookCommand()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			assert.Error(t, err)
		})
	}
}

// TestNewNotebookCommand_Etymology_HidesMasteredWords drives the
// `langner notebooks etymology <id>` CLI end-to-end and asserts the
// rendered markdown excludes a derived word the user has already
// learned. Reproduces the user-reported "egomaniac shows on the PDF
// file in word power made easy though it shouldn't be" symptom
// against a neutral fixture (`braveword` / `wordless`).
//
// Earlier tests for this filter lived only at the writer level and
// missed the bug because the failing path runs through the full CLI:
// config loading, reader setup, learning-history join, the
// EtymologyNotebookWriter, and the markdown template. The CLI test
// covers all of those together.
func TestNewNotebookCommand_Etymology_HidesMasteredWords(t *testing.T) {
	skipIfNoDB(t)
	tmpDir := t.TempDir()

	// Build an etymology + definitions + learning_notes layout matching
	// real user data: a session whose origins still need review (the
	// user has never drilled brave-root / word-root) and a derived word
	// `braveword` they have answered correctly in both directions today
	// (interval 30) plus an unanswered `wordless`.
	for _, d := range []string{
		"stories", "learning_notes", "flashcards", "dictionaries",
		"output_stories", "output_flashcards", "books", "definitions",
		"ebooks", "etymology", "output_etymology",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, d), 0o755))
	}

	etymDir := filepath.Join(tmpDir, "etymology", "demo-vocab")
	require.NoError(t, os.MkdirAll(etymDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: demo-vocab
kind: Etymology
name: Demo Vocab
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: brave-root
    language: Latin
    meaning: brave
  - origin: word-root
    language: Latin
    meaning: word
`), 0o644))

	defDir := filepath.Join(tmpDir, "definitions", "books", "demo-vocab")
	require.NoError(t, os.MkdirAll(defDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defDir, "index.yml"), []byte(`id: demo-vocab
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        title: "brave-root (brave)"
      expressions:
        - expression: braveword
          meaning: "uses brave-root and word-root"
          origin_parts:
            - origin: brave-root
            - origin: word-root
        - expression: wordless
          meaning: "uses word-root, never seen"
          origin_parts:
            - origin: word-root
`), 0o644))

	today := time.Now().UTC().Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "learning_notes", "demo-vocab.yml"),
		[]byte(fmt.Sprintf(`- metadata:
    id: demo-vocab
    title: "Session 1"
  scenes:
    - metadata:
        title: __index_0
      expressions:
        - expression: braveword
          learned_logs:
            - status: understood
              learned_at: %q
              quality: 4
              quiz_type: notebook
              interval_days: 30
          reverse_logs:
            - status: understood
              learned_at: %q
              quality: 4
              quiz_type: reverse
              interval_days: 30
`, today, today)), 0o644))

	cfgPath := filepath.Join(tmpDir, "config.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(fmt.Sprintf(`notebooks:
  stories_directories:
    - %s
  learning_notes_directory: %s
  flashcards_directories:
    - %s
  books_directories:
    - %s
  definitions_directories:
    - %s
  etymology_directories:
    - %s
dictionaries:
  rapidapi:
    cache_directory: %s
outputs:
  story_directory: %s
  flashcard_directory: %s
  etymology_directory: %s
books:
  repo_directory: %s
  repositories_file: %s
`,
		filepath.Join(tmpDir, "stories"),
		filepath.Join(tmpDir, "learning_notes"),
		filepath.Join(tmpDir, "flashcards"),
		filepath.Join(tmpDir, "books"),
		filepath.Join(tmpDir, "definitions"),
		filepath.Join(tmpDir, "etymology"),
		filepath.Join(tmpDir, "dictionaries"),
		filepath.Join(tmpDir, "output_stories"),
		filepath.Join(tmpDir, "output_flashcards"),
		filepath.Join(tmpDir, "output_etymology"),
		filepath.Join(tmpDir, "ebooks"),
		filepath.Join(tmpDir, "repos.yml"),
	)), 0o644))
	setConfigFile(t, cfgPath)

	cmd := newNotebookCommand()
	cmd.SetArgs([]string{"etymology", "demo-vocab"})
	require.NoError(t, cmd.Execute())

	mdPath := filepath.Join(tmpDir, "output_etymology", "demo-vocab.md")
	content, err := os.ReadFile(mdPath)
	require.NoError(t, err, "the etymology CLI should have written %s", mdPath)
	out := string(content)

	assert.NotContains(t, out, "braveword",
		"the CLI-driven etymology PDF/markdown must exclude derived words "+
			"the user has already learned (recent correct in both "+
			"directions, interval 30) — re-reading a known word's "+
			"definition doesn't help drill its origins")
	assert.Contains(t, out, "wordless",
		"unlearned words must still appear so the user can drill the origins via context")
	assert.Contains(t, out, "brave-root",
		"unmastered origins must still appear in the origins list at the top of the chapter")
	assert.Contains(t, out, "word-root",
		"unmastered origins must still appear in the origins list at the top of the chapter")
}
