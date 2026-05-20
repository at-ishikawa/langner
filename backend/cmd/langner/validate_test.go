package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/testutil"
)

func TestIsDBConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.DatabaseConfig
		want bool
	}{
		{
			name: "password set - configured",
			cfg:  config.DatabaseConfig{Host: "localhost", Password: "secret"},
			want: true,
		},
		{
			name: "empty password - not configured",
			cfg:  config.DatabaseConfig{Host: "localhost"},
			want: false,
		},
		{
			name: "empty config - not configured",
			cfg:  config.DatabaseConfig{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDBConfigured(tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewValidateCommand(t *testing.T) {
	cmd := newValidateCommand()

	assert.Equal(t, "validate", cmd.Use)
	assert.NotNil(t, cmd.RunE)

	// Verify fix flag
	fixFlag := cmd.Flags().Lookup("fix")
	assert.NotNil(t, fixFlag)
	assert.Equal(t, "false", fixFlag.DefValue)
}

func TestDisplayValidationResults(t *testing.T) {
	tests := []struct {
		name   string
		result *notebook.ValidationResult
		want   []string
	}{
		{
			name:   "no errors or warnings",
			result: &notebook.ValidationResult{},
			want:   []string{"All validations passed!"},
		},
		{
			name: "learning notes errors",
			result: &notebook.ValidationResult{
				LearningNotesErrors: []notebook.ValidationError{
					{Location: "file1.yml", Message: "invalid format"},
				},
			},
			want: []string{"Learning Notes Validation Errors (1)", "invalid format", "Total errors: 1"},
		},
		{
			name: "consistency errors - orphaned",
			result: &notebook.ValidationResult{
				ConsistencyErrors: []notebook.ValidationError{
					{Location: "file1.yml", Message: "orphaned learning note for expression"},
				},
			},
			want: []string{"Consistency Validation Errors (1)", "Orphaned learning notes (1)", "Total errors: 1"},
		},
		{
			name: "consistency errors - duplicate",
			result: &notebook.ValidationResult{
				ConsistencyErrors: []notebook.ValidationError{
					{Location: "file1.yml", Message: "duplicate expression found"},
				},
			},
			want: []string{"Duplicate expressions (1)"},
		},
		{
			name: "consistency errors - missing scene",
			result: &notebook.ValidationResult{
				ConsistencyErrors: []notebook.ValidationError{
					{Location: "file1.yml", Message: "scene foo not found in notebook"},
				},
			},
			want: []string{"Missing or mismatched scenes (1)"},
		},
		{
			name: "consistency errors - dictionary",
			result: &notebook.ValidationResult{
				ConsistencyErrors: []notebook.ValidationError{
					{Location: "file1.yml", Message: "dictionary reference missing"},
				},
			},
			want: []string{"Dictionary reference errors (1)"},
		},
		{
			name: "consistency errors - other",
			result: &notebook.ValidationResult{
				ConsistencyErrors: []notebook.ValidationError{
					{Location: "file1.yml", Message: "some other error"},
				},
			},
			want: []string{"Other errors (1)"},
		},
		{
			name: "warnings - missing learning notes",
			result: &notebook.ValidationResult{
				Warnings: []notebook.ValidationError{
					{Location: "file1.yml", Message: "missing learning note for expression"},
				},
			},
			want: []string{"Warnings (1)", "Missing learning notes (1)"},
		},
		{
			name: "warnings - no learned_logs",
			result: &notebook.ValidationResult{
				Warnings: []notebook.ValidationError{
					{Location: "file1.yml", Message: "no learned_logs for expression"},
				},
			},
			want: []string{"Expressions without learning logs (1)"},
		},
		{
			name: "warnings - other",
			result: &notebook.ValidationResult{
				Warnings: []notebook.ValidationError{
					{Location: "file1.yml", Message: "some other warning"},
				},
			},
			want: []string{"Other warnings (1)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			displayValidationResults(tt.result)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			for _, want := range tt.want {
				assert.Contains(t, output, want)
			}
		})
	}
}

func TestDisplayValidationResults_ManyMissingLearningNotes(t *testing.T) {
	// Test truncation of missing learning notes (>10)
	result := &notebook.ValidationResult{}
	for i := 0; i < 15; i++ {
		result.Warnings = append(result.Warnings, notebook.ValidationError{
			Location: "file.yml",
			Message:  "missing learning note for expression",
		})
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	displayValidationResults(result)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "... and 5 more")
}

func TestNewValidateCommand_RunE(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		args    []string
		wantErr string
	}{
		{
			name: "invalid config",
			setup: func(t *testing.T) {
				cfgPath := setupBrokenConfigFile(t)
				setConfigFile(t, cfgPath)
			},
			wantErr: "configuration",
		},
		{
			name: "valid config",
			setup: func(t *testing.T) {
				tmpDir := t.TempDir()
				cfgPath := testutil.SetupTestConfig(t, tmpDir)
				setConfigFile(t, cfgPath)
			},
		},
		{
			name: "with fix flag",
			setup: func(t *testing.T) {
				tmpDir := t.TempDir()
				cfgPath := testutil.SetupTestConfig(t, tmpDir)
				setConfigFile(t, cfgPath)
			},
			args: []string{"--fix"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			cmd := newValidateCommand()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestNewValidateCommand_FixPreservesSkippedBookExpression pins the
// current behaviour for the user-reported "I skipped introvert/extrovert
// from my notebooks, confirmed they were recorded, but they were
// deleted" symptom. The deletion path was the fixConsistency orphan
// sweep without a SkippedAt carve-out; the carve-out (`hasSkip` check at
// validator.go's `Only keep expressions that either:` block) is in
// place now, and this test locks it in so a future regression that
// drops the check immediately surfaces.
//
// I could NOT reproduce the user's deletion with the current binary, so
// this test is a regression guard rather than a failing repro. If the
// user can still see introvert/extrovert disappear on a fresh skip + fix
// cycle, the bug lives elsewhere and we need a different scenario.
func TestNewValidateCommand_FixPreservesSkippedBookExpression(t *testing.T) {
	tmpDir := t.TempDir()

	for _, d := range []string{
		"stories", "learning_notes", "flashcards", "dictionaries",
		"output_stories", "output_flashcards", "books", "definitions",
		"ebooks", "etymology",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, d), 0o755))
	}

	// Definitions book (NOT story) with two expressions. Book-side
	// existsInStory check inside fixConsistency only consults story
	// files, so a book expression that only has SkippedAt set must
	// still survive on the strength of hasSkip alone.
	defDir := filepath.Join(tmpDir, "definitions", "books", "demo-book")
	require.NoError(t, os.MkdirAll(defDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defDir, "index.yml"), []byte(`id: demo-book
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        title: "demo-scene"
      expressions:
        - expression: skipword
          meaning: "the word the user has skipped"
        - expression: keepword
          meaning: "the word the user is still learning"
`), 0o644))

	today := time.Now().UTC().Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "learning_notes", "demo-book.yml"),
		[]byte(fmt.Sprintf(`- metadata:
    id: demo-book
    title: "Session 1"
  scenes:
    - metadata:
        title: "demo-scene"
      expressions:
        - expression: skipword
          learned_logs: []
          skipped_at:
            notebook: %q
`, today)), 0o644))

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
		filepath.Join(tmpDir, "ebooks"),
		filepath.Join(tmpDir, "repos.yml"),
	)), 0o644))
	setConfigFile(t, cfgPath)

	cmd := newValidateCommand()
	cmd.SetArgs([]string{"--fix"})
	_ = cmd.Execute() // --fix may exit non-zero if other validations fail; the assertion is on the skip-only entry

	after, err := os.ReadFile(filepath.Join(tmpDir, "learning_notes", "demo-book.yml"))
	require.NoError(t, err)
	out := string(after)
	assert.Contains(t, out, "skipword",
		"a book expression with SkippedAt set but no learned_logs must survive validate --fix; "+
			"the skip is a deliberate user signal, not an orphan to clean up")
	assert.Contains(t, out, "skipped_at",
		"the skipped_at map itself must round-trip through validate --fix")
}
