package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestNewDictionaryCommand(t *testing.T) {
	cmd := newDictionaryCommand()

	assert.Equal(t, "dictionary", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	// Verify the api flag is registered
	apiFlag := cmd.PersistentFlags().Lookup("api")
	assert.NotNil(t, apiFlag)
}
