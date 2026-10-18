package notebook

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleBook returns a one-chapter "book" story notebook with a single scene
// that already declares one content (shipped) definition, plus the parallel
// notebookPaths slice ReadStoryNotebooksWithPaths would return. Public/generic
// data only (never real user notebooks).
func sampleBook() ([]StoryNotebook, []string) {
	nbs := []StoryNotebook{
		{
			Event: "Chapter I",
			Scenes: []StoryScene{
				{
					Title:      "Chapter I",
					Statements: []string{"The quick brown fox jumped over the lazy dog."},
					// A content-declared (shipped) definition every reader sees.
					Definitions: []Note{{Expression: "fox", Meaning: "a wild animal"}},
				},
			},
		},
	}
	return nbs, []string{"Chapter I"}
}

func TestBuildUserDefinitionsMap_KeyedByEventAndSceneIndex(t *testing.T) {
	rows := []UserDefinition{
		{Event: "Chapter I", SceneIndex: 0, Expression: "lazy", Meaning: "unwilling to work"},
	}
	m := BuildUserDefinitionsMap("gutenberg-1", rows)
	require.Contains(t, m, "gutenberg-1")
	require.Contains(t, m["gutenberg-1"], "Chapter I")
	notes := m["gutenberg-1"]["Chapter I"]["__index_0"]
	require.Len(t, notes, 1)
	assert.Equal(t, "lazy", notes[0].Expression)
	assert.Equal(t, "unwilling to work", notes[0].Meaning)
}

func TestBuildUserDefinitionsMap_EmptyRows(t *testing.T) {
	assert.Nil(t, BuildUserDefinitionsMap("gutenberg-1", nil))
}

func TestApplyUserDefinitionsOverlay_AdditiveOnTopOfContentDefs(t *testing.T) {
	nbs, paths := sampleBook()
	rows := []UserDefinition{
		{Event: "Chapter I", SceneIndex: 0, Expression: "lazy", Meaning: "unwilling to work"},
	}

	out := ApplyUserDefinitionsOverlay("gutenberg-1", nbs, paths, rows)

	defs := out[0].Scenes[0].Definitions
	exprs := make([]string, 0, len(defs))
	for _, d := range defs {
		exprs = append(exprs, d.Expression)
	}
	// Overlay ADDS the user's word while PRESERVING the shipped one for everyone.
	assert.Contains(t, exprs, "fox", "content-declared definition must be preserved")
	assert.Contains(t, exprs, "lazy", "user registration must be overlaid")

	// The reader bolds registered words by wrapping them in {{ }} markers in the
	// statements — the bridge that makes a highlight visible.
	assert.True(t, strings.Contains(out[0].Scenes[0].Statements[0], "{{ lazy }}"),
		"registered word should be marked for bolding, got: %q", out[0].Scenes[0].Statements[0])
}

func TestApplyUserDefinitionsOverlay_PerUserIsolation(t *testing.T) {
	// User A registered "lazy"; user B registered nothing. Building each user's
	// overlay from THEIR OWN rows yields A's word for A and nothing extra for B.
	nbsA, pathsA := sampleBook()
	outA := ApplyUserDefinitionsOverlay("gutenberg-1", nbsA, pathsA, []UserDefinition{
		{Event: "Chapter I", SceneIndex: 0, Expression: "lazy", Meaning: "unwilling to work"},
	})
	nbsB, pathsB := sampleBook()
	outB := ApplyUserDefinitionsOverlay("gutenberg-1", nbsB, pathsB, nil)

	exprs := func(nbs []StoryNotebook) []string {
		var out []string
		for _, d := range nbs[0].Scenes[0].Definitions {
			out = append(out, d.Expression)
		}
		return out
	}
	assert.Contains(t, exprs(outA), "lazy")
	// B sees only the shared content definition, never A's private registration.
	assert.NotContains(t, exprs(outB), "lazy")
	assert.Contains(t, exprs(outB), "fox")
}

func TestApplyUserDefinitionsOverlay_NoRowsReturnsUnchanged(t *testing.T) {
	nbs, paths := sampleBook()
	out := ApplyUserDefinitionsOverlay("gutenberg-1", nbs, paths, nil)
	assert.Len(t, out[0].Scenes[0].Definitions, 1)
	assert.Equal(t, "fox", out[0].Scenes[0].Definitions[0].Expression)
}
