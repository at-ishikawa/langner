package notebook

import (
	"strings"
	"testing"
	"time"

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

func TestApplyUserDefinitionsOverlay_DateFromUserRecency(t *testing.T) {
	// A shared book's chapters have no authored date; the overlay stamps the
	// user's LATEST registration time so the quiz hub orders the book by when
	// THIS user last engaged with it (per-user, not a shared import timestamp).
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	nbs, paths := sampleBook()
	out := ApplyUserDefinitionsOverlay("gutenberg-1", nbs, paths, []UserDefinition{
		{Event: "Chapter I", SceneIndex: 0, Expression: "lazy", Meaning: "x", CreatedAt: older},
		{Event: "Chapter I", SceneIndex: 0, Expression: "quick", Meaning: "y", CreatedAt: newer},
	})
	assert.True(t, out[0].Date.Equal(newer), "date-less book takes the user's latest registration time, got %v", out[0].Date)

	// A notebook that already carries an authored date keeps it (only date-less
	// book notebooks borrow the per-user recency).
	dated, dpaths := sampleBook()
	authored := time.Date(2025, 6, 6, 0, 0, 0, 0, time.UTC)
	dated[0].Date = authored
	out2 := ApplyUserDefinitionsOverlay("gutenberg-1", dated, dpaths, []UserDefinition{
		{Event: "Chapter I", SceneIndex: 0, Expression: "lazy", Meaning: "x", CreatedAt: newer},
	})
	assert.True(t, out2[0].Date.Equal(authored), "authored date must be preserved, got %v", out2[0].Date)
}
