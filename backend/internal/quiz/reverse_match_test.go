package quiz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/inference"
)

func TestMatchReverseAnswer(t *testing.T) {
	const misspelled = reverseReasonMisspelled
	tests := []struct {
		name       string
		accepted   []string
		answer     string
		wantOK     bool
		wantClass  inference.ValidateWordFormClassification
		wantReason string
	}{
		{"exact word", []string{"harbour"}, "harbour", true, inference.ClassificationSameWord, reverseReasonExact},
		{"case and trailing punctuation", []string{"harbour"}, "Harbour.", true, inference.ClassificationSameWord, reverseReasonExact},
		{"the example's own form", []string{"step on someone's toes", "stepped on my toes"}, "stepped on my toes", true, inference.ClassificationSameWord, reverseReasonExact},
		{"irregular highlighted form", []string{"go", "went"}, "went", true, inference.ClassificationSameWord, reverseReasonExact},
		{"curly apostrophe and spacing", []string{"keep one's cool"}, "  keep one’s   cool ", true, inference.ClassificationSameWord, reverseReasonExact},
		{"pronoun for one's plus tense", []string{"keep one's cool"}, "keeps her cool", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"the example's highlighted irregular form", []string{"keep one's cool", "kept her cool"}, "kept her cool", true, inference.ClassificationSameWord, reverseReasonExact},
		{"pronoun for someone's plus doubled consonant", []string{"step on someone's toes"}, "stepped on his toes", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"named possessive for someone's", []string{"step on someone's toes"}, "step on Sam's toes", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"reflexive for oneself", []string{"hurt oneself"}, "hurt himself", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"object pronoun for someone", []string{"let someone down"}, "let them down", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"optional part omitted", []string{"(get) cold feet"}, "cold feet", true, inference.ClassificationSameWord, reverseReasonExact},
		{"optional part kept", []string{"(get) cold feet"}, "get cold feet", true, inference.ClassificationSameWord, reverseReasonExact},
		{"typo in a multi-word expression", []string{"spill the beans"}, "spill the beens", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"inflection plus typo", []string{"spill the beans"}, "spilled the beens", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"regular inflection of a single word", []string{"harbour"}, "harboured", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"y to ied", []string{"carry"}, "carried", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"dropped e before ing", []string{"make up"}, "making up", true, inference.ClassificationSameWord, reverseReasonSameForm},
		{"misspelled single word", []string{"millennium"}, "millenium", true, inference.ClassificationWrong, misspelled},
		{"misspelled inflection of a single word", []string{"accommodate"}, "acommodated", true, inference.ClassificationWrong, misspelled},
		{"correct single word", []string{"millennium"}, "millennium", true, inference.ClassificationSameWord, reverseReasonExact},

		// Not certain: left to the LLM.
		{"changed preposition", []string{"step on someone's toes", "stepped on my toes"}, "stepped over my toes", false, "", ""},
		{"irregular form", []string{"pay one's way"}, "paid my way", false, "", ""},
		{"irregular form not on the card", []string{"keep one's cool"}, "kept her cool", false, "", ""},
		{"different short real word", []string{"affect"}, "effect", false, "", ""},
		{"spelling variant", []string{"realize"}, "realise", false, "", ""},
		{"spelling variant our", []string{"colour"}, "color", false, "", ""},
		{"typo in a single word is never accepted", []string{"harbour"}, "harbor", false, "", ""},
		{"placeholder gets no typo tolerance", []string{"keep one's cool"}, "keep ones cool", false, "", ""},
		{"short word gets no typo tolerance", []string{"pay one's way"}, "pat my way", false, "", ""},
		{"different word count", []string{"spill the beans"}, "spill beans", false, "", ""},
		{"synonym", []string{"happy"}, "joyful", false, "", ""},
		{"no accepted forms", nil, "anything", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := matchReverseAnswer(tt.accepted, tt.answer, 1000)
			require.Equal(t, tt.wantOK, ok, "got %+v", got)
			if !ok {
				return
			}
			assert.Equal(t, string(tt.wantClass), got.Classification)
			assert.Equal(t, tt.wantClass == inference.ClassificationSameWord, got.Correct)
			assert.Equal(t, tt.wantReason, got.Reason)
			assert.Equal(t, reverseQuality(got.Correct, 1000), got.Quality)
		})
	}
}

func TestReverseAcceptedForms(t *testing.T) {
	t.Run("lemma, alt form and shown highlights, deduplicated", func(t *testing.T) {
		got := reverseAcceptedForms(ReverseCard{
			Expression: "go",
			AltForm:    "Go",
			Contexts: []ReverseContext{
				{Context: "We went home.", Highlight: "went"},
				{Context: "She goes there.", Highlight: " "},
				{Context: "They went out.", Highlight: "went"},
			},
		})
		assert.Equal(t, []string{"go", "went"}, got)
	})
	t.Run("family concept members are accepted", func(t *testing.T) {
		got := reverseAcceptedForms(ReverseCard{Expression: "cardiograph", ConceptHead: "cardiograph", ConceptMembers: []string{"cardiograph", "cardiogram"}})
		assert.Equal(t, []string{"cardiograph", "cardiogram"}, got)
	})
	t.Run("display-only concept members are not accepted", func(t *testing.T) {
		got := reverseAcceptedForms(ReverseCard{Expression: "hot", ConceptMembers: []string{"hot", "cold"}})
		assert.Equal(t, []string{"hot"}, got)
	})
}
