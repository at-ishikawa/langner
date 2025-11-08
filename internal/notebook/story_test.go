package notebook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertMarkersInText(t *testing.T) {
	definitions := []Note{
		{Expression: "test phrase"},
		{Expression: "another word"},
	}

	tests := []struct {
		name             string
		text             string
		conversionStyle  ConversionStyle
		targetExpression string
		expected         string
	}{
		{
			name:             "Markdown - highlight specific expression",
			text:             "I have {{ test phrase }} and {{ another word }} here.",
			conversionStyle:  ConversionStyleMarkdown,
			targetExpression: "test phrase",
			expected:         "I have **test phrase** and another word here.",
		},
		{
			name:             "Terminal - highlight specific expression",
			text:             "I have {{ test phrase }} and {{ another word }} here.",
			conversionStyle:  ConversionStyleTerminal,
			targetExpression: "test phrase",
			expected:         "I have \x1b[1mtest phrase\x1b[22m and another word here.",
		},
		{
			name:             "Plain - all plain text",
			text:             "I have {{ test phrase }} and {{ another word }} here.",
			conversionStyle:  ConversionStylePlain,
			targetExpression: "test phrase",
			expected:         "I have test phrase and another word here.",
		},
		{
			name:             "Empty target - all expressions highlighted",
			text:             "I have {{ test phrase }} and {{ another word }} here.",
			conversionStyle:  ConversionStyleMarkdown,
			targetExpression: "",
			expected:         "I have **test phrase** and **another word** here.",
		},
		{
			name:             "Non-learning expression removed",
			text:             "I have {{ test phrase }} and {{ unknown }} here.",
			conversionStyle:  ConversionStyleMarkdown,
			targetExpression: "",
			expected:         "I have **test phrase** and unknown here.",
		},
		{
			name:             "Case insensitive matching",
			text:             "I have {{ TEST PHRASE }} here.",
			conversionStyle:  ConversionStyleMarkdown,
			targetExpression: "test phrase",
			expected:         "I have **TEST PHRASE** here.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ConvertMarkersInText(tc.text, definitions, tc.conversionStyle, tc.targetExpression)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestConvertStoryNotebookMarkers(t *testing.T) {
	notebooks := []StoryNotebook{
		{
			Event: "Test Story",
			Scenes: []StoryScene{
				{
					Title: "Scene 1",
					Conversations: []Conversation{
						{Speaker: "A", Quote: "This is a {{ test phrase }} here."},
					},
					Definitions: []Note{
						{Expression: "test phrase"},
					},
				},
			},
		},
	}

	tests := []struct {
		name            string
		conversionStyle ConversionStyle
		expectedQuote   string
	}{
		{
			name:            "Markdown conversion",
			conversionStyle: ConversionStyleMarkdown,
			expectedQuote:   "This is a **test phrase** here.",
		},
		{
			name:            "Terminal conversion",
			conversionStyle: ConversionStyleTerminal,
			expectedQuote:   "This is a \x1b[1mtest phrase\x1b[22m here.",
		},
		{
			name:            "Plain conversion",
			conversionStyle: ConversionStylePlain,
			expectedQuote:   "This is a test phrase here.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ConvertStoryNotebookMarkers(notebooks, tc.conversionStyle)
			assert.Equal(t, tc.expectedQuote, result[0].Scenes[0].Conversations[0].Quote)
		})
	}
}

