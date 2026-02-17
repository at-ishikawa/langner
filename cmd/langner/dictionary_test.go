package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI_Set(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    API
		wantErr bool
	}{
		{
			name:  "valid API value",
			value: "words_api",
			want:  APIWordsAPIInRapidAPI,
		},
		{
			name:    "invalid API value",
			value:   "invalid_api",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var api API
			err := api.Set(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid API")
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, api)
		})
	}
}

func TestAPI_String(t *testing.T) {
	api := APIWordsAPIInRapidAPI
	assert.Equal(t, "words_api", api.String())
}

func TestAPI_Type(t *testing.T) {
	api := APIWordsAPIInRapidAPI
	assert.Equal(t, "API", api.Type())
}

func TestNewDictionaryCommand_Lookup_RunE_CachedWord(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	// Create a cached dictionary response
	cacheDir := filepath.Join(tmpDir, "dictionaries")
	cacheJSON := `{"word":"happy","results":[{"definition":"feeling pleasure","partOfSpeech":"adjective","synonyms":["glad","joyful"]}]}`
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "happy.json"), []byte(cacheJSON), 0644))

	cmd := newDictionaryCommand()
	cmd.SetArgs([]string{"lookup", "happy"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewDictionaryCommand_Lookup_RunE_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newDictionaryCommand()
	cmd.SetArgs([]string{"lookup", "test"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration")
}

func TestNewDictionaryCommand(t *testing.T) {
	cmd := newDictionaryCommand()

	assert.Equal(t, "dictionary", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	// Verify the api flag is registered
	apiFlag := cmd.PersistentFlags().Lookup("api")
	assert.NotNil(t, apiFlag)

	// Verify lookup subcommand
	lookup, _, err := cmd.Find([]string{"lookup"})
	assert.NoError(t, err)
	assert.Equal(t, "lookup", lookup.Use)
	assert.NotNil(t, lookup.RunE)
}
