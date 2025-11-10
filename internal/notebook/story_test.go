package notebook

import (
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/assets"
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

func TestAssetsStoryConverter_convertToAssetsStoryTemplate(t *testing.T) {
	testDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	notebooks := []StoryNotebook{
		{
			Event: "Test Story",
			Date:  testDate,
			Metadata: Metadata{
				Series:  "Test Series",
				Season:  1,
				Episode: 1,
			},
			Scenes: []StoryScene{
				{
					Title: "Scene 1",
					Conversations: []Conversation{
						{Speaker: "A", Quote: "This is a {{ test phrase }} here."},
					},
					Definitions: []Note{
						{Expression: "test phrase", Meaning: "A phrase for testing"},
					},
				},
			},
		},
	}

	tests := []struct {
		name string
		want assets.StoryTemplate
	}{
		{
			name: "Markdown conversion",
			want: assets.StoryTemplate{
				Notebooks: []assets.StoryNotebook{
					{
						Event: "Test Story",
						Date:  testDate,
						Metadata: assets.Metadata{
							Series:  "Test Series",
							Season:  1,
							Episode: 1,
						},
						Scenes: []assets.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []assets.Conversation{
									{Speaker: "A", Quote: "This is a **test phrase** here."},
								},
								Definitions: []assets.StoryNote{
									{
										Expression: "test phrase",
										Meaning:    "A phrase for testing",
										// Other fields will be empty strings/nil
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := newAssetsStoryConverter()
			result := converter.convertToAssetsStoryTemplate(notebooks)
			assert.Equal(t, tt.want, result)
		})
	}
}
