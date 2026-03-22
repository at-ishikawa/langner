package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestExtractDefinitions(t *testing.T) {
	tests := []struct {
		name           string
		setupFiles     map[string]map[string]string // storyID -> filename -> content
		wantDefsFiles  bool
		wantDefsScenes int
		wantModified   map[string]bool // storyID -> whether story file should be modified
	}{
		{
			name: "extract definitions from story with conversations",
			setupFiles: map[string]map[string]string{
				"daily-conversations": {
					"index.yml": `id: daily-conversations
notebooks:
  - ./book.yml`,
					"book.yml": `- event: 'LESSON 1'
  date: 2026-01-19T00:00:00Z
  scenes:
    - scene: |
        Two friends meet at a cafe.
      conversations:
        - speaker: Alice
          quote: Let's {{ break the ice }} and get to know each other.
        - speaker: Bob
          quote: Sure, I didn't mean to {{ lose my temper }} earlier.
      definitions:
        - expression: break the ice
          meaning: to initiate a conversation in a social setting
        - expression: lose my temper
          definition: lose one's temper
          meaning: to become angry`,
				},
			},
			wantDefsFiles:  true,
			wantDefsScenes: 1,
			wantModified:   map[string]bool{"daily-conversations": true},
		},
		{
			name: "extract definitions from story with statements",
			setupFiles: map[string]map[string]string{
				"common-idioms": {
					"index.yml": `id: common-idioms
notebooks:
  - ./chapter1.yml`,
					"chapter1.yml": `- event: 'Chapter 1'
  date: 2026-02-01T00:00:00Z
  scenes:
    - scene: |
        Practice common expressions.
      statements:
        - He decided to {{ hit the road }} before sunset.
      definitions:
        - expression: hit the road
          meaning: to leave or begin a journey`,
				},
			},
			wantDefsFiles:  true,
			wantDefsScenes: 1,
			wantModified:   map[string]bool{"common-idioms": true},
		},
		{
			name: "skip story without definitions",
			setupFiles: map[string]map[string]string{
				"no-defs-story": {
					"index.yml": `id: no-defs-story
notebooks:
  - ./book.yml`,
					"book.yml": `- event: 'LESSON 1'
  date: 2026-01-19T00:00:00Z
  scenes:
    - scene: |
        A simple scene.
      conversations:
        - speaker: Alice
          quote: Hello, how are you?`,
				},
			},
			wantDefsFiles: false,
			wantModified:  map[string]bool{"no-defs-story": false},
		},
		{
			name: "skip index.yml files",
			setupFiles: map[string]map[string]string{
				"test-story": {
					"index.yml": `id: test-story
notebooks:
  - ./book.yml`,
					"book.yml": `- event: 'LESSON 1'
  date: 2026-01-19T00:00:00Z
  scenes:
    - scene: |
        A scene with idioms.
      conversations:
        - speaker: Alice
          quote: That was a {{ piece of cake }}.
      definitions:
        - expression: piece of cake
          meaning: something very easy to do`,
				},
			},
			wantDefsFiles:  true,
			wantDefsScenes: 1,
			wantModified:   map[string]bool{"test-story": true},
		},
		{
			name: "multiple scenes with definitions",
			setupFiles: map[string]map[string]string{
				"multi-scene": {
					"index.yml": `id: multi-scene
notebooks:
  - ./book.yml`,
					"book.yml": `- event: 'LESSON 1'
  date: 2026-01-19T00:00:00Z
  scenes:
    - scene: |
        Scene one.
      conversations:
        - speaker: Alice
          quote: Don't {{ beat around the bush }}.
      definitions:
        - expression: beat around the bush
          meaning: to avoid talking about the main topic
    - scene: |
        Scene two.
      conversations:
        - speaker: Bob
          quote: I need to {{ pull myself together }}.
      definitions:
        - expression: pull myself together
          definition: pull oneself together
          meaning: to calm down and regain composure`,
				},
			},
			wantDefsFiles:  true,
			wantDefsScenes: 2,
			wantModified:   map[string]bool{"multi-scene": true},
		},
		{
			name:           "empty stories directory",
			setupFiles:     map[string]map[string]string{},
			wantDefsFiles:  false,
			wantModified:   map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storiesDir := t.TempDir()
			defsDir := t.TempDir()

			// Setup story files
			for storyID, files := range tt.setupFiles {
				storyPath := filepath.Join(storiesDir, storyID)
				require.NoError(t, os.MkdirAll(storyPath, 0755))
				for filename, content := range files {
					err := os.WriteFile(filepath.Join(storyPath, filename), []byte(content), 0644)
					require.NoError(t, err)
				}
			}

			err := ExtractDefinitions([]string{storiesDir}, defsDir)
			require.NoError(t, err)

			for storyID, wantModified := range tt.wantModified {
				if !wantModified {
					continue
				}

				// Verify markers were removed from the story file
				bookPath := filepath.Join(storiesDir, storyID, "book.yml")
				if _, ok := tt.setupFiles[storyID]["chapter1.yml"]; ok {
					bookPath = filepath.Join(storiesDir, storyID, "chapter1.yml")
				}

				data, err := os.ReadFile(bookPath)
				require.NoError(t, err)
				content := string(data)

				assert.NotContains(t, content, "{{")
				assert.NotContains(t, content, "}}")

				// Verify definitions section was removed
				var notebooks []notebook.StoryNotebook
				require.NoError(t, yaml.Unmarshal(data, &notebooks))
				for _, nb := range notebooks {
					for _, scene := range nb.Scenes {
						assert.Empty(t, scene.Definitions, "definitions should be removed from story file")
					}
				}
			}

			if tt.wantDefsFiles {
				// Find which story has definitions
				for storyID := range tt.wantModified {
					if !tt.wantModified[storyID] {
						continue
					}

					defsPath := filepath.Join(defsDir, "stories", storyID)

					// Verify index.yml was created
					indexPath := filepath.Join(defsPath, "index.yml")
					assert.FileExists(t, indexPath)
					indexData, err := os.ReadFile(indexPath)
					require.NoError(t, err)
					var idx definitionsIndexFile
					require.NoError(t, yaml.Unmarshal(indexData, &idx))
					assert.Equal(t, storyID, idx.ID)
					assert.Equal(t, []string{"./definitions.yml"}, idx.Notebooks)

					// Verify definitions.yml was created
					defsFilePath := filepath.Join(defsPath, "definitions.yml")
					assert.FileExists(t, defsFilePath)
					defsData, err := os.ReadFile(defsFilePath)
					require.NoError(t, err)
					var defs []notebook.Definitions
					require.NoError(t, yaml.Unmarshal(defsData, &defs))
					assert.NotEmpty(t, defs)

					totalScenes := 0
					for _, d := range defs {
						totalScenes += len(d.Scenes)
					}
					assert.Equal(t, tt.wantDefsScenes, totalScenes)
				}
			} else {
				// Verify no definitions directory was created for any story
				storiesDefsDir := filepath.Join(defsDir, "stories")
				_, err := os.Stat(storiesDefsDir)
				if err == nil {
					entries, err := os.ReadDir(storiesDefsDir)
					require.NoError(t, err)
					assert.Empty(t, entries, "no definitions directories should be created")
				}
			}
		})
	}
}

