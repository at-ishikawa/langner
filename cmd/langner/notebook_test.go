package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
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

func TestNewNotebookCommand(t *testing.T) {
	cmd := newNotebookCommand()

	assert.Equal(t, "notebooks", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	// Verify sort flag
	sortFlag := cmd.PersistentFlags().Lookup("sort")
	assert.NotNil(t, sortFlag)
	assert.Equal(t, "desc", sortFlag.DefValue)

	// Verify subcommands exist
	stories, _, err := cmd.Find([]string{"stories"})
	assert.NoError(t, err)
	assert.Equal(t, "stories <notebook id>", stories.Use)

	flashcards, _, err := cmd.Find([]string{"flashcards"})
	assert.NoError(t, err)
	assert.Equal(t, "flashcards <flashcard id>", flashcards.Use)
}

func TestNewNotebookCommand_Stories_RunE(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	// Create story data
	storiesDir := filepath.Join(tmpDir, "stories", "test-story")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(storiesDir, "index.yml"), notebook.Index{
		Kind: "story", ID: "test-story", Name: "Test Story",
		NotebookPaths: []string{"stories.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(storiesDir, "stories.yml"), []notebook.StoryNotebook{
		{
			Event: "Episode 1",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Scenes: []notebook.StoryScene{
				{
					Title: "Scene 1",
					Conversations: []notebook.Conversation{
						{Speaker: "A", Quote: "The {{ eager }} student arrived early."},
					},
					Definitions: []notebook.Note{
						{Expression: "eager", Meaning: "wanting to do something very much"},
					},
				},
			},
		},
	}))

	cmd := newNotebookCommand()
	cmd.SetArgs([]string{"stories", "test-story"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewNotebookCommand_Flashcards_RunE(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	// Create flashcard data
	flashcardDir := filepath.Join(tmpDir, "flashcards", "test-fc")
	require.NoError(t, os.MkdirAll(flashcardDir, 0755))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "index.yml"), notebook.FlashcardIndex{
		ID: "test-fc", Name: "Test Flashcards",
		NotebookPaths: []string{"cards.yml"},
	}))
	require.NoError(t, notebook.WriteYamlFile(filepath.Join(flashcardDir, "cards.yml"), []notebook.FlashcardNotebook{
		{
			Title: "Common Words",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Cards: []notebook.Note{
				{Expression: "break the ice", Meaning: "to initiate social interaction"},
			},
		},
	}))

	cmd := newNotebookCommand()
	cmd.SetArgs([]string{"flashcards", "test-fc"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewNotebookCommand_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

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

func TestNewNotebookCommand_Stories_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newNotebookCommand()
	cmd.SetArgs([]string{"stories", "nonexistent"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestNewNotebookCommand_Flashcards_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newNotebookCommand()
	cmd.SetArgs([]string{"flashcards", "nonexistent"})
	err := cmd.Execute()
	assert.Error(t, err)
}
