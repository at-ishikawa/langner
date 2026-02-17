package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/assert"
)

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

func TestNewValidateCommand_RunE_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newValidateCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration")
}

func TestNewValidateCommand_RunE(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newValidateCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewValidateCommand_RunE_WithFix(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newValidateCommand()
	cmd.SetArgs([]string{"--fix"})
	err := cmd.Execute()
	assert.NoError(t, err)
}
