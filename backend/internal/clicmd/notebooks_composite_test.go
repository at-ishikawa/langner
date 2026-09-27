package clicmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveNotebookBundle_Composite verifies the CLI resolves the shipped
// composite example (examples/user-notebooks/roots-mini — a definitions family +
// an etymology family sharing one id) into a single compositeBundleKind bundle
// with family-prefixed paths, so `push` uploads it as one notebook (§13.6). No
// DB needed — this is the client bundle-resolution path.
func TestResolveNotebookBundle_Composite(t *testing.T) {
	dir := filepath.Join(findRepoRoot(t), "examples", "user-notebooks", "roots-mini")

	files, meta, err := resolveNotebookBundle(dir)
	require.NoError(t, err)
	assert.Equal(t, compositeBundleKind, meta.Kind, "a parent dir with family subdirs resolves as composite")
	assert.Equal(t, "roots-mini", meta.ID, "the shared family id is carried")

	byPath := map[string]bundleFile{}
	for _, f := range files {
		byPath[f.RelPath] = f
		assert.Truef(t, strings.HasPrefix(f.RelPath, f.Family+"/"),
			"path %q must be prefixed by its family %q", f.RelPath, f.Family)
	}
	for _, want := range []string{
		"definitions/index.yml", "definitions/definitions.yml",
		"etymology/index.yml", "etymology/origins.yml",
	} {
		_, ok := byPath[want]
		assert.Truef(t, ok, "composite bundle must include %s", want)
	}
	assert.Equal(t, "definitions", byPath["definitions/index.yml"].Family)
	assert.Equal(t, "etymology", byPath["etymology/origins.yml"].Family)
}
