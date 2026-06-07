package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
)

// TestMaskEtymologyGraphFutureAnswers pins the reverse-quiz forward-mask:
// a concept cluster rendered for an early card must hide any sibling that
// a LATER card will ask, while earlier (already-answered) cards stay
// visible — and the blank's own prompt is scrubbed of a future answer it
// happens to name.
func TestMaskEtymologyGraphFutureAnswers(t *testing.T) {
	// Two cards on the same "writing" concept: card 0 blanks scribo,
	// card 1 blanks scripta. Each card's graph shows the whole cluster.
	clusterFor := func(blankID string) *apiv1.GraphPrompt {
		return &apiv1.GraphPrompt{
			Shape:       apiv1.GraphPrompt_CLUSTER,
			BlankNodeId: blankID,
			Nodes: []*apiv1.GraphNode{
				{Id: "concept", Kind: apiv1.GraphNode_CONCEPT, Label: "to write"},
				{
					Id: "m0", Kind: apiv1.GraphNode_ORIGIN,
					Label:   map[bool]string{true: "", false: "scribo"}[blankID == "m0"],
					Hint:    map[bool]string{true: "to write", false: ""}[blankID == "m0"],
					Meaning: map[bool]string{true: "", false: "to write"}[blankID == "m0"],
				},
				{
					Id: "m1", Kind: apiv1.GraphNode_ORIGIN,
					Label: map[bool]string{true: "", false: "scripta"}[blankID == "m1"],
					Hint:  map[bool]string{true: "written (past participle of scribo)", false: ""}[blankID == "m1"],
					// scripta's meaning names scribo — a future answer for
					// card 1, so it must be scrubbed there.
					Meaning: map[bool]string{true: "", false: "written (past participle of scribo)"}[blankID == "m1"],
				},
			},
		}
	}

	cards := []*apiv1.EtymologyQuizCard{
		{Origin: "scribo", GraphPrompt: clusterFor("m0")},
		{Origin: "scripta", GraphPrompt: clusterFor("m1")},
	}

	maskEtymologyGraphFutureAnswers(cards)

	// Card 0 (blanks scribo): scripta is a FUTURE answer → its sibling
	// node label must be masked.
	card0 := cards[0].GraphPrompt
	var m1OnCard0 *apiv1.GraphNode
	for _, n := range card0.Nodes {
		if n.Id == "m1" {
			m1OnCard0 = n
		}
	}
	require.NotNil(t, m1OnCard0)
	assert.Equal(t, graphAnswerMask, m1OnCard0.Label,
		"scripta is asked later, so its label must be masked on the earlier card")

	// Card 1 (blanks scripta): scribo was asked EARLIER (card 0), so the
	// scribo sibling stays visible...
	card1 := cards[1].GraphPrompt
	var m0OnCard1, blankOnCard1 *apiv1.GraphNode
	for _, n := range card1.Nodes {
		switch n.Id {
		case "m0":
			m0OnCard1 = n
		case "m1":
			blankOnCard1 = n
		}
	}
	require.NotNil(t, m0OnCard1)
	assert.Equal(t, "scribo", m0OnCard1.Label,
		"scribo was already asked, so it stays visible on the later card")

	// ...but the blank's own prompt on card 1 names scribo, and scribo is
	// NOT a future answer relative to card 1 (it's earlier), so the prompt
	// is left intact here.
	require.NotNil(t, blankOnCard1)
	assert.Contains(t, blankOnCard1.Hint, "scribo",
		"card 1's prompt may keep scribo — scribo is an earlier answer, not a future one")
}

// TestMaskEtymologyGraphFutureAnswers_ScrubsPromptLeak checks the prompt-
// scrub path: when the blank's meaning names an origin asked LATER, that
// origin is masked out of the prompt while the rest of the prompt stays.
func TestMaskEtymologyGraphFutureAnswers_ScrubsPromptLeak(t *testing.T) {
	// Reverse order: card 0 blanks scripta (prompt names scribo), card 1
	// blanks scribo. scribo is now a FUTURE answer → scrub it from card
	// 0's prompt.
	cards := []*apiv1.EtymologyQuizCard{
		{
			Origin: "scripta",
			GraphPrompt: &apiv1.GraphPrompt{
				Shape:       apiv1.GraphPrompt_CLUSTER,
				BlankNodeId: "m1",
				Nodes: []*apiv1.GraphNode{
					{Id: "concept", Kind: apiv1.GraphNode_CONCEPT, Label: "to write"},
					{Id: "m0", Kind: apiv1.GraphNode_ORIGIN, Label: "scribo", Meaning: "to write"},
					{Id: "m1", Kind: apiv1.GraphNode_ORIGIN, Label: "", Hint: "written (past participle of scribo)"},
				},
			},
		},
		{Origin: "scribo", GraphPrompt: &apiv1.GraphPrompt{
			Shape:       apiv1.GraphPrompt_CLUSTER,
			BlankNodeId: "m0",
			Nodes: []*apiv1.GraphNode{
				{Id: "concept", Kind: apiv1.GraphNode_CONCEPT, Label: "to write"},
				{Id: "m0", Kind: apiv1.GraphNode_ORIGIN, Label: "", Hint: "to write"},
				{Id: "m1", Kind: apiv1.GraphNode_ORIGIN, Label: "scripta", Meaning: "written"},
			},
		}},
	}

	maskEtymologyGraphFutureAnswers(cards)

	var blank, sibling *apiv1.GraphNode
	for _, n := range cards[0].GraphPrompt.Nodes {
		switch n.Id {
		case "m1":
			blank = n
		case "m0":
			sibling = n
		}
	}
	require.NotNil(t, blank)
	require.NotNil(t, sibling)
	// scribo (future answer) is masked out of the prompt, but the rest of
	// the prompt remains so the user can still guess scripta.
	assert.NotContains(t, blank.Hint, "scribo",
		"scribo is asked later → must be scrubbed from the prompt")
	assert.Contains(t, blank.Hint, "past participle",
		"the rest of the prompt stays so the word is still guessable")
	// The visible scribo sibling node is also masked (it IS the next answer).
	assert.Equal(t, graphAnswerMask, sibling.Label)
}
