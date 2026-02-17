package main

import (
	"path/filepath"
	"testing"

	"github.com/at-ishikawa/langner/internal/testutil"
	"github.com/stretchr/testify/assert"
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

func TestNewNotebookCommand(t *testing.T) {
	cmd := newNotebookCommand()

	assert.Equal(t, "notebooks", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	// Verify sort flag
	sortFlag := cmd.PersistentFlags().Lookup("sort")
	assert.NotNil(t, sortFlag)
	assert.Equal(t, "desc", sortFlag.DefValue)
}

func TestNewNotebookCommand_Stories_RunE(t *testing.T) {
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
