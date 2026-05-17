package notebook

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateEtymologyExtensions_RepoExample loads the worked example
// notebook checked into the repo (`examples/etymology/common-roots/`) and
// asserts validation produces no warnings. This pins the example as a
// living spec: if a future change makes the schema stricter, this test
// flags that the example needs updating alongside it.
func TestValidateEtymologyExtensions_RepoExample(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) failed")
	// from backend/internal/notebook/.. = backend; backend/.. = repo root
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	exampleDir := filepath.Join(repoRoot, "examples", "etymology")

	v := NewValidator("", nil, nil, nil, []string{exampleDir}, "", nil)
	result := &ValidationResult{}
	v.validateEtymologyExtensions(result)
	v.validateFromForm(result)

	assert.Empty(
		t,
		result.Warnings,
		"the in-repo example etymology notebook should validate cleanly; got: %+v",
		result.Warnings,
	)
}
