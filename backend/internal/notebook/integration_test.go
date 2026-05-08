package notebook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_NotebookPipeline exercises the full pipeline from YAML files
// on disk through the Reader, definitions merge, filter, template rendering,
// and markdown output for all notebook types. These tests catch issues that
// unit tests miss, such as scene-index mismatches in definitions merging.
func TestIntegration_NotebookPipeline(t *testing.T) {
	fixedDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ptr := func(i int) *int { return &i }

	type assertion struct {
		// contains lists strings that must appear in the markdown output.
		contains []string
		// notContains lists strings that must NOT appear.
		notContains []string
		// orderedPairs verifies that first appears before second in the output.
		orderedPairs [][2]string
		// sceneDefs verifies that after ReadStoryNotebooks, each scene index
		// has exactly the listed expression names. nil means skip this check.
		sceneDefs map[int][]string
	}

	tests := []struct {
		name string
		// setup creates temp directories and returns reader args + output dir
		setup func(t *testing.T) (storiesDirs, flashcardsDirs, booksDirs, definitionsDirs, etymologyDirs []string, learningHistories map[string][]LearningHistory, outputDir string)
		// notebookID is the ID passed to the writer
		notebookID string
		// notebookType selects which writer to use
		notebookType string // "story", "flashcard", "etymology"
		assert       assertion
	}{
		{
			name: "story with inline definitions",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				storiesDir := t.TempDir()
				outputDir := t.TempDir()
				storyDir := filepath.Join(storiesDir, "tv-show")
				require.NoError(t, os.MkdirAll(storyDir, 0755))
				require.NoError(t, WriteYamlFile(filepath.Join(storyDir, "index.yml"), Index{
					Kind: "TVShows", ID: "tv-show", Name: "TV Show",
					NotebookPaths: []string{"./episode1.yml"},
				}))
				require.NoError(t, WriteYamlFile(filepath.Join(storyDir, "episode1.yml"), []StoryNotebook{
					{
						Event: "Episode 1", Date: fixedDate,
						Scenes: []StoryScene{
							{
								Title:         "At the cafe",
								Conversations: []Conversation{{Speaker: "Alice", Quote: "I need to break the ice."}},
								Definitions:   []Note{{Expression: "break the ice", Meaning: "initiate conversation"}},
							},
							{
								Title:         "At the park",
								Conversations: []Conversation{{Speaker: "Bob", Quote: "Don't lose your temper."}},
								Definitions:   []Note{{Expression: "lose one's temper", Meaning: "become angry"}},
							},
						},
					},
				}))
				return []string{storiesDir}, nil, nil, nil, nil, map[string][]LearningHistory{}, outputDir
			},
			notebookID:   "tv-show",
			notebookType: "story",
			assert: assertion{
				contains: []string{"**break the ice**", "**lose one's temper**", "### At the cafe", "### At the park"},
				orderedPairs: [][2]string{
					{"### At the cafe", "**break the ice**"},
					{"**break the ice**", "### At the park"},
					{"### At the park", "**lose one's temper**"},
				},
			},
		},
		{
			name: "story with separate definitions matched by title",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				storiesDir, definitionsDir := setupNotebookWithDefinitions(t, "show",
					[]StoryNotebook{{
						Event: "Episode 1", Date: fixedDate,
						Scenes: []StoryScene{
							{Title: "Opening", Conversations: []Conversation{{Speaker: "Alice", Quote: "Good morning."}}},
							{Title: "At the office", Conversations: []Conversation{{Speaker: "Bob", Quote: "Let me break the ice."}}},
						},
					}},
					[]Definitions{{
						Metadata: DefinitionsMetadata{Title: "Episode 1"},
						Scenes: []DefinitionsScene{
							{Metadata: DefinitionsSceneMetadata{Index: 1, Title: "At the office"}, Expressions: []Note{{Expression: "break the ice", Meaning: "initiate conversation"}}},
						},
					}},
				)
				return []string{storiesDir}, nil, nil, []string{definitionsDir}, nil, map[string][]LearningHistory{}, t.TempDir()
			},
			notebookID:   "show",
			notebookType: "story",
			assert: assertion{
				contains: []string{"**break the ice**"},
				sceneDefs: map[int][]string{
					0: {},
					1: {"break the ice"},
				},
			},
		},
		{
			name: "book with separate definitions at matching indices",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				storiesDir, definitionsDir := setupNotebookWithDefinitions(t, "novel",
					[]StoryNotebook{{
						Event: "Chapter 1", Date: fixedDate,
						Scenes: []StoryScene{
							{Statements: []string{"The wind howled through the trees."}},
							{Statements: []string{"She braced herself for what came next."}},
						},
					}},
					[]Definitions{{
						Metadata: DefinitionsMetadata{Title: "Chapter 1"},
						Scenes: []DefinitionsScene{
							{Metadata: DefinitionsSceneMetadata{Index: 0}, Expressions: []Note{{Expression: "howled", Meaning: "made a loud cry"}}},
							{Metadata: DefinitionsSceneMetadata{Index: 1}, Expressions: []Note{{Expression: "braced", Definition: "brace", Meaning: "to prepare"}}},
						},
					}},
				)
				return nil, nil, []string{storiesDir}, []string{definitionsDir}, nil, map[string][]LearningHistory{}, t.TempDir()
			},
			notebookID:   "novel",
			notebookType: "story",
			assert: assertion{
				contains:     []string{"**howled**", "**brace**"},
				orderedPairs: [][2]string{{"**howled**", "**brace**"}},
				sceneDefs: map[int][]string{
					0: {"howled"},
					1: {"braced"},
				},
			},
		},
		{
			name: "book definitions at non-zero index (bug: array position vs declared index)",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				storiesDir, definitionsDir := setupNotebookWithDefinitions(t, "novel-idx",
					[]StoryNotebook{{
						Event: "Chapter 1", Date: fixedDate,
						Scenes: []StoryScene{
							{Statements: []string{"It was a quiet morning."}},
							{Statements: []string{"She lost her temper and stormed out."}},
						},
					}},
					[]Definitions{{
						Metadata: DefinitionsMetadata{Title: "Chapter 1"},
						Scenes: []DefinitionsScene{
							// Only one scene in the definitions file, declared at index 1.
							// Must merge into story scene 1, not scene 0.
							{Metadata: DefinitionsSceneMetadata{Index: 1}, Expressions: []Note{{Expression: "lose one's temper", Meaning: "become angry"}}},
						},
					}},
				)
				return nil, nil, []string{storiesDir}, []string{definitionsDir}, nil, map[string][]LearningHistory{}, t.TempDir()
			},
			notebookID:   "novel-idx",
			notebookType: "story",
			assert: assertion{
				sceneDefs: map[int][]string{
					0: {},
					1: {"lose one's temper"},
				},
				// Scene 0 has no definitions so it's filtered out of the
				// markdown. Only scene 1 (with definitions) appears.
				contains:    []string{"lost her temper", "lose one's temper"},
				notContains: []string{"quiet morning"},
			},
		},
		{
			name: "definitions matched by notebook filename",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				storiesDir, definitionsDir := setupNotebookWithDefinitions(t, "file-match",
					[]StoryNotebook{{
						Event: "Chapter 1", Date: fixedDate,
						Scenes: []StoryScene{
							{Statements: []string{"A simple opening line."}},
						},
					}},
					[]Definitions{{
						// Match by notebook filename instead of title
						Metadata: DefinitionsMetadata{Notebook: "./stories.yml"},
						Scenes: []DefinitionsScene{
							{Metadata: DefinitionsSceneMetadata{Index: 0}, Expressions: []Note{{Expression: "opening", Meaning: "the beginning"}}},
						},
					}},
				)
				return nil, nil, []string{storiesDir}, []string{definitionsDir}, nil, map[string][]LearningHistory{}, t.TempDir()
			},
			notebookID:   "file-match",
			notebookType: "story",
			assert: assertion{
				sceneDefs: map[int][]string{0: {"opening"}},
			},
		},
		{
			name: "definitions using scene field instead of index",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				storiesDir, definitionsDir := setupNotebookWithDefinitions(t, "scene-field",
					[]StoryNotebook{{
						Event: "Chapter 1", Date: fixedDate,
						Scenes: []StoryScene{
							{Statements: []string{"First paragraph."}},
							{Statements: []string{"Second paragraph."}},
							{Statements: []string{"Third paragraph with an idiom."}},
						},
					}},
					[]Definitions{{
						Metadata: DefinitionsMetadata{Title: "Chapter 1"},
						Scenes: []DefinitionsScene{
							// Uses the `scene` pointer field which takes precedence over `index`
							{Metadata: DefinitionsSceneMetadata{Scene: ptr(2), Index: 0, Title: "third"}, Expressions: []Note{{Expression: "idiom", Meaning: "a common expression"}}},
						},
					}},
				)
				return nil, nil, []string{storiesDir}, []string{definitionsDir}, nil, map[string][]LearningHistory{}, t.TempDir()
			},
			notebookID:   "scene-field",
			notebookType: "story",
			assert: assertion{
				sceneDefs: map[int][]string{
					0: {},
					1: {},
					2: {"idiom"},
				},
			},
		},
		{
			name: "duplicate scene titles resolved by index",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				storiesDir, definitionsDir := setupNotebookWithDefinitions(t, "dup-scenes",
					[]StoryNotebook{{
						Event: "Episode 1", Date: fixedDate,
						Scenes: []StoryScene{
							{Title: "At the office", Conversations: []Conversation{{Speaker: "Alice", Quote: "Let me break the ice."}}},
							{Title: "On the street", Conversations: []Conversation{{Speaker: "Bob", Quote: "Time to catch up."}}},
							{Title: "At the office", Conversations: []Conversation{{Speaker: "Carol", Quote: "Don't lose your temper."}}},
						},
					}},
					[]Definitions{{
						Metadata: DefinitionsMetadata{Title: "Episode 1"},
						Scenes: []DefinitionsScene{
							{Metadata: DefinitionsSceneMetadata{Index: 0}, Expressions: []Note{{Expression: "break the ice", Meaning: "initiate conversation"}}},
							{Metadata: DefinitionsSceneMetadata{Index: 1}, Expressions: []Note{{Expression: "catch up", Meaning: "get current"}}},
							{Metadata: DefinitionsSceneMetadata{Index: 2}, Expressions: []Note{{Expression: "lose one's temper", Meaning: "become angry"}}},
						},
					}},
				)
				return []string{storiesDir}, nil, nil, []string{definitionsDir}, nil, map[string][]LearningHistory{}, t.TempDir()
			},
			notebookID:   "dup-scenes",
			notebookType: "story",
			assert: assertion{
				contains: []string{"**break the ice**", "**catch up**", "**lose one's temper**"},
				sceneDefs: map[int][]string{
					0: {"break the ice"},
					1: {"catch up"},
					2: {"lose one's temper"},
				},
			},
		},
		{
			name: "sparse indices (gaps in scene numbers)",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				storiesDir, definitionsDir := setupNotebookWithDefinitions(t, "sparse",
					[]StoryNotebook{{
						Event: "Chapter 1", Date: fixedDate,
						Scenes: []StoryScene{
							{Statements: []string{"Scene zero."}},
							{Statements: []string{"Scene one."}},
							{Statements: []string{"Scene two."}},
							{Statements: []string{"Scene three with words."}},
						},
					}},
					[]Definitions{{
						Metadata: DefinitionsMetadata{Title: "Chapter 1"},
						Scenes: []DefinitionsScene{
							// Only scenes 0 and 3 have definitions — scenes 1 and 2 are skipped
							{Metadata: DefinitionsSceneMetadata{Index: 0}, Expressions: []Note{{Expression: "zero", Meaning: "nothing"}}},
							{Metadata: DefinitionsSceneMetadata{Index: 3}, Expressions: []Note{{Expression: "words", Meaning: "units of language"}}},
						},
					}},
				)
				return nil, nil, []string{storiesDir}, []string{definitionsDir}, nil, map[string][]LearningHistory{}, t.TempDir()
			},
			notebookID:   "sparse",
			notebookType: "story",
			assert: assertion{
				sceneDefs: map[int][]string{
					0: {"zero"},
					1: {},
					2: {},
					3: {"words"},
				},
			},
		},
		{
			name: "story with learning history filters correctly",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				storiesDir := t.TempDir()
				storyDir := filepath.Join(storiesDir, "quiz-test")
				require.NoError(t, os.MkdirAll(storyDir, 0755))
				require.NoError(t, WriteYamlFile(filepath.Join(storyDir, "index.yml"), Index{
					ID: "quiz-test", Name: "Quiz Test", NotebookPaths: []string{"./stories.yml"},
				}))
				require.NoError(t, WriteYamlFile(filepath.Join(storyDir, "stories.yml"), []StoryNotebook{{
					Event: "Episode 1", Date: fixedDate,
					Scenes: []StoryScene{{
						Title:         "Scene 1",
						Conversations: []Conversation{{Speaker: "Alice", Quote: "Let me break the ice and catch up."}},
						Definitions: []Note{
							{Expression: "break the ice", Meaning: "initiate conversation"},
							{Expression: "catch up", Meaning: "get current"},
						},
					}},
				}}))

				histories := map[string][]LearningHistory{
					"quiz-test": {{
						Metadata: LearningHistoryMetadata{NotebookID: "quiz-test", Title: "Episode 1"},
						Scenes: []LearningScene{{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								// misunderstood: should be included (needs review)
								{Expression: "break the ice", LearnedLogs: []LearningRecord{{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(time.Now())}}},
								// recently correct with long interval: should be excluded (not due)
								{Expression: "catch up", LearnedLogs: []LearningRecord{{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Now()), IntervalDays: 365}}},
							},
						}},
					}},
				}
				return []string{storiesDir}, nil, nil, nil, nil, histories, t.TempDir()
			},
			notebookID:   "quiz-test",
			notebookType: "story",
			assert: assertion{
				contains:    []string{"**break the ice**"},
				notContains: []string{"**catch up**"},
			},
		},
		{
			name: "flashcard notebook output",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				flashcardsDir := t.TempDir()
				dir := filepath.Join(flashcardsDir, "vocab")
				require.NoError(t, os.MkdirAll(dir, 0755))
				require.NoError(t, WriteYamlFile(filepath.Join(dir, "index.yml"), FlashcardIndex{
					ID: "vocab", Name: "Vocabulary", NotebookPaths: []string{"./cards.yml"},
				}))
				require.NoError(t, WriteYamlFile(filepath.Join(dir, "cards.yml"), []FlashcardNotebook{{
					Title: "Common Idioms", Date: fixedDate,
					Cards: []Note{
						{Expression: "break the ice", Meaning: "initiate conversation"},
						{Expression: "lose one's temper", Meaning: "become angry"},
					},
				}}))
				return nil, []string{flashcardsDir}, nil, nil, nil, map[string][]LearningHistory{}, t.TempDir()
			},
			notebookID:   "vocab",
			notebookType: "flashcard",
			assert: assertion{
				contains: []string{"break the ice", "lose one's temper", "Common Idioms"},
			},
		},
		{
			name: "etymology notebook output",
			setup: func(t *testing.T) ([]string, []string, []string, []string, []string, map[string][]LearningHistory, string) {
				etymDir := t.TempDir()
				dir := filepath.Join(etymDir, "roots")
				require.NoError(t, os.MkdirAll(dir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "index.yml"), []byte(`id: roots
kind: Etymology
name: Common Roots
notebooks:
  - ./session1.yml
`), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "session1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: spect
    type: root
    language: Latin
    meaning: to look or see
  - origin: graph
    type: root
    language: Greek
    meaning: to write
`), 0644))
				return nil, nil, nil, nil, []string{etymDir}, map[string][]LearningHistory{}, t.TempDir()
			},
			notebookID:   "roots",
			notebookType: "etymology",
			assert: assertion{
				contains: []string{"spect", "to look or see", "graph", "to write"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storiesDirs, flashcardsDirs, booksDirs, definitionsDirs, etymologyDirs, histories, outputDir := tt.setup(t)

			reader, err := NewReader(storiesDirs, flashcardsDirs, booksDirs, definitionsDirs, etymologyDirs, nil)
			require.NoError(t, err)

			// Verify scene-level definitions merge (when expected)
			if tt.assert.sceneDefs != nil {
				notebooks, err := reader.ReadStoryNotebooks(tt.notebookID)
				require.NoError(t, err)
				require.NotEmpty(t, notebooks)
				for sceneIdx, wantExprs := range tt.assert.sceneDefs {
					require.Greater(t, len(notebooks[0].Scenes), sceneIdx,
						"story should have scene at index %d", sceneIdx)
					var gotExprs []string
					for _, d := range notebooks[0].Scenes[sceneIdx].Definitions {
						gotExprs = append(gotExprs, d.Expression)
					}
					if len(wantExprs) == 0 {
						assert.Empty(t, gotExprs, "scene %d should have no definitions", sceneIdx)
					} else {
						assert.Equal(t, wantExprs, gotExprs, "scene %d definitions mismatch", sceneIdx)
					}
				}
			}

			// Run the writer pipeline and read the markdown output
			switch tt.notebookType {
			case "story":
				writer := NewStoryNotebookWriter(reader, "")
				err = writer.OutputStoryNotebooks(tt.notebookID, nil, histories, false, outputDir, false)
			case "flashcard":
				writer := NewFlashcardNotebookWriter(reader, "")
				err = writer.OutputFlashcardNotebooks(tt.notebookID, nil, histories, false, outputDir, false)
			case "etymology":
				writer := NewEtymologyNotebookWriter(reader, "", definitionsDirs, histories)
				err = writer.OutputEtymologyNotebook(tt.notebookID, outputDir, false)
			default:
				t.Fatalf("unknown notebook type: %s", tt.notebookType)
			}
			require.NoError(t, err)

			mdPath := filepath.Join(outputDir, tt.notebookID+".md")
			md, err := os.ReadFile(mdPath)
			require.NoError(t, err)
			content := string(md)
			require.NotEmpty(t, content)

			for _, want := range tt.assert.contains {
				assert.Contains(t, content, want)
			}
			for _, notWant := range tt.assert.notContains {
				assert.NotContains(t, content, notWant)
			}
			for _, pair := range tt.assert.orderedPairs {
				firstIdx := strings.Index(content, pair[0])
				secondIdx := strings.Index(content, pair[1])
				require.NotEqual(t, -1, firstIdx, "%q should appear in output", pair[0])
				require.NotEqual(t, -1, secondIdx, "%q should appear in output", pair[1])
				assert.Less(t, firstIdx, secondIdx,
					"%q should appear before %q", pair[0], pair[1])
			}
		})
	}
}

// setupNotebookWithDefinitions creates a story/book notebook and a separate
// definitions file in temp directories and returns (storiesDir, definitionsDir).
func setupNotebookWithDefinitions(t *testing.T, id string, stories []StoryNotebook, defs []Definitions) (string, string) {
	t.Helper()

	storiesDir := t.TempDir()
	storyDir := filepath.Join(storiesDir, id)
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, WriteYamlFile(filepath.Join(storyDir, "index.yml"), Index{
		ID: id, Name: id, NotebookPaths: []string{"./stories.yml"},
	}))
	require.NoError(t, WriteYamlFile(filepath.Join(storyDir, "stories.yml"), stories))

	definitionsDir := t.TempDir()
	defsDir := filepath.Join(definitionsDir, id)
	require.NoError(t, os.MkdirAll(defsDir, 0755))
	require.NoError(t, WriteYamlFile(filepath.Join(defsDir, "index.yml"), definitionsIndex{
		ID: id, Notebooks: []string{"./definitions.yml"},
	}))
	require.NoError(t, WriteYamlFile(filepath.Join(defsDir, "definitions.yml"), defs))

	return storiesDir, definitionsDir
}
