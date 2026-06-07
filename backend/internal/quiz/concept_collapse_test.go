package quiz

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollapseConceptCards_GroupsByConceptHead(t *testing.T) {
	index := map[string]map[string]*conceptInfo{
		"vocab-book": {
			"bright":     {Head: "bright", Meaning: "the quality of being bright", Members: []string{"bright", "brighten", "brightness"}},
			"brighten":   {Head: "bright", Meaning: "the quality of being bright", Members: []string{"bright", "brighten", "brightness"}},
			"brightness": {Head: "bright", Meaning: "the quality of being bright", Members: []string{"bright", "brighten", "brightness"}},
		},
	}
	cards := []Card{
		{NotebookName: "vocab-book", Entry: "bright", Meaning: "emitting much light"},
		{NotebookName: "vocab-book", Entry: "brighten", Meaning: "to make or become bright"},
		{NotebookName: "vocab-book", Entry: "brightness", Meaning: "the quality of being bright"},
		{NotebookName: "vocab-book", Entry: "unrelated", Meaning: "stands alone"},
	}
	got := collapseConceptCards(cards, index)
	assert.Len(t, got, 2, "three concept members collapse to one card; the unrelated card survives")

	var conceptCard Card
	var loneCard Card
	for _, c := range got {
		if c.Entry == "unrelated" {
			loneCard = c
		} else {
			conceptCard = c
		}
	}
	assert.Equal(t, "bright", conceptCard.ConceptHead, "head-named card should be selected")
	assert.Equal(t, "bright", conceptCard.Entry, "displayed entry is the head")
	assert.Equal(t, "the quality of being bright", conceptCard.ConceptMeaning)
	assert.ElementsMatch(t, []string{"bright", "brighten", "brightness"}, conceptCard.ConceptMembers)
	assert.Empty(t, loneCard.ConceptHead, "non-concept cards keep an empty ConceptHead")
}

func TestCollapseConceptCards_NoIndex_PassesThrough(t *testing.T) {
	cards := []Card{
		{NotebookName: "vocab-book", Entry: "alpha"},
		{NotebookName: "vocab-book", Entry: "beta"},
	}
	got := collapseConceptCards(cards, nil)
	assert.Equal(t, cards, got, "no concept index should be a no-op")
}

func TestCollapseConceptCards_PrefersHeadCardWhenMemberAlreadySeen(t *testing.T) {
	// Member appears in the input before the head; collapse must
	// upgrade to use the head's row once it's encountered.
	index := map[string]map[string]*conceptInfo{
		"vocab-book": {
			"first":  {Head: "first", Meaning: "primary form", Members: []string{"first", "firstly", "firstness"}},
			"firstly": {Head: "first", Meaning: "primary form", Members: []string{"first", "firstly", "firstness"}},
			"firstness": {Head: "first", Meaning: "primary form", Members: []string{"first", "firstly", "firstness"}},
		},
	}
	cards := []Card{
		{NotebookName: "vocab-book", Entry: "firstly", Meaning: "as the first item"},
		{NotebookName: "vocab-book", Entry: "first", Meaning: "before any other"},
	}
	got := collapseConceptCards(cards, index)
	assert.Len(t, got, 1)
	assert.Equal(t, "first", got[0].Entry, "head row replaces the earlier member row")
	assert.Equal(t, "before any other", got[0].Meaning, "head's per-expression meaning is preserved")
	assert.Equal(t, "primary form", got[0].ConceptMeaning, "umbrella meaning is set from the concept")
}

func TestCollapseConceptReverseCards_GroupsByConceptHead(t *testing.T) {
	index := map[string]map[string]*conceptInfo{
		"vocab-book": {
			"bright":   {Head: "bright", Meaning: "the quality of being bright", Members: []string{"bright", "brighten"}},
			"brighten": {Head: "bright", Meaning: "the quality of being bright", Members: []string{"bright", "brighten"}},
		},
	}
	cards := []ReverseCard{
		{NotebookName: "vocab-book", Expression: "brighten", Meaning: "to make bright"},
		{NotebookName: "vocab-book", Expression: "bright", Meaning: "emitting much light"},
		{NotebookName: "vocab-book", Expression: "unrelated", Meaning: "stands alone"},
	}
	got := collapseConceptReverseCards(cards, index)
	assert.Len(t, got, 2)

	for _, c := range got {
		if c.Expression == "bright" {
			assert.Equal(t, "bright", c.ConceptHead)
			assert.Equal(t, "the quality of being bright", c.ConceptMeaning)
			assert.ElementsMatch(t, []string{"bright", "brighten"}, c.ConceptMembers)
		}
		if c.Expression == "unrelated" {
			assert.Empty(t, c.ConceptHead)
		}
	}
}
