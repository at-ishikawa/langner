package main

import (
	"testing"

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

	// Verify subcommands exist
	stories, _, err := cmd.Find([]string{"stories"})
	assert.NoError(t, err)
	assert.Equal(t, "stories <notebook id>", stories.Use)

	flashcards, _, err := cmd.Find([]string{"flashcards"})
	assert.NoError(t, err)
	assert.Equal(t, "flashcards <flashcard id>", flashcards.Use)
}
