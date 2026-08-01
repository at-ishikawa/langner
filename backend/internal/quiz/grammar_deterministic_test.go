package quiz

import (
	"testing"

	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestDeterministicGrammarGrade(t *testing.T) {
	tests := []struct {
		name        string
		answer      string
		correct     string
		incorrect   string
		wantDecided bool
		wantCorrect bool
	}{
		{"exact match", "John", "John", "the John", true, true},
		{"case and punctuation insensitive", "  john. ", "John", "the John", true, true},
		{"correct phrase inside a sentence", "I met John yesterday", "John", "the John", true, true},
		{"gerund fix", "suggested going", "suggested going", "suggested to go", true, true},
		{"still contains the mistake -> defer to LLM", "the John", "John", "the John", false, false},
		{"fix inside a longer answer is accepted", "a friend named John", "John", "the John", true, true},
		{"unrelated answer -> defer to LLM", "banana", "John", "the John", false, false},
		{"empty answer -> defer", "", "John", "the John", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, decided := deterministicGrammarGrade(tt.answer, tt.correct, tt.incorrect)
			if decided != tt.wantDecided {
				t.Fatalf("decided = %v, want %v (result %+v)", decided, tt.wantDecided, got)
			}
			if decided {
				if got.Correct != tt.wantCorrect {
					t.Fatalf("correct = %v, want %v", got.Correct, tt.wantCorrect)
				}
				if got.Correct && got.Quality != int(notebook.QualityCorrect) {
					t.Fatalf("quality = %d, want %d", got.Quality, int(notebook.QualityCorrect))
				}
			}
		})
	}
}
