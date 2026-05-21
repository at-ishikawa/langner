package notebook

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeDefinitionsBook writes a minimal indexed definitions book to disk
// with one index.yml plus the given session files (filename -> raw YAML
// body). Returns the parent directory containing the indexed bookID
// directory, suitable for passing as a definitionsDirs entry.
func makeDefinitionsBook(t *testing.T, bookID string, sessions map[string]string) string {
	t.Helper()
	tmp := t.TempDir()
	bookDir := filepath.Join(tmp, bookID)
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	paths := make([]string, 0, len(sessions))
	for name := range sessions {
		paths = append(paths, "./"+name)
	}
	sort.Strings(paths)

	indexBody := "id: " + bookID + "\nnotebooks:\n"
	for _, p := range paths {
		indexBody += "  - " + p + "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(indexBody), 0o644))

	for name, body := range sessions {
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, name), []byte(body), 0o644))
	}
	return tmp
}

func TestValidateDefinitionConcepts_HappyPath(t *testing.T) {
	dir := makeDefinitionsBook(t, "demo-book", map[string]string{
		"session1.yml": `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "Brightness"
      expressions:
        - expression: bright
          meaning: emitting much light
          part_of_speech: adjective
        - expression: brighten
          meaning: to make or become bright
          part_of_speech: verb
        - expression: brightness
          meaning: the quality of being bright
          part_of_speech: noun
  concepts:
    - head: bright
      meaning: the quality or act of being bright
      expressions:
        - bright
        - brighten
        - brightness
`,
	})

	v := NewValidator("", nil, nil, []string{dir}, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)

	assert.Empty(t, result.Warnings,
		"happy-path data should produce no warnings; got: %+v", result.Warnings)
}

func TestValidateDefinitionConcepts_EmptyHead(t *testing.T) {
	dir := makeDefinitionsBook(t, "demo-book", map[string]string{
		"session1.yml": `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: bright
          meaning: emitting much light
  concepts:
    - head: ""
      meaning: anything
      expressions:
        - bright
`,
	})

	v := NewValidator("", nil, nil, []string{dir}, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)

	assert.NotEmpty(t, result.Warnings)
	assert.NotEmpty(t, findWarning(result.Warnings, "concept.head must be non-empty"))
}

func TestValidateDefinitionConcepts_EmptyMeaning(t *testing.T) {
	dir := makeDefinitionsBook(t, "demo-book", map[string]string{
		"session1.yml": `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: bright
          meaning: emitting much light
  concepts:
    - head: bright
      meaning: ""
      expressions:
        - bright
`,
	})

	v := NewValidator("", nil, nil, []string{dir}, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)

	assert.NotEmpty(t, findWarning(result.Warnings, "concept.meaning must be non-empty"))
}

func TestValidateDefinitionConcepts_HeadNotInExpressions(t *testing.T) {
	dir := makeDefinitionsBook(t, "demo-book", map[string]string{
		"session1.yml": `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: bright
          meaning: emitting much light
        - expression: brighten
          meaning: to make bright
  concepts:
    - head: bright
      meaning: the quality of being bright
      expressions:
        - brighten
`,
	})

	v := NewValidator("", nil, nil, []string{dir}, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)

	assert.NotEmpty(t, findWarning(result.Warnings,
		`concept.head "bright" must appear in its own expressions[]`))
}

func TestValidateDefinitionConcepts_UnknownMember(t *testing.T) {
	dir := makeDefinitionsBook(t, "demo-book", map[string]string{
		"session1.yml": `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: bright
          meaning: emitting much light
  concepts:
    - head: bright
      meaning: the quality of being bright
      expressions:
        - bright
        - nonexistent
`,
	})

	v := NewValidator("", nil, nil, []string{dir}, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)

	assert.NotEmpty(t, findWarning(result.Warnings,
		`concept member "nonexistent" does not match any expression`))
}

func TestValidateDefinitionConcepts_MemberFromAnotherSession(t *testing.T) {
	// Members may live in a different session within the same book — this
	// is the notebook-scope guarantee. A concept declared in session1 may
	// list an expression that's only declared in session2 and still
	// validate cleanly.
	dir := makeDefinitionsBook(t, "demo-book", map[string]string{
		"session1.yml": `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: bright
          meaning: emitting much light
  concepts:
    - head: bright
      meaning: the quality of being bright
      expressions:
        - bright
        - brightness
`,
		"session2.yml": `- metadata:
    title: "Session 2"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: brightness
          meaning: the quality of being bright
`,
	})

	v := NewValidator("", nil, nil, []string{dir}, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)

	assert.Empty(t, result.Warnings,
		"cross-session members should resolve cleanly; got: %+v", result.Warnings)
}

func TestValidateDefinitionConcepts_MeaningDisagreement(t *testing.T) {
	// Two sessions declare the same head with different meanings. The
	// validator must report the disagreement (book-level merge would
	// otherwise silently pick one).
	dir := makeDefinitionsBook(t, "demo-book", map[string]string{
		"session1.yml": `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: bright
          meaning: emitting much light
  concepts:
    - head: bright
      meaning: meaning A
      expressions:
        - bright
`,
		"session2.yml": `- metadata:
    title: "Session 2"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: bright
          meaning: emitting much light
  concepts:
    - head: bright
      meaning: meaning B
      expressions:
        - bright
`,
	})

	v := NewValidator("", nil, nil, []string{dir}, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)

	assert.NotEmpty(t, findWarning(result.Warnings, "disagrees with earlier declaration"))
}

func TestValidateDefinitionConcepts_ExpressionInTwoConcepts(t *testing.T) {
	dir := makeDefinitionsBook(t, "demo-book", map[string]string{
		"session1.yml": `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: bright
          meaning: emitting much light
        - expression: brighten
          meaning: to make bright
        - expression: shine
          meaning: to emit light
  concepts:
    - head: bright
      meaning: the quality of being bright
      expressions:
        - bright
        - brighten
    - head: shine
      meaning: emission of light
      expressions:
        - shine
        - brighten
`,
	})

	v := NewValidator("", nil, nil, []string{dir}, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)

	assert.NotEmpty(t, findWarning(result.Warnings,
		`expression "brighten" belongs to multiple concepts`))
}

func TestValidateDefinitionConcepts_NoConceptsClean(t *testing.T) {
	// A book with no concepts: blocks at all must validate cleanly so
	// existing definitions don't fail validation just because the new
	// field exists.
	dir := makeDefinitionsBook(t, "demo-book", map[string]string{
		"session1.yml": `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: bright
          meaning: emitting much light
`,
	})

	v := NewValidator("", nil, nil, []string{dir}, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)

	assert.Empty(t, result.Warnings,
		"no-concepts data should produce no warnings; got: %+v", result.Warnings)
}

func TestValidateDefinitionConcepts_NoDirsConfigured(t *testing.T) {
	v := NewValidator("", nil, nil, nil, nil, "", nil)
	result := &ValidationResult{}
	v.validateDefinitionConcepts(result)
	assert.Empty(t, result.Warnings,
		"no definitions dirs configured should be a no-op; got: %+v", result.Warnings)
}
