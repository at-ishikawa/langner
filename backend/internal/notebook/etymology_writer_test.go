package notebook

import (
	"os"
	"path/filepath"
	"testing"

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

			writer := NewEtymologyNotebookWriter(reader, "", definitionsDirs)
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
