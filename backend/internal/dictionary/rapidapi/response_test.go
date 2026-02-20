package rapidapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromResponsesToMap(t *testing.T) {
	tests := []struct {
		name      string
		responses []Response
		wantKeys  []string
	}{
		{
			name:      "empty responses",
			responses: []Response{},
			wantKeys:  []string{},
		},
		{
			name: "single response",
			responses: []Response{
				{Word: "hello"},
			},
			wantKeys: []string{"hello"},
		},
		{
			name: "multiple responses",
			responses: []Response{
				{Word: "hello"},
				{Word: "world"},
			},
			wantKeys: []string{"hello", "world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromResponsesToMap(tt.responses)
			assert.Len(t, got, len(tt.wantKeys))
			for _, key := range tt.wantKeys {
				_, ok := got[key]
				assert.True(t, ok, "expected key %q in map", key)
			}
		})
	}
}

func TestPronunciation_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantAll string
		wantErr bool
	}{
		{
			name:    "struct format",
			json:    `{"all": "həˈloʊ"}`,
			wantAll: "həˈloʊ",
		},
		{
			name:    "string format",
			json:    `"həˈloʊ"`,
			wantAll: "\"həˈloʊ\"",
		},
		{
			name:    "empty struct",
			json:    `{"all": ""}`,
			wantAll: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Pronunciation
			err := json.Unmarshal([]byte(tt.json), &p)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantAll, p.All)
		})
	}
}

func TestResponse_ToFlashCard(t *testing.T) {
	tests := []struct {
		name          string
		response      Response
		sideSeparator string
		wantContains  []string
	}{
		{
			name: "word with pronunciation and results",
			response: Response{
				Word:          "happy",
				Pronunciation: Pronunciation{All: "ˈhæpi"},
				Results: []Result{
					{
						PartOfSpeech: "adjective",
						Definition:   "feeling pleasure",
						Examples:     []string{"a happy child"},
						Synonyms:     []string{"joyful", "cheerful"},
					},
				},
			},
			sideSeparator: "---\n",
			wantContains: []string{
				"happy: /ˈhæpi/",
				"[adjective]: feeling pleasure",
				"Examples: a happy child",
				"Synonyms: joyful, cheerful",
			},
		},
		{
			name: "word without pronunciation",
			response: Response{
				Word: "test",
				Results: []Result{
					{
						PartOfSpeech: "noun",
						Definition:   "a trial",
					},
				},
			},
			sideSeparator: "---\n",
			wantContains: []string{
				"test\n",
				"[noun]: a trial",
			},
		},
		{
			name: "word with derivation and similar",
			response: Response{
				Word:          "run",
				Pronunciation: Pronunciation{All: "rʌn"},
				Results: []Result{
					{
						PartOfSpeech: "verb",
						Definition:   "to move swiftly",
						SimilarTo:    []string{"sprint", "dash"},
						Derivation:   []string{"runner"},
					},
				},
			},
			sideSeparator: "|\n",
			wantContains: []string{
				"Similar to: sprint, dash",
				"Derivation: runner",
			},
		},
		{
			name: "multiple results",
			response: Response{
				Word: "bank",
				Results: []Result{
					{PartOfSpeech: "noun", Definition: "a financial institution"},
					{PartOfSpeech: "noun", Definition: "the side of a river"},
				},
			},
			sideSeparator: "---\n",
			wantContains: []string{
				"a financial institution",
				"the side of a river",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.ToFlashCard(tt.sideSeparator)
			for _, want := range tt.wantContains {
				assert.Contains(t, got, want)
			}
		})
	}
}