func TestExtractDefinitions_SkipExistingDefsDir(t *testing.T) {
	storiesDir := t.TempDir()
	defsDir := t.TempDir()

	storyID := "existing-story"
	storyPath := filepath.Join(storiesDir, storyID)
	require.NoError(t, os.MkdirAll(storyPath, 0755))

	storyContent := `- event: 'LESSON 1'
  date: 2026-01-19T00:00:00Z
  scenes:
    - scene: |
        A scene with an idiom.
      conversations:
        - speaker: Alice
          quote: That's the {{ last straw }}.
      definitions:
        - expression: last straw
          meaning: the final problem in a series`

	require.NoError(t, os.WriteFile(filepath.Join(storyPath, "index.yml"), []byte(`id: existing-story
notebooks:
  - ./book.yml`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyPath, "book.yml"), []byte(storyContent), 0644))

	// Pre-create the definitions directory
	existingDefsDir := filepath.Join(defsDir, "stories", storyID)
	require.NoError(t, os.MkdirAll(existingDefsDir, 0755))

	err := ExtractDefinitions([]string{storiesDir}, defsDir)
	require.NoError(t, err)

	// Verify index.yml was NOT created (directory already existed)
	indexPath := filepath.Join(existingDefsDir, "index.yml")
	_, err = os.Stat(indexPath)
	assert.True(t, os.IsNotExist(err), "index.yml should not be created when directory already exists")
}

func TestExtractDefinitions_NonexistentDirectory(t *testing.T) {
	defsDir := t.TempDir()
	err := ExtractDefinitions([]string{"/nonexistent/directory"}, defsDir)
	assert.NoError(t, err, "should skip nonexistent directories")
}

func TestProcessStoryFile(t *testing.T) {
	markerPattern := regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

	tests := []struct {
		name            string
		content         string
		wantScenes      int
		wantModified    bool
		wantExpressions []string
		wantErr         bool
	}{
		{
			name: "extracts definitions and removes markers",
			content: `- event: 'Test Event'
  date: 2026-01-19T00:00:00Z
  scenes:
    - scene: |
        A test scene.
      conversations:
        - speaker: Alice
          quote: I need to {{ get the ball rolling }}.
      definitions:
        - expression: get the ball rolling
          meaning: to start an activity or process`,
			wantScenes:      1,
			wantModified:    true,
			wantExpressions: []string{"get the ball rolling"},
		},
		{
			name: "no definitions - no modification",
			content: `- event: 'Test Event'
  date: 2026-01-19T00:00:00Z
  scenes:
    - scene: |
        A test scene.
      conversations:
        - speaker: Alice
          quote: Hello there.`,
			wantScenes:   0,
			wantModified: false,
		},
		{
			name:    "invalid yaml",
			content: `not: valid: yaml: [`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "test.yml")
			require.NoError(t, os.WriteFile(filePath, []byte(tt.content), 0644))

			defs, modified, err := processStoryFile(filePath, "test.yml", markerPattern)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantModified, modified)
			assert.Equal(t, tt.wantScenes, len(defs.Scenes))

			if tt.wantModified {
				assert.Equal(t, "test.yml", defs.Metadata.Notebook)

				// Verify markers were removed from the written file
				data, err := os.ReadFile(filePath)
				require.NoError(t, err)
				assert.NotContains(t, string(data), "{{")
				assert.NotContains(t, string(data), "}}")
			}

			// Verify expressions
			for i, scene := range defs.Scenes {
				if i < len(tt.wantExpressions) {
					found := false
					for _, expr := range scene.Expressions {
						if expr.Expression == tt.wantExpressions[i] {
							found = true
							break
						}
					}
					assert.True(t, found, "expected expression %q in scene %d", tt.wantExpressions[i], i)
				}
			}
		})
	}
}
