package cli

import (
	"testing"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/assert"
)

func TestWordOccurrence_GetExpression(t *testing.T) {
	tests := []struct {
		name       string
		definition *notebook.Note
		want       string
	}{
		{
			name: "Return Definition over Expression when Definition is set",
			definition: &notebook.Note{
				Expression: "lost his temper",
				Definition: "lose one's temper",
			},
			want: "lose one's temper",
		},
		{
			name: "Return Expression when Definition is empty",
			definition: &notebook.Note{
				Expression: "break the ice",
				Definition: "",
			},
			want: "break the ice",
		},
		{
			name: "Return empty string when both are empty",
			definition: &notebook.Note{
				Expression: "",
				Definition: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			occurrence := &WordOccurrence{
				Definition: tt.definition,
			}
			got := occurrence.GetExpression()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWordOccurrence_GetCleanContexts(t *testing.T) {
	tests := []struct {
		name     string
		contexts []WordOccurrenceContext
		want     []string
	}{
		{
			name: "Multiple contexts with markers",
			contexts: []WordOccurrenceContext{
				{Context: "The {{student}} learned {{ words }} from the {{ teacher }}.", Usage: "student"},
				{Context: "The {{ important }} {{ tasks }} are completed.", Usage: "task"},
			},
			want: []string{
				"The student learned words from the teacher.",
				"The important tasks are completed.",
			},
		},
		{
			name: "No markers",
			contexts: []WordOccurrenceContext{
				{Context: "This is a simple sentence.", Usage: "simple"},
			},
			want: []string{"This is a simple sentence."},
		},
		{
			name:     "Empty list",
			contexts: []WordOccurrenceContext{},
			want:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			occurrence := &WordOccurrence{
				Contexts: tt.contexts,
			}
			got := occurrence.GetCleanContexts()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWordOccurrence_GetMeaning(t *testing.T) {
	tests := []struct {
		name       string
		definition *notebook.Note
		want       string
	}{
		{
			name: "returns meaning",
			definition: &notebook.Note{
				Expression: "break the ice",
				Meaning:    "to initiate social interaction",
			},
			want: "to initiate social interaction",
		},
		{
			name: "empty meaning",
			definition: &notebook.Note{
				Expression: "hello",
				Meaning:    "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			occurrence := &WordOccurrence{Definition: tt.definition}
			got := occurrence.GetMeaning()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWordOccurrence_GetImages(t *testing.T) {
	tests := []struct {
		name       string
		definition *notebook.Note
		want       []string
	}{
		{
			name: "returns images",
			definition: &notebook.Note{
				Expression: "castle",
				Meaning:    "a large building",
				Images:     []string{"castle1.jpg", "castle2.jpg"},
			},
			want: []string{"castle1.jpg", "castle2.jpg"},
		},
		{
			name: "no images",
			definition: &notebook.Note{
				Expression: "hello",
				Meaning:    "a greeting",
			},
			want: nil,
		},
		{
			name: "empty images",
			definition: &notebook.Note{
				Expression: "hello",
				Images:     []string{},
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			occurrence := &WordOccurrence{Definition: tt.definition}
			got := occurrence.GetImages()
			assert.Equal(t, tt.want, got)
		})
	}
}
