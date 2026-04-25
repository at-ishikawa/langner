package notebook

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEtymologyNotebookWriter_OutputEtymologyNotebook(t *testing.T) {
	tests := []struct {
		name           string
		setupFiles     func(t *testing.T, tmpDir string)
		etymologyID    string
		wantErr        string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:        "etymology not found",
			etymologyID: "nonexistent",
			setupFiles:  func(t *testing.T, tmpDir string) {},
			wantErr:     "not found",
		},
		{
			name:        "origins only without definitions",
			etymologyID: "latin-roots",
			setupFiles: func(t *testing.T, tmpDir string) {
				etymDir := filepath.Join(tmpDir, "etymology", "latin-roots")
				require.NoError(t, os.MkdirAll(etymDir, 0755))

				indexYAML := `id: latin-roots
kind: Etymology
name: Latin Roots
notebooks:
  - ./session1.yml
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(indexYAML), 0644))

				sessionYAML := `origins:
  - origin: spect
    language: Latin
    meaning: to look
  - origin: duc
    language: Latin
    meaning: to lead
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session1.yml"), []byte(sessionYAML), 0644))
			},
			wantContains: []string{
				"# Latin Roots",
				"**spect** [Latin]: to look",
				"**duc** [Latin]: to lead",
			},
		},
		{
			name:        "origins with matching definitions",
			etymologyID: "test-etymology",
			setupFiles: func(t *testing.T, tmpDir string) {
				// Create etymology directory with origins
				etymDir := filepath.Join(tmpDir, "etymology", "test-etymology")
				require.NoError(t, os.MkdirAll(etymDir, 0755))

				indexYAML := `id: test-etymology
kind: Etymology
name: Test Etymology
notebooks:
  - ./session1.yml
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(indexYAML), 0644))

				sessionYAML := `origins:
  - origin: graph
    language: Greek
    meaning: to write
  - origin: logos
    language: Greek
    meaning: word or study
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session1.yml"), []byte(sessionYAML), 0644))

				// Create definitions directory with matching session
				defDir := filepath.Join(tmpDir, "definitions", "books", "test-etymology-vocab")
				require.NoError(t, os.MkdirAll(defDir, 0755))

				defIndex := `id: test-etymology-vocab
notebooks:
  - ./session1.yml
`
				require.NoError(t, os.WriteFile(filepath.Join(defDir, "index.yml"), []byte(defIndex), 0644))

				defYAML := `- metadata:
    title: "graph (to write)"
  scenes:
    - metadata:
        title: "graph (to write)"
      expressions:
        - expression: graphologist
          meaning: "analyzes handwriting"
          origin_parts:
            - origin: graph
            - origin: logos
        - expression: calligraphy
          meaning: "beautiful handwriting"
          pronunciation: "kuh-LIG-ruh-fee"
          part_of_speech: noun
          note: "Often used for decorative writing"
          origin_parts:
            - origin: graph
`
				require.NoError(t, os.WriteFile(filepath.Join(defDir, "session1.yml"), []byte(defYAML), 0644))
			},
			wantContains: []string{
				"# Test Etymology",
				"### graph (to write)",
				"**graph** [Greek]: to write",
				"**logos** [Greek]: word or study",
				"**graphologist**",
				"analyzes handwriting",
				"*graph* (to write)",
				"*logos* (word or study)",
				"**calligraphy**",
				"/kuh-LIG-ruh-fee/",
				"[noun]",
				"beautiful handwriting",
				"Note: Often used for decorative writing",
			},
			wantNotContain: []string{
				"### Words",
			},
		},
		{
			name:        "session title with multiple sections",
			etymologyID: "multi-section",
			setupFiles: func(t *testing.T, tmpDir string) {
				etymDir := filepath.Join(tmpDir, "etymology", "multi-section")
				require.NoError(t, os.MkdirAll(etymDir, 0755))

				indexYAML := `id: multi-section
kind: Etymology
name: Word Roots
notebooks:
  - ./session1.yml
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(indexYAML), 0644))

				sessionYAML := `origins:
  - origin: graph
    language: Greek
    meaning: to write
  - origin: duc
    language: Latin
    meaning: to lead
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session1.yml"), []byte(sessionYAML), 0644))

				defDir := filepath.Join(tmpDir, "definitions", "books", "multi-section-vocab")
				require.NoError(t, os.MkdirAll(defDir, 0755))

				defIndex := `id: multi-section-vocab
notebooks:
  - ./session1.yml
`
				require.NoError(t, os.WriteFile(filepath.Join(defDir, "index.yml"), []byte(defIndex), 0644))

				defYAML := `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        title: "graph (to write)"
      expressions:
        - expression: autograph
          meaning: "self-writing; a signature"
          origin_parts:
            - origin: graph
    - metadata:
        title: "duc (to lead)"
      expressions:
        - expression: conduct
          meaning: "to lead together"
          origin_parts:
            - origin: duc
`
				require.NoError(t, os.WriteFile(filepath.Join(defDir, "session1.yml"), []byte(defYAML), 0644))
			},
			wantContains: []string{
				"# Word Roots",
				"## Session 1",
				"### graph (to write)",
				"**autograph**",
				"self-writing; a signature",
				"### duc (to lead)",
				"**conduct**",
				"to lead together",
			},
			wantNotContain: []string{
				"### Words",
			},
		},
		{
			name:        "merges entries with same session title into one chapter",
			etymologyID: "merge-test",
			setupFiles: func(t *testing.T, tmpDir string) {
				etymDir := filepath.Join(tmpDir, "etymology", "merge-test")
				require.NoError(t, os.MkdirAll(etymDir, 0755))

				indexYAML := `id: merge-test
kind: Etymology
name: Merge Test
notebooks:
  - ./session1.yml
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(indexYAML), 0644))

				sessionYAML := `origins:
  - origin: ego
    language: Latin
    meaning: I, self
  - origin: alter
    language: Latin
    meaning: other
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session1.yml"), []byte(sessionYAML), 0644))

				defDir := filepath.Join(tmpDir, "definitions", "books", "merge-test-vocab")
				require.NoError(t, os.MkdirAll(defDir, 0755))

				defIndex := `id: merge-test-vocab
notebooks:
  - ./session1.yml
`
				require.NoError(t, os.WriteFile(filepath.Join(defDir, "index.yml"), []byte(defIndex), 0644))

				// Two entries with the same title "Session 1", each with a different section
				defYAML := `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        title: "ego (I, self)"
      expressions:
        - expression: egoist
          meaning: "one whose primary concern is self-interest"
          origin_parts:
            - origin: ego
- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        title: "alter (other)"
      expressions:
        - expression: altruist
          meaning: "one who puts others first"
          origin_parts:
            - origin: alter
`
				require.NoError(t, os.WriteFile(filepath.Join(defDir, "session1.yml"), []byte(defYAML), 0644))
			},
			wantContains: []string{
				"## Session 1",
				"### ego (I, self)",
				"**egoist**",
				"### alter (other)",
				"**altruist**",
			},
			wantNotContain: []string{
				"### Words",
			},
		},
		{
			name:        "falls back to Words heading when scenes have no titles",
			etymologyID: "no-scene-title",
			setupFiles: func(t *testing.T, tmpDir string) {
				etymDir := filepath.Join(tmpDir, "etymology", "no-scene-title")
				require.NoError(t, os.MkdirAll(etymDir, 0755))

				indexYAML := `id: no-scene-title
kind: Etymology
name: Basic Roots
notebooks:
  - ./session1.yml
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(indexYAML), 0644))

				sessionYAML := `origins:
  - origin: spect
    language: Latin
    meaning: to look
`
				require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session1.yml"), []byte(sessionYAML), 0644))

				defDir := filepath.Join(tmpDir, "definitions", "books", "no-scene-title-vocab")
				require.NoError(t, os.MkdirAll(defDir, 0755))

				defIndex := `id: no-scene-title-vocab
notebooks:
  - ./session1.yml
`
				require.NoError(t, os.WriteFile(filepath.Join(defDir, "index.yml"), []byte(defIndex), 0644))

				defYAML := `- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: spectator
          meaning: "one who looks"
          origin_parts:
            - origin: spect
`
				require.NoError(t, os.WriteFile(filepath.Join(defDir, "session1.yml"), []byte(defYAML), 0644))
			},
			wantContains: []string{
				"## Session 1",
				"### Words",
				"**spectator**",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setupFiles(t, tmpDir)

			outputDir := filepath.Join(tmpDir, "output")
			etymologyDirs := []string{filepath.Join(tmpDir, "etymology")}
			definitionsDirs := []string{filepath.Join(tmpDir, "definitions")}

			reader, err := NewReader(nil, nil, nil, definitionsDirs, etymologyDirs, nil)
			require.NoError(t, err)

			writer := NewEtymologyNotebookWriter(reader, "", definitionsDirs, nil)
			err = writer.OutputEtymologyNotebook(tt.etymologyID, outputDir, false)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)

			// Read the output file
			outputPath := filepath.Join(outputDir, tt.etymologyID+".md")
			content, err := os.ReadFile(outputPath)
			require.NoError(t, err)

			for _, want := range tt.wantContains {
				assert.Contains(t, string(content), want, "output should contain %q", want)
			}
			for _, notWant := range tt.wantNotContain {
				assert.NotContains(t, string(content), notWant, "output should not contain %q", notWant)
			}
		})
	}
}

// TestEtymologyNotebookWriter_HidesSectionsWhoseOriginsAreMastered pins the
// expected behavior for the markdown export: when every origin referenced by
// every word in a section has been mastered (latest log is non-misunderstood
// and the next review date is still in the future), the entire section — its
// header AND its words — is omitted from the output. Sections with at least
// one origin that still needs study are kept in full so the user can review
// them in context.
func TestEtymologyNotebookWriter_HidesSectionsWhoseOriginsAreMastered(t *testing.T) {
	tmpDir := t.TempDir()

	etymDir := filepath.Join(tmpDir, "etymology", "test-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: test-roots
kind: Etymology
name: Test Roots
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session1.yml"), []byte(`origins:
  - origin: mastered-root
    language: Latin
    meaning: well-known
  - origin: open-root
    language: Latin
    meaning: needs work
`), 0o644))

	defDir := filepath.Join(tmpDir, "definitions", "books", "test-roots-vocab")
	require.NoError(t, os.MkdirAll(defDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defDir, "index.yml"), []byte(`id: test-roots-vocab
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        title: "mastered-root (well-known)"
      expressions:
        - expression: mastered-word
          meaning: "uses only the mastered root"
          origin_parts:
            - origin: mastered-root
    - metadata:
        title: "open-root (needs work)"
      expressions:
        - expression: open-word
          meaning: "uses the root that still needs study"
          origin_parts:
            - origin: open-root
`), 0o644))

	// Learning history: mastered-root has a recent correct answer with a long
	// interval (next review far in the future). open-root has no etymology
	// history at all.
	learningHistories := map[string][]LearningHistory{
		"test-roots": {{
			Metadata: LearningHistoryMetadata{NotebookID: "test-roots", Title: "Test Roots", Type: "etymology"},
			Expressions: []LearningHistoryExpression{{
				Expression: "mastered-root",
				EtymologyBreakdownLogs: []LearningRecord{{
					Status:       LearnedStatusUnderstood,
					LearnedAt:    Date{Time: time.Now()},
					Quality:      5,
					QuizType:     string(QuizTypeEtymologyStandard),
					IntervalDays: 365,
				}},
			}},
		}},
	}

	outputDir := filepath.Join(tmpDir, "output")
	reader, err := NewReader(nil, nil, nil,
		[]string{filepath.Join(tmpDir, "definitions")},
		[]string{filepath.Join(tmpDir, "etymology")}, nil)
	require.NoError(t, err)

	writer := NewEtymologyNotebookWriter(reader, "",
		[]string{filepath.Join(tmpDir, "definitions")}, learningHistories)
	require.NoError(t, writer.OutputEtymologyNotebook("test-roots", outputDir, false))

	content, err := os.ReadFile(filepath.Join(outputDir, "test-roots.md"))
	require.NoError(t, err)
	out := string(content)

	assert.NotContains(t, out, "mastered-root", "mastered origin must not appear anywhere in the export — not as an origin entry, not as a section header, not as a word's origin reference")
	assert.NotContains(t, out, "mastered-word", "a word whose origin parts are all mastered must be omitted")

	assert.Contains(t, out, "### open-root (needs work)", "sections with at least one origin still needing study must remain")
	assert.Contains(t, out, "**open-word**", "words with at least one origin still needing study must remain")
}

func TestEtymologyNotebookWriter_buildOriginMap(t *testing.T) {
	origins := []EtymologyOrigin{
		{Origin: "graph", Language: "Greek", Meaning: "to write"},
		{Origin: "logos", Language: "Greek", Meaning: "word or study"},
	}

	got := buildOriginMap(origins)
	assert.Equal(t, "to write", got["graph"])
	assert.Equal(t, "word or study", got["logos"])
	assert.Equal(t, "", got["nonexistent"])
}
