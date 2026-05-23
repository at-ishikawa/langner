package notebook

import (
	"testing"

	"github.com/at-ishikawa/langner/internal/assets"
	"github.com/stretchr/testify/assert"
)

func TestCollapseDefinitionConceptsForExport_GroupsByHeadAndPrefersHead(t *testing.T) {
	notes := []Note{
		{Expression: "brighten", Meaning: "to make or become bright", PartOfSpeech: "verb"},
		{Expression: "bright", Meaning: "emitting much light", PartOfSpeech: "adjective"},
		{Expression: "brightness", Meaning: "the quality of being bright", PartOfSpeech: "noun"},
		{Expression: "unrelated", Meaning: "stands alone"},
	}
	byExpr := map[string]string{
		"bright": "bright", "brighten": "bright", "brightness": "bright",
	}
	byHead := map[string]DefinitionConceptInfo{
		"bright": {
			Head:    "bright",
			Meaning: "the quality of being bright",
			Members: []string{"bright", "brighten", "brightness"},
		},
	}

	got := collapseDefinitionConceptsForExport(notes, byExpr, byHead)
	assert.Len(t, got, 2, "three members collapse to one row; unrelated passes through")

	var conceptEntry, loneEntry assets.StoryNote
	for _, n := range got {
		if n.ConceptHead == "" {
			loneEntry = n
		} else {
			conceptEntry = n
		}
	}
	assert.Equal(t, "bright", conceptEntry.Expression, "head's row survives (upgraded from initial brighten)")
	assert.Equal(t, "emitting much light", conceptEntry.Meaning, "head's per-expression meaning preserved")
	assert.Equal(t, "the quality of being bright", conceptEntry.ConceptMeaning, "umbrella meaning attached")
	assert.ElementsMatch(t, []string{"bright", "brighten", "brightness"}, conceptEntry.ConceptMembers)
	assert.Equal(t, "unrelated", loneEntry.Expression)
	assert.Empty(t, loneEntry.ConceptHead)
}

func TestCollapseDefinitionConceptsForExport_NoConceptIndex_PassThrough(t *testing.T) {
	notes := []Note{
		{Expression: "alpha", Meaning: "first"},
		{Expression: "beta", Meaning: "second"},
	}
	got := collapseDefinitionConceptsForExport(notes, nil, nil)
	assert.Len(t, got, 2)
	assert.Empty(t, got[0].ConceptHead)
	assert.Empty(t, got[1].ConceptHead)
}
