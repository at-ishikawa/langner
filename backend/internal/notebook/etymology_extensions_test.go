package notebook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeEtymologyBook writes a minimal etymology book to disk with one
// index.yml plus the given session files (filename -> raw YAML body). The
// returned path is the directory containing the index. Use this for
// end-to-end tests of validateEtymologyExtensions.
func makeEtymologyBook(t *testing.T, bookID string, sessions map[string]string) string {
	t.Helper()
	tmp := t.TempDir()
	bookDir := filepath.Join(tmp, bookID)
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	var paths []string
	for name := range sessions {
		paths = append(paths, "./"+name)
	}
	indexBody := "id: " + bookID + "\nkind: Etymology\nname: \"" + bookID + "\"\nnotebooks:\n"
	for _, p := range paths {
		indexBody += "  - " + p + "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(indexBody), 0o644))

	for name, body := range sessions {
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, name), []byte(body), 0o644))
	}
	return tmp
}

// findWarning returns the first warning whose message contains substr, or
// "" if none match.
func findWarning(warnings []ValidationError, substr string) string {
	for _, w := range warnings {
		if strings.Contains(w.Message, substr) {
			return w.Message
		}
	}
	return ""
}

func TestValidateEtymologyExtensions_HappyPath(t *testing.T) {
	dir := makeEtymologyBook(t, "demo", map[string]string{
		"session1.yml": `metadata:
  title: "Session 1"
origins:
  - origin: dexter
    language: Latin
    meaning: right
  - origin: sinister
    language: Latin
    meaning: left
concepts:
  - key: leftness
    meaning: left
    members:
      - { origin: sinister, language: Latin }
  - key: rightness
    meaning: right
    members:
      - { origin: dexter, language: Latin }
relations:
  - { type: antonym, between: [leftness, rightness] }
`,
	})

	v := NewValidator("", nil, nil, nil, []string{dir}, "", nil)
	result := &ValidationResult{}
	v.validateEtymologyExtensions(result)

	assert.Empty(t, result.Warnings, "happy-path data should produce no warnings; got: %+v", result.Warnings)
}

func TestValidateEtymologyExtensions_UnknownMember(t *testing.T) {
	dir := makeEtymologyBook(t, "demo", map[string]string{
		"session1.yml": `metadata:
  title: "Session 1"
origins:
  - origin: dexter
    language: Latin
    meaning: right
concepts:
  - key: leftness
    meaning: left
    members:
      - { origin: missing-origin, language: Latin }
`,
	})

	v := NewValidator("", nil, nil, nil, []string{dir}, "", nil)
	result := &ValidationResult{}
	v.validateEtymologyExtensions(result)

	msg := findWarning(result.Warnings, "missing-origin")
	assert.NotEmpty(t, msg, "expected warning about unknown member; got: %+v", result.Warnings)
	assert.Contains(t, msg, "does not match any origin")
}

func TestValidateEtymologyExtensions_RelationBothShapes(t *testing.T) {
	dir := makeEtymologyBook(t, "demo", map[string]string{
		"session1.yml": `metadata:
  title: "Session 1"
origins:
  - origin: a
    language: Latin
    meaning: a
  - origin: b
    language: Latin
    meaning: b
concepts:
  - key: ka
    meaning: ka
    members: [{ origin: a, language: Latin }]
  - key: kb
    meaning: kb
    members: [{ origin: b, language: Latin }]
relations:
  - { type: antonym, between: [ka, kb], from: ka, to: kb }
`,
	})

	v := NewValidator("", nil, nil, nil, []string{dir}, "", nil)
	result := &ValidationResult{}
	v.validateEtymologyExtensions(result)

	msg := findWarning(result.Warnings, "either `between`")
	assert.NotEmpty(t, msg, "expected warning about mixed between/directed; got: %+v", result.Warnings)
}

func TestValidateEtymologyExtensions_RelationUnknownEndpoint(t *testing.T) {
	dir := makeEtymologyBook(t, "demo", map[string]string{
		"session1.yml": `metadata:
  title: "Session 1"
origins:
  - origin: a
    language: Latin
    meaning: a
concepts:
  - key: ka
    meaning: ka
    members: [{ origin: a, language: Latin }]
relations:
  - { type: antonym, between: [ka, never-declared] }
`,
	})

	v := NewValidator("", nil, nil, nil, []string{dir}, "", nil)
	result := &ValidationResult{}
	v.validateEtymologyExtensions(result)

	msg := findWarning(result.Warnings, "never-declared")
	assert.NotEmpty(t, msg, "expected warning about unknown endpoint; got: %+v", result.Warnings)
	assert.Contains(t, msg, "not a declared concept")
}

func TestValidateEtymologyExtensions_CrossSessionMeaningMismatch(t *testing.T) {
	dir := makeEtymologyBook(t, "demo", map[string]string{
		"session1.yml": `metadata:
  title: "Session 1"
origins:
  - { origin: a, language: Latin, meaning: a }
concepts:
  - key: shared
    meaning: first-meaning
    members: [{ origin: a, language: Latin }]
`,
		"session2.yml": `metadata:
  title: "Session 2"
origins:
  - { origin: b, language: Latin, meaning: b }
concepts:
  - key: shared
    meaning: second-meaning
    members: [{ origin: b, language: Latin }]
`,
	})

	v := NewValidator("", nil, nil, nil, []string{dir}, "", nil)
	result := &ValidationResult{}
	v.validateEtymologyExtensions(result)

	msg := findWarning(result.Warnings, "disagrees with earlier")
	assert.NotEmpty(t, msg, "expected warning about cross-session meaning mismatch; got: %+v", result.Warnings)
}

func TestValidateEtymologyExtensions_FormsBasic(t *testing.T) {
	dir := makeEtymologyBook(t, "demo", map[string]string{
		"session1.yml": `metadata:
  title: "Session 1"
origins:
  - origin: mittere
    language: Latin
    meaning: to send
    forms:
      - { form: mittere, role: present_active_infinitive }
      - { form: missum, role: supine }
`,
	})

	v := NewValidator("", nil, nil, nil, []string{dir}, "", nil)
	result := &ValidationResult{}
	v.validateEtymologyExtensions(result)

	assert.Empty(t, result.Warnings, "well-formed forms should not warn; got: %+v", result.Warnings)
}

func TestValidateFromForm(t *testing.T) {
	dir := makeEtymologyBook(t, "demo", map[string]string{
		"session1.yml": `metadata:
  title: "Session 1"
origins:
  - origin: mittere
    language: Latin
    meaning: to send
    forms:
      - { form: mittere, role: present_active_infinitive }
      - { form: missum, role: supine }
definitions:
  - expression: missile
    origin_parts:
      - { origin: mittere, language: Latin, from_form: missum }
  - expression: mistake
    origin_parts:
      - { origin: mittere, language: Latin, from_form: typo-form }
`,
	})

	v := NewValidator("", nil, nil, nil, []string{dir}, "", nil)
	result := &ValidationResult{}
	v.validateFromForm(result)

	msg := findWarning(result.Warnings, "typo-form")
	assert.NotEmpty(t, msg, "expected warning about unknown from_form; got: %+v", result.Warnings)
	// The valid (missum) reference should not warn.
	for _, w := range result.Warnings {
		assert.NotContains(t, w.Message, "missum", "valid from_form should not warn")
	}
}
