package ebook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "Single letter initial M. Waldman",
			input: "In M. Waldman I found a true friend.",
			expected: []string{
				"In M. Waldman I found a true friend.",
			},
		},
		{
			name:  "Multiple single letter initials",
			input: "Mr. J. Smith met Dr. A. Brown at the office.",
			expected: []string{
				"Mr. J. Smith met Dr. A. Brown at the office.",
			},
		},
		{
			name:  "Simple two sentences",
			input: "Hello world. How are you?",
			expected: []string{
				"Hello world.",
				"How are you?",
			},
		},
		{
			name:  "Sentence with Mr. abbreviation",
			input: "Mr. Smith went to the store. He bought milk.",
			expected: []string{
				"Mr. Smith went to the store.",
				"He bought milk.",
			},
		},
		{
			name:  "Sentence ending with abbreviation-like word",
			input: "The meeting ended at 5 PM. Everyone left.",
			expected: []string{
				"The meeting ended at 5 PM.",
				"Everyone left.",
			},
		},
		{
			name:  "Single letter initial at start",
			input: "M. Krempe was a professor. He taught chemistry.",
			expected: []string{
				"M. Krempe was a professor.",
				"He taught chemistry.",
			},
		},
		{
			name:  "Date line with month abbreviation",
			input: "St. Petersburgh, Dec. 11th, 17—.",
			expected: []string{
				"St. Petersburgh, Dec. 11th, 17—.",
			},
		},
		{
			name:  "Repeated exclamation with lowercase continuation",
			input: `"Oh, save me! save me!" I imagined that the monster seized me.`,
			expected: []string{
				// Split at !" I because I is uppercase (narrator continues after dialogue)
				`"Oh, save me! save me!"`,
				"I imagined that the monster seized me.",
			},
		},
		{
			name:  "Lowercase after exclamation keeps together",
			input: `Oh, save me! save me! Please help!`,
			expected: []string{
				// "save" is lowercase so stays with previous exclamation
				"Oh, save me! save me!",
				"Please help!",
			},
		},
		{
			name:  "Exclamation followed by capital letter starts new sentence",
			input: "Stop! Don't move.",
			expected: []string{
				"Stop!",
				"Don't move.",
			},
		},
		{
			name:  "Question with lowercase continuation in dialogue",
			input: `"What is it? he asked nervously.`,
			expected: []string{
				`"What is it? he asked nervously.`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSentences(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAbbreviation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Mr.", "Hello Mr.", true},
		{"Dr.", "Visit Dr.", true},
		{"Single letter M.", "In M.", true},
		{"Single letter J.", "Meet J.", true},
		{"Single letter at start", "M.", true},
		{"Not abbreviation AM.", "10 AM.", false}, // AM is not preceded by space
		{"Regular word ending in period", "Hello.", false},
		{"etc.", "and so on etc.", true},
		{"Dec. month abbreviation", "On Dec.", true},
		{"Jan. month abbreviation", "In Jan.", true},
		{"St. abbreviation", "Visit St.", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAbbreviation(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
