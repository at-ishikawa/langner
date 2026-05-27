package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ValidationError represents a single validation error
type ValidationError struct {
	File        string
	Location    string
	Message     string
	Severity    string // "error" or "warning"
	Suggestions []string
}

func (e ValidationError) Error() string {
	location := ""
	if e.Location != "" {
		location = fmt.Sprintf(" (%s)", e.Location)
	}
	msg := fmt.Sprintf("%s%s: %s", e.File, location, e.Message)
	if len(e.Suggestions) > 0 {
		msg += fmt.Sprintf(" [Suggestion: %s]", strings.Join(e.Suggestions, "; "))
	}
	return msg
}

// ValidationResult contains all validation errors grouped by type
type ValidationResult struct {
	LearningNotesErrors []ValidationError
	ConsistencyErrors   []ValidationError
	Warnings            []ValidationError
}

func (r *ValidationResult) HasErrors() bool {
	return len(r.LearningNotesErrors) > 0 || len(r.ConsistencyErrors) > 0
}

func (r *ValidationResult) AddError(category string, err ValidationError) {
	err.Severity = "error"
	switch category {
	case "learning_notes":
		r.LearningNotesErrors = append(r.LearningNotesErrors, err)
	case "consistency":
		r.ConsistencyErrors = append(r.ConsistencyErrors, err)
	}
}

func (r *ValidationResult) AddWarning(err ValidationError) {
	err.Severity = "warning"
	r.Warnings = append(r.Warnings, err)
}

// Validator performs validation of learning notes and story notebooks
type Validator struct {
	learningNotesDir   string
	storyNotebooksDirs []string
	flashcardsDirs     []string
	definitionsDirs    []string
	etymologyDirs      []string
	dictionaryDir      string
	calculator         IntervalCalculator
}

// NewValidator creates a new validator
func NewValidator(learningNotesDir string, storyNotebooksDirs []string, flashcardsDirs []string, definitionsDirs []string, etymologyDirs []string, dictionaryDir string, calculator IntervalCalculator) *Validator {
	if calculator == nil {
		calculator = &SM2Calculator{}
	}
	return &Validator{
		learningNotesDir:   learningNotesDir,
		storyNotebooksDirs: storyNotebooksDirs,
		flashcardsDirs:     flashcardsDirs,
		definitionsDirs:    definitionsDirs,
		etymologyDirs:      etymologyDirs,
		dictionaryDir:      dictionaryDir,
		calculator:         calculator,
	}
}

// Validate performs all validations
func (v *Validator) Validate() (*ValidationResult, error) {
	result := &ValidationResult{}

	// Load all learning notes
	learningHistories, err := v.loadLearningHistories()
	if err != nil {
		return nil, fmt.Errorf("loadLearningHistories() > %w", err)
	}

	// Load all story notebooks
	storyNotebooks, err := v.loadStoryNotebooks()
	if err != nil {
		return nil, fmt.Errorf("loadStoryNotebooks() > %w", err)
	}

	// Load all flashcard notebooks if flashcardsDirs is configured
	if len(v.flashcardsDirs) > 0 {
		flashcardNotebooks, err := v.loadFlashcardNotebooks()
		if err != nil {
			return nil, fmt.Errorf("loadFlashcardNotebooks() > %w", err)
		}

		// Validate flashcard notebooks
		v.validateFlashcardNotebooks(flashcardNotebooks, result)
	}

	// Load definitions
	definitionsExpressions := v.loadDefinitionsExpressions()

	// Validate learning notes structure
	v.validateLearningNotesStructure(learningHistories, result)

	// Validate cross-notebook consistency
	v.validateConsistency(learningHistories, storyNotebooks, definitionsExpressions, result)

	// Validate dictionary references
	v.validateDictionaryReferences(storyNotebooks, result)

	// Validate definitions appear in conversations (inline definitions)
	v.validateDefinitionsInConversations(storyNotebooks, result)

	// Validate definitions from separate definitions files appear in conversations
	v.validateSeparateDefinitionsInConversations(storyNotebooks, result)

	// Validate the new etymology extensions (forms, concepts, relations).
	// Failures are warn-only while the schema matures so existing notebooks
	// (which don't carry these fields) keep validating cleanly.
	v.validateEtymologyExtensions(result)
	v.validateFromForm(result)

	// Validate the new definitions-side concepts: block. Warn-only for the
	// same reason as the etymology extensions — existing books without
	// concepts must keep validating cleanly while the feature lands.
	v.validateDefinitionConcepts(result)

	return result, nil
}

// Fix automatically fixes validation errors
func (v *Validator) Fix() (*ValidationResult, error) {
	result := &ValidationResult{}

	// Load all learning notes
	learningHistories, err := v.loadLearningHistories()
	if err != nil {
		return nil, fmt.Errorf("loadLearningHistories() > %w", err)
	}

	// Load all story notebooks
	storyNotebooks, err := v.loadStoryNotebooks()
	if err != nil {
		return nil, fmt.Errorf("loadStoryNotebooks() > %w", err)
	}

	// Promote legacy interval_days: 3650 skips into the per-type
	// SkippedAt map. Must run before fixConsistency, which would otherwise
	// recompute interval_days and silently overwrite the skip.
	fixedLearning := v.migrateLegacySkipIntervals(learningHistories, result)

	// Migrate legacy etymology shape (notebook-name top-level + sessions
	// as scenes + type=etymology) to the canonical per-session shape.
	// Must run before fixMismatchedScenes/fixConsistency: those passes
	// treat Shape A scenes as orphaned because the source story
	// notebooks don't contain those scene titles.
	fixedLearning = v.migrateEtymologyShape(fixedLearning, result)

	// Fix mismatched scenes by moving expressions to correct scenes (do this first)
	fixedLearning = v.fixMismatchedScenes(fixedLearning, storyNotebooks, result)

	// Fix expression names to match story definitions (do this before merging duplicates)
	fixedLearning = v.fixExpressionNames(fixedLearning, storyNotebooks, result)

	// Rename legacy "__index_N" definitions-book scene keys to their human
	// scene title so the next pass (duplicate-scene merge) collapses the
	// split scenes a word's logs and skip were spread across. Must run
	// before fixLearningNotesStructure, which does the merge.
	fixedLearning = v.renameDefinitionsIndexScenes(fixedLearning, result)

	// Consolidate an etymology origin whose logs got split across two
	// scene titles in one session (derived-scene-title drift). Must run
	// before fixLearningNotesStructure so any resulting same-title
	// duplicate scenes are collapsed by the merge pass.
	fixedLearning = v.consolidateEtymologyOriginScenes(fixedLearning, result)

	// Move definitions-book vocabulary words mis-filed under a synthetic
	// session-named scene (or any non-home scene) to the scene where the
	// word is actually defined — merging logs when it's already there.
	fixedLearning = v.consolidateDefinitionsScenes(fixedLearning, result)

	// Fix learning notes structure issues (including duplicate merging - do this after renaming)
	fixedLearning = v.fixLearningNotesStructure(fixedLearning, result)

	// Fix cross-notebook consistency issues
	fixedLearning = v.fixConsistency(fixedLearning, storyNotebooks, result)

	// Backfill quiz_type for learning logs that predate the field
	fixedLearning = v.backfillQuizType(fixedLearning, result)

	// Create missing learning note entries
	fixedLearning = v.createMissingLearningNotes(fixedLearning, storyNotebooks, result)

	// Replay every log series through the SR calculator so interval_days
	// reflects the actual chain of answers (each log's interval threaded
	// into the next via RecalculateAll, with the early-review guard
	// preventing growth on too-soon correct answers). Touches all four
	// slots: LearnedLogs / ReverseLogs / EtymologyBreakdownLogs /
	// EtymologyAssemblyLogs. Reports each interval drift as a warning so
	// the run shows which logs got corrected; logs whose recalculated
	// value matches stored stay untouched in the output.
	fixedLearning = v.recalculateAllIntervals(fixedLearning, result)

	// Fix dictionary reference issues
	fixedStory := v.fixDictionaryReferences(storyNotebooks, result)

	// Save fixed learning histories
	for _, file := range fixedLearning {
		if err := WriteYamlFile(file.path, file.contents); err != nil {
			return nil, fmt.Errorf("WriteYamlFile(%s) > %w", file.path, err)
		}
	}

	// Save fixed story notebooks
	for _, file := range fixedStory {
		if err := WriteYamlFile(file.path, file.contents); err != nil {
			return nil, fmt.Errorf("WriteYamlFile(%s) > %w", file.path, err)
		}
	}

	return result, nil
}

// learningHistoryFile is a type alias for learning history files
type learningHistoryFile = yamlFile[[]LearningHistory]

// storyNotebookFile is a type alias for story notebook files
type storyNotebookFile = yamlFile[[]StoryNotebook]

// flashcardNotebookFile is a type alias for flashcard notebook files
type flashcardNotebookFile = yamlFile[[]FlashcardNotebook]

func (v *Validator) loadLearningHistories() ([]learningHistoryFile, error) {
	return loadYamlFiles[[]LearningHistory](v.learningNotesDir, func(path string, info os.FileInfo) bool {
		return !info.IsDir() && filepath.Ext(path) == ".yml"
	})
}

func (v *Validator) loadStoryNotebooks() ([]storyNotebookFile, error) {
	var allFiles []storyNotebookFile
	for _, dir := range v.storyNotebooksDirs {
		files, err := loadYamlFilesSkipErrors[[]StoryNotebook](dir, func(path string, info os.FileInfo) bool {
			return !info.IsDir() && filepath.Ext(path) == ".yml" && filepath.Base(path) != "index.yml"
		})
		if err != nil {
			return nil, fmt.Errorf("loadYamlFiles(%s) > %w", dir, err)
		}
		allFiles = append(allFiles, files...)
	}
	return allFiles, nil
}

// loadDefinitionsExpressions returns a set of all expressions found in definitions directories.
func (v *Validator) loadDefinitionsExpressions() map[string]bool {
	result := make(map[string]bool)
	defsMap, _, _, err := NewDefinitionsMap(v.definitionsDirs)
	if err != nil {
		return result
	}
	for _, bookDefs := range defsMap {
		for _, notebookDefs := range bookDefs {
			for _, sceneDefs := range notebookDefs {
				for _, note := range sceneDefs {
					expr := strings.TrimSpace(note.Expression)
					if expr != "" {
						result[expr] = true
					}
					if note.Definition != "" {
						result[strings.TrimSpace(note.Definition)] = true
					}
				}
			}
		}
	}
	// Etymology origins are valid expression sources for learning history
	// (the etymology learning history tracks origins, not vocabulary words).
	for k := range v.loadEtymologyOriginExpressions() {
		result[k] = true
	}
	return result
}

// loadEtymologyOriginExpressions returns a set of all etymology origins
// across configured etymology directories. Used by the consistency check so
// origins recorded in learning history (under per-session scenes) aren't
// flagged as orphaned.
func (v *Validator) loadEtymologyOriginExpressions() map[string]bool {
	result := make(map[string]bool)
	indexMap := make(map[string]EtymologyIndex)
	for _, dir := range v.etymologyDirs {
		if dir == "" {
			continue
		}
		_ = walkEtymologyIndexFiles(dir, indexMap)
	}
	for _, idx := range indexMap {
		for _, nbPath := range idx.NotebookPaths {
			wrapped, err := readYamlFile[etymologySessionFile](filepath.Join(idx.Path, nbPath))
			if err != nil {
				continue
			}
			for _, o := range wrapped.Origins {
				origin := strings.TrimSpace(o.Origin)
				if origin != "" {
					result[origin] = true
				}
			}
		}
	}
	return result
}

func (v *Validator) loadFlashcardNotebooks() ([]flashcardNotebookFile, error) {
	var allFiles []flashcardNotebookFile
	for _, dir := range v.flashcardsDirs {
		files, err := loadYamlFiles[[]FlashcardNotebook](dir, func(path string, info os.FileInfo) bool {
			return !info.IsDir() && filepath.Ext(path) == ".yml" && filepath.Base(path) != "index.yml"
		})
		if err != nil {
			return nil, fmt.Errorf("loadYamlFiles(%s) > %w", dir, err)
		}
		allFiles = append(allFiles, files...)
	}
	return allFiles, nil
}

func (v *Validator) validateLearningNotesStructure(files []learningHistoryFile, result *ValidationResult) {
	for _, file := range files {
		for histIdx, history := range file.contents {
			historyLocation := fmt.Sprintf("history[%d]: %s", histIdx, history.Metadata.Title)

			// Call the Validate method on the LearningHistory object
			errors := history.Validate(historyLocation)
			for _, err := range errors {
				err.File = file.path
				result.AddError("learning_notes", err)
			}
		}
	}
}

func (v *Validator) validateConsistency(
	learningFiles []learningHistoryFile,
	storyFiles []storyNotebookFile,
	definitionsExpressions map[string]bool,
	result *ValidationResult,
) {
	// Build index of all expressions in story notebooks
	// Map: expression -> []storyLocation
	storyExpressions := make(map[string][]string)
	// Map: series title + scene title -> set of expressions
	storySceneExpressions := make(map[string]map[string]bool)

	for _, file := range storyFiles {
		for _, notebook := range file.contents {
			for _, scene := range notebook.Scenes {
				sceneKey := fmt.Sprintf("%s::%s", notebook.Event, scene.Title)
				if storySceneExpressions[sceneKey] == nil {
					storySceneExpressions[sceneKey] = make(map[string]bool)
				}

				for _, def := range scene.Definitions {
					expr := strings.TrimSpace(def.Expression)
					if expr == "" {
						continue
					}

					location := fmt.Sprintf("%s (%s - %s)", filepath.Base(file.path), notebook.Event, scene.Title)
					storyExpressions[expr] = append(storyExpressions[expr], location)
					storySceneExpressions[sceneKey][expr] = true

					// Also index by definition if it exists
					if def.Definition != "" {
						definition := strings.TrimSpace(def.Definition)
						storyExpressions[definition] = append(storyExpressions[definition], location)
						storySceneExpressions[sceneKey][definition] = true
					}
				}
			}
		}
	}

	// Check learning notes expressions exist in story notebooks
	for _, file := range learningFiles {
		for histIdx, history := range file.contents {
			for sceneIdx, scene := range history.Scenes {
				sceneKey := fmt.Sprintf("%s::%s", history.Metadata.Title, scene.Metadata.Title)
				sceneLocation := fmt.Sprintf("history[%d]: %s -> scene[%d]: %s", histIdx, history.Metadata.Title, sceneIdx, scene.Metadata.Title)

				// Track expressions in this scene for duplicate detection
				seenExpressions := make(map[string]int)

				for exprIdx, expr := range scene.Expressions {
					expression := strings.TrimSpace(expr.Expression)
					if expression == "" {
						continue
					}

					exprLocation := fmt.Sprintf("%s -> expression[%d]: %s", sceneLocation, exprIdx, expression)

					// Check for duplicates in the same scene
					if prevIdx, seen := seenExpressions[expression]; seen {
						result.AddError("consistency", ValidationError{
							File:     file.path,
							Location: sceneLocation,
							Message:  fmt.Sprintf("duplicate expression %q found at indices %d and %d", expression, prevIdx, exprIdx),
						})
					}
					seenExpressions[expression] = exprIdx

					// Check if expression exists in story notebooks or definitions
					locations, found := storyExpressions[expression]
					if !found && !definitionsExpressions[expression] {
						result.AddError("consistency", ValidationError{
							File:     file.path,
							Location: exprLocation,
							Message:  fmt.Sprintf("orphaned learning note: expression %q not found in any notebook or definition", expression),
							Suggestions: []string{
								"remove this learning note or add the expression to a notebook or definition",
							},
						})
						continue
					}
					if !found {
						// Found in definitions but not stories — skip scene checks
						continue
					}

					// Check if the expression is in the correct scene
					sceneExprs, sceneExists := storySceneExpressions[sceneKey]
					if !sceneExists {
						result.AddError("consistency", ValidationError{
							File:     file.path,
							Location: exprLocation,
							Message:  fmt.Sprintf("scene %q not found in story notebooks", sceneKey),
							Suggestions: []string{
								fmt.Sprintf("expression %q exists in: %s", expression, strings.Join(locations, ", ")),
							},
						})
					} else if !sceneExprs[expression] {
						result.AddError("consistency", ValidationError{
							File:     file.path,
							Location: exprLocation,
							Message:  fmt.Sprintf("expression %q not found in expected scene %q", expression, sceneKey),
							Suggestions: []string{
								fmt.Sprintf("expression exists in: %s", strings.Join(locations, ", ")),
							},
						})
					}
				}
			}
		}
	}

	// Check for missing learning notes
	for _, file := range storyFiles {
		for _, notebook := range file.contents {
			for _, scene := range notebook.Scenes {
				for _, def := range scene.Definitions {
					expr := strings.TrimSpace(def.Expression)
					if expr == "" {
						continue
					}

					// Check if this expression has learning notes
					hasLearningNote := false
					for _, learningFile := range learningFiles {
						for _, history := range learningFile.contents {
							if history.Metadata.Title != notebook.Event {
								continue
							}
							for _, learningScene := range history.Scenes {
								if learningScene.Metadata.Title != scene.Title {
									continue
								}
								for _, learningExpr := range learningScene.Expressions {
									if learningExpr.Expression == expr || learningExpr.Expression == def.Definition {
										hasLearningNote = true
										break
									}
								}
								if hasLearningNote {
									break
								}
							}
							if hasLearningNote {
								break
							}
						}
						if hasLearningNote {
							break
						}
					}

					if !hasLearningNote {
						location := fmt.Sprintf("%s (%s - %s)", filepath.Base(file.path), notebook.Event, scene.Title)
						result.AddWarning(ValidationError{
							File:     location,
							Location: fmt.Sprintf("expression: %s", expr),
							Message:  fmt.Sprintf("missing learning note for expression %q", expr),
							Suggestions: []string{
								"consider adding a learning note for this expression",
							},
						})
					}
				}
			}
		}
	}
}

func (v *Validator) validateDictionaryReferences(files []storyNotebookFile, result *ValidationResult) {
	for _, file := range files {
		for notebookIdx, notebook := range file.contents {
			for sceneIdx, scene := range notebook.Scenes {
				for defIdx, def := range scene.Definitions {
					if def.DictionaryNumber <= 0 {
						continue
					}

					// Determine the word to look up
					word := def.Definition
					if word == "" {
						word = def.Expression
					}
					word = strings.ToLower(strings.TrimSpace(word))

					// Check if dictionary file exists
					dictPath := filepath.Join(v.dictionaryDir, word+".json")
					if _, err := os.Stat(dictPath); os.IsNotExist(err) {
						location := fmt.Sprintf("notebook[%d]: %s -> scene[%d]: %s -> definition[%d]: %s",
							notebookIdx, notebook.Event, sceneIdx, scene.Title, defIdx, def.Expression)
						result.AddError("consistency", ValidationError{
							File:     file.path,
							Location: location,
							Message:  fmt.Sprintf("dictionary file not found for word %q (expected: %s)", word, dictPath),
							Suggestions: []string{
								"run dictionary command to fetch the definition",
								"or remove dictionary_number field",
							},
						})
					}
				}
			}
		}
	}
}

func (v *Validator) fixLearningNotesStructure(files []learningHistoryFile, result *ValidationResult) []learningHistoryFile {
	for fileIdx := range files {
		// First, merge duplicate top-level histories whose Metadata.Title
		// matches after quote normalization. Without this, two episode entries
		// for the same lesson — one written with a smart apostrophe, one with
		// an ASCII apostrophe — survive as separate histories. The same
		// expression then appears in both, the older entry's stale shorter
		// interval keeps the word due, and the user is asked it every day.
		files[fileIdx].contents = v.mergeDuplicateHistories(
			files[fileIdx].contents, files[fileIdx].path, result,
		)
	}
	for _, file := range files {
		for histIdx := range file.contents {
			// Merge duplicate scenes whose titles differ only by quote style
			// (e.g., smart quotes from book imports vs ASCII apostrophes).
			file.contents[histIdx].Scenes = v.mergeDuplicateScenes(
				file.contents[histIdx].Scenes, file.path, file.contents[histIdx].Metadata.Title, result,
			)

			for sceneIdx := range file.contents[histIdx].Scenes {
				scene := &file.contents[histIdx].Scenes[sceneIdx]

				// First, merge duplicate expressions in the same scene.
				// Dedup key is (name, type) so a vocab "ego" and an
				// origin "ego" can coexist without merging.
				type exprKey struct{ name, typ string }
				normaliseType := func(t string) string {
					if t == LearningExpressionTypeVocabulary {
						return ""
					}
					return t
				}
				expressionMap := make(map[exprKey]int)
				var mergedExpressions []LearningHistoryExpression

				for _, expr := range scene.Expressions {
					name := strings.TrimSpace(expr.Expression)
					if name == "" {
						continue
					}
					key := exprKey{name: name, typ: normaliseType(expr.Type)}

					if existingIdx, found := expressionMap[key]; found {
						// Merge learning logs into the existing expression
						if len(expr.LearnedLogs) > 0 {
							mergedExpressions[existingIdx].LearnedLogs = append(mergedExpressions[existingIdx].LearnedLogs, expr.LearnedLogs...)
							_, mergedExpressions[existingIdx].LearnedLogs, _ = recalculateLearningLogs(mergedExpressions[existingIdx].LearnedLogs, v.calculator)
						}
						if len(expr.ReverseLogs) > 0 {
							mergedExpressions[existingIdx].ReverseLogs = append(mergedExpressions[existingIdx].ReverseLogs, expr.ReverseLogs...)
							_, mergedExpressions[existingIdx].ReverseLogs, _ = recalculateLearningLogs(mergedExpressions[existingIdx].ReverseLogs, v.calculator)
						}
						result.AddWarning(ValidationError{
							File:    file.path,
							Message: fmt.Sprintf("Merged duplicate expression %q (type=%q) in scene %s::%s", name, key.typ, file.contents[histIdx].Metadata.Title, scene.Metadata.Title),
						})
					} else {
						expressionMap[key] = len(mergedExpressions)
						mergedExpressions = append(mergedExpressions, expr)
					}
				}

				scene.Expressions = mergedExpressions

				// Sort learned logs chronologically (newest first)
				for exprIdx := range scene.Expressions {
					expr := &scene.Expressions[exprIdx]

					if len(expr.LearnedLogs) == 0 {
						continue
					}

					// Sort by date descending (newest first)
					sort.Slice(expr.LearnedLogs, func(i, j int) bool {
						return expr.LearnedLogs[i].LearnedAt.After(expr.LearnedLogs[j].LearnedAt.Time)
					})
				}
			}

			// Etymology histories use scenes to disambiguate multi-sense
			// origins (e.g. "ana" = "up" in Session 13, "negative" in
			// Session 16). The same expression in two scenes is intentional,
			// so the cross-scene merge below would corrupt the data — skip
			// it entirely for etymology.
			if file.contents[histIdx].Metadata.Type == "etymology" {
				continue
			}

			// Then, merge duplicate expressions across different scenes in
			// the same episode. Key by (name, type) so a vocab "ego" in
			// one scene and an origin "ego" in another stay separate.
			type crossKey struct{ name, typ string }
			normaliseType := func(t string) string {
				if t == LearningExpressionTypeVocabulary {
					return ""
				}
				return t
			}
			episodeExpressions := make(map[crossKey]int) // -> scene index
			for sceneIdx := range file.contents[histIdx].Scenes {
				scene := &file.contents[histIdx].Scenes[sceneIdx]

				expressionsToRemove := make(map[int]bool)
				for exprIdx := range scene.Expressions {
					expr := &scene.Expressions[exprIdx]
					name := strings.TrimSpace(expr.Expression)
					if name == "" {
						continue
					}
					ck := crossKey{name: name, typ: normaliseType(expr.Type)}

					if firstSceneIdx, found := episodeExpressions[ck]; found {
						firstScene := &file.contents[histIdx].Scenes[firstSceneIdx]
						for firstExprIdx := range firstScene.Expressions {
							firstExpr := &firstScene.Expressions[firstExprIdx]
							if strings.TrimSpace(firstExpr.Expression) != name {
								continue
							}
							if normaliseType(firstExpr.Type) != ck.typ {
								continue
							}
							if len(expr.LearnedLogs) > 0 {
								firstExpr.LearnedLogs = append(firstExpr.LearnedLogs, expr.LearnedLogs...)
								_, firstExpr.LearnedLogs, _ = recalculateLearningLogs(firstExpr.LearnedLogs, v.calculator)
							}
							if len(expr.ReverseLogs) > 0 {
								firstExpr.ReverseLogs = append(firstExpr.ReverseLogs, expr.ReverseLogs...)
								_, firstExpr.ReverseLogs, _ = recalculateLearningLogs(firstExpr.ReverseLogs, v.calculator)
							}
							expressionsToRemove[exprIdx] = true
							result.AddWarning(ValidationError{
								File:    file.path,
								Message: fmt.Sprintf("Merged duplicate expression %q (type=%q) from scene %s into scene %s in episode %s",
									name, ck.typ,
									scene.Metadata.Title,
									firstScene.Metadata.Title,
									file.contents[histIdx].Metadata.Title),
							})
							break
						}
					} else {
						episodeExpressions[ck] = sceneIdx
					}
				}

				if len(expressionsToRemove) > 0 {
					filteredExpressions := make([]LearningHistoryExpression, 0)
					for exprIdx, expr := range scene.Expressions {
						if !expressionsToRemove[exprIdx] {
							filteredExpressions = append(filteredExpressions, expr)
						}
					}
					scene.Expressions = filteredExpressions
				}
			}

			// After merging across scenes, need to sort within each scene
			for sceneIdx := range file.contents[histIdx].Scenes {
				scene := &file.contents[histIdx].Scenes[sceneIdx]
				for exprIdx := range scene.Expressions {
					expr := &scene.Expressions[exprIdx]

					if len(expr.LearnedLogs) == 0 {
						continue
					}

					// Sort by date descending (newest first)
					sort.Slice(expr.LearnedLogs, func(i, j int) bool {
						return expr.LearnedLogs[i].LearnedAt.After(expr.LearnedLogs[j].LearnedAt.Time)
					})
				}
			}
		}
	}

	return files
}

// mergeDuplicateHistories merges top-level LearningHistory entries whose
// Metadata.Title matches after normalizing quote characters and trimming
// whitespace. Scenes from later histories are appended to the first one;
// the per-history scene/expression dedup that runs after this call collapses
// the resulting overlap.
//
// The encounter-order of titles is preserved so existing files don't see a
// gratuitous reordering on --fix.
func (v *Validator) mergeDuplicateHistories(
	histories []LearningHistory, filePath string, result *ValidationResult,
) []LearningHistory {
	groups := make(map[string][]int)
	var order []string
	for i, h := range histories {
		key := normalizeQuotes(strings.TrimSpace(h.Metadata.Title))
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}

	needsMerge := false
	for _, indices := range groups {
		if len(indices) > 1 {
			needsMerge = true
			break
		}
	}
	if !needsMerge {
		return histories
	}

	var merged []LearningHistory
	for _, key := range order {
		indices := groups[key]
		base := histories[indices[0]]
		if len(indices) == 1 {
			merged = append(merged, base)
			continue
		}
		for _, hi := range indices[1:] {
			base.Scenes = append(base.Scenes, histories[hi].Scenes...)
		}
		result.AddWarning(ValidationError{
			File:    filePath,
			Message: fmt.Sprintf("Merged %d duplicate history entry(ies) with title %q", len(indices)-1, base.Metadata.Title),
		})
		merged = append(merged, base)
	}
	return merged
}

// mergeDuplicateScenes merges scenes whose titles match after normalizing
// quote characters (e.g. smart quotes vs ASCII apostrophes). Expressions in
// duplicate scenes are merged; logs are combined and recalculated.
func (v *Validator) mergeDuplicateScenes(
	scenes []LearningScene, filePath, storyTitle string, result *ValidationResult,
) []LearningScene {
	groups := make(map[string][]int) // normalized title -> scene indices
	var order []string               // preserve encounter order
	for i, s := range scenes {
		key := normalizeQuotes(strings.TrimSpace(s.Metadata.Title))
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}

	needsMerge := false
	for _, indices := range groups {
		if len(indices) > 1 {
			needsMerge = true
			break
		}
	}
	if !needsMerge {
		return scenes
	}

	var merged []LearningScene
	for _, key := range order {
		indices := groups[key]
		base := scenes[indices[0]]
		if len(indices) == 1 {
			merged = append(merged, base)
			continue
		}

		// Merge expressions from later scenes into base
		exprMap := make(map[string]int) // expression -> index in base.Expressions
		for i, e := range base.Expressions {
			exprMap[strings.TrimSpace(e.Expression)] = i
		}
		for _, si := range indices[1:] {
			for _, e := range scenes[si].Expressions {
				ek := strings.TrimSpace(e.Expression)
				if idx, ok := exprMap[ek]; ok {
					// Merge logs
					if len(e.LearnedLogs) > 0 {
						base.Expressions[idx].LearnedLogs = append(base.Expressions[idx].LearnedLogs, e.LearnedLogs...)
						_, base.Expressions[idx].LearnedLogs, _ = recalculateLearningLogs(base.Expressions[idx].LearnedLogs, v.calculator)
					}
					if len(e.ReverseLogs) > 0 {
						base.Expressions[idx].ReverseLogs = append(base.Expressions[idx].ReverseLogs, e.ReverseLogs...)
						_, base.Expressions[idx].ReverseLogs, _ = recalculateLearningLogs(base.Expressions[idx].ReverseLogs, v.calculator)
					}
					if len(e.EtymologyBreakdownLogs) > 0 {
						base.Expressions[idx].EtymologyBreakdownLogs = append(base.Expressions[idx].EtymologyBreakdownLogs, e.EtymologyBreakdownLogs...)
						_, base.Expressions[idx].EtymologyBreakdownLogs, _ = recalculateLearningLogs(base.Expressions[idx].EtymologyBreakdownLogs, v.calculator)
					}
					if len(e.EtymologyAssemblyLogs) > 0 {
						base.Expressions[idx].EtymologyAssemblyLogs = append(base.Expressions[idx].EtymologyAssemblyLogs, e.EtymologyAssemblyLogs...)
						_, base.Expressions[idx].EtymologyAssemblyLogs, _ = recalculateLearningLogs(base.Expressions[idx].EtymologyAssemblyLogs, v.calculator)
					}
					// Merge skip state: a skip recorded on either copy must
					// survive the merge. Without this, a word skipped in the
					// "__index_N" copy but logged in the human-title copy (or
					// vice-versa) would silently lose its skip.
					base.Expressions[idx].SkippedAt = mergeSkippedAt(base.Expressions[idx].SkippedAt, e.SkippedAt)
					result.AddWarning(ValidationError{
						File:    filePath,
						Message: fmt.Sprintf("Merged duplicate expression %q from duplicate scene in %s", ek, storyTitle),
					})
				} else {
					exprMap[ek] = len(base.Expressions)
					base.Expressions = append(base.Expressions, e)
				}
			}
		}

		result.AddWarning(ValidationError{
			File:    filePath,
			Message: fmt.Sprintf("Merged %d duplicate scene(s) with title %q in %s", len(indices)-1, key[:min(60, len(key))], storyTitle),
		})
		merged = append(merged, base)
	}

	return merged
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// mergeSkippedAt returns the union of two per-quiz-type skip maps,
// keeping the later timestamp when both record a skip for the same quiz
// type. A nil/empty input is treated as "no skips".
func mergeSkippedAt(a, b SkippedAtMap) SkippedAtMap {
	if len(a) == 0 && len(b) == 0 {
		return a
	}
	out := make(SkippedAtMap, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if existing, ok := out[k]; !ok || v > existing {
			out[k] = v
		}
	}
	return out
}

// renameDefinitionsIndexScenes rewrites learning-history scene titles
// that use the legacy "__index_N" key to the human scene title the quiz
// now reads (via Reader.GetDefinitionsNotesByTitle). Definitions-book
// learning history was historically written under two conventions for
// the same scene — "__index_N" (old quiz path) and the human title
// (detail page / skip path) — which split a word's logs and skip across
// two scene entries. This pass resolves "__index_N" to its human title
// using the definitions source, then leaves the now-duplicate titles for
// mergeDuplicateScenes (run later in fixLearningNotesStructure) to
// collapse, unioning logs and skip state.
//
// Only learning files whose notebook ID is a definitions book are
// touched; story/flashcard histories keep their titles. Scenes whose
// index can't be resolved to a human title are left as-is.
func (v *Validator) renameDefinitionsIndexScenes(files []learningHistoryFile, result *ValidationResult) []learningHistoryFile {
	if len(v.definitionsDirs) == 0 {
		return files
	}
	_, defsRaw, _, err := NewDefinitionsMap(v.definitionsDirs)
	if err != nil {
		return files
	}

	// Build (bookID, session, index) -> human scene title.
	type key struct {
		book, session string
		index         int
	}
	titleByIndex := make(map[key]string)
	for bookID, fileDefs := range defsRaw {
		for _, def := range fileDefs {
			session := def.Metadata.Notebook
			if session == "" {
				session = def.Metadata.Title
			}
			for _, scene := range def.Scenes {
				titleByIndex[key{bookID, session, scene.Metadata.GetIndex()}] = scene.Metadata.Title
			}
		}
	}
	if len(titleByIndex) == 0 {
		return files
	}

	for fileIdx := range files {
		file := &files[fileIdx]
		bookID := strings.TrimSuffix(filepath.Base(file.path), ".yml")
		for histIdx := range file.contents {
			hist := &file.contents[histIdx]
			session := hist.Metadata.Title
			for sceneIdx := range hist.Scenes {
				title := hist.Scenes[sceneIdx].Metadata.Title
				var n int
				if _, err := fmt.Sscanf(title, "__index_%d", &n); err != nil {
					continue
				}
				human, ok := titleByIndex[key{bookID, session, n}]
				if !ok || human == "" || human == title {
					continue
				}
				hist.Scenes[sceneIdx].Metadata.Title = human
				result.AddWarning(ValidationError{
					File:    file.path,
					Message: fmt.Sprintf("Renamed legacy scene key %q to %q in %s", title, human, session),
				})
			}
		}
	}
	return files
}

// isEtymologyOriginEntry reports whether a learning-history expression is
// an etymology origin (rather than a vocabulary word). Origins are typed
// "origin" and/or carry etymology_*_logs; vocabulary words never have
// etymology logs, so either signal is sufficient.
func isEtymologyOriginEntry(e *LearningHistoryExpression) bool {
	if e.Type == LearningExpressionTypeOrigin {
		return true
	}
	return len(e.EtymologyBreakdownLogs) > 0 || len(e.EtymologyAssemblyLogs) > 0
}

// latestLogTime returns the most recent LearnedAt across all of an
// expression's log slots (zero time if it has none). Used to pick the
// merge target when an origin is split across scenes — the scene with
// the freshest activity is where the live quiz currently writes.
func latestLogTime(e *LearningHistoryExpression) time.Time {
	var latest time.Time
	consider := func(logs []LearningRecord) {
		for _, l := range logs {
			if l.LearnedAt.Time.After(latest) {
				latest = l.LearnedAt.Time
			}
		}
	}
	consider(e.LearnedLogs)
	consider(e.ReverseLogs)
	consider(e.EtymologyBreakdownLogs)
	consider(e.EtymologyAssemblyLogs)
	return latest
}

// mergeOriginExpressionInto folds other's logs and skip state into base
// (recalculating each log slot through the SR calculator after the
// append, matching mergeDuplicateScenes).
func (v *Validator) mergeOriginExpressionInto(base, other *LearningHistoryExpression) {
	if other.Type == LearningExpressionTypeOrigin {
		base.Type = LearningExpressionTypeOrigin
	}
	if len(other.LearnedLogs) > 0 {
		base.LearnedLogs = append(base.LearnedLogs, other.LearnedLogs...)
		_, base.LearnedLogs, _ = recalculateLearningLogs(base.LearnedLogs, v.calculator)
	}
	if len(other.ReverseLogs) > 0 {
		base.ReverseLogs = append(base.ReverseLogs, other.ReverseLogs...)
		_, base.ReverseLogs, _ = recalculateLearningLogs(base.ReverseLogs, v.calculator)
	}
	if len(other.EtymologyBreakdownLogs) > 0 {
		base.EtymologyBreakdownLogs = append(base.EtymologyBreakdownLogs, other.EtymologyBreakdownLogs...)
		_, base.EtymologyBreakdownLogs, _ = recalculateLearningLogs(base.EtymologyBreakdownLogs, v.calculator)
	}
	if len(other.EtymologyAssemblyLogs) > 0 {
		base.EtymologyAssemblyLogs = append(base.EtymologyAssemblyLogs, other.EtymologyAssemblyLogs...)
		_, base.EtymologyAssemblyLogs, _ = recalculateLearningLogs(base.EtymologyAssemblyLogs, v.calculator)
	}
	base.SkippedAt = mergeSkippedAt(base.SkippedAt, other.SkippedAt)
}

// consolidateEtymologyOriginScenes repairs an etymology origin whose
// learning history is split across two scene titles within one session.
// The etymology SceneTitle for legacy-shape origins is DERIVED from where
// the origin is referenced in definitions (see pickBestSceneForOrigin);
// when that derivation changes (data edits, or the pre-determinism map-
// order flakiness), the same origin's logs get written under a new scene
// title while the old logs linger under the previous one — the "multiple
// gamos records" symptom.
//
// For each session, origins appearing under 2+ scenes are merged into a
// single scene: the canonically-derived scene when it's among the
// current locations, else the location with the freshest log (where the
// live quiz writes today). Logs and skip state are unioned; the origin
// entry is removed from the other scenes; scenes left empty are dropped.
func (v *Validator) consolidateEtymologyOriginScenes(files []learningHistoryFile, result *ValidationResult) []learningHistoryFile {
	if len(v.etymologyDirs) == 0 {
		return files
	}
	candidates := v.buildOriginSceneIndexForValidator()

	for fileIdx := range files {
		file := &files[fileIdx]
		notebookID := strings.TrimSuffix(filepath.Base(file.path), ".yml")
		for histIdx := range file.contents {
			hist := &file.contents[histIdx]
			session := hist.Metadata.Title

			// origin key -> scene indices it appears in (with expr index).
			type loc struct{ scene, expr int }
			locs := make(map[string][]loc)
			for si := range hist.Scenes {
				for ei := range hist.Scenes[si].Expressions {
					e := &hist.Scenes[si].Expressions[ei]
					if !isEtymologyOriginEntry(e) {
						continue
					}
					key := strings.ToLower(strings.TrimSpace(e.Expression))
					locs[key] = append(locs[key], loc{si, ei})
				}
			}

			// removals[scene] = set of expr indices to drop after merging.
			removals := make(map[int]map[int]bool)
			// replacements[scene][expr] = the merged expression to write.
			replacements := make(map[int]map[int]LearningHistoryExpression)
			markRemove := func(s, e int) {
				if removals[s] == nil {
					removals[s] = make(map[int]bool)
				}
				removals[s][e] = true
			}

			for origin, ls := range locs {
				// distinct scenes only — same-scene dups are handled elsewhere.
				distinct := make(map[int]bool)
				for _, l := range ls {
					distinct[l.scene] = true
				}
				if len(distinct) < 2 {
					continue
				}

				canonical := pickBestSceneForOrigin(candidates, origin, notebookID, session)

				// Choose the target location: the one whose scene title is
				// canonical, else the one with the freshest log.
				targetIdx := -1
				var targetTime time.Time
				for i, l := range ls {
					sceneTitle := hist.Scenes[l.scene].Metadata.Title
					if canonical != "" && normalizeQuotes(sceneTitle) == normalizeQuotes(canonical) {
						targetIdx = i
						break
					}
					if t := latestLogTime(&hist.Scenes[l.scene].Expressions[l.expr]); targetIdx < 0 || t.After(targetTime) {
						targetIdx = i
						targetTime = t
					}
				}
				if targetIdx < 0 {
					continue
				}
				target := ls[targetIdx]

				merged := hist.Scenes[target.scene].Expressions[target.expr]
				for i, l := range ls {
					if i == targetIdx {
						continue
					}
					other := hist.Scenes[l.scene].Expressions[l.expr]
					v.mergeOriginExpressionInto(&merged, &other)
					markRemove(l.scene, l.expr)
				}
				if replacements[target.scene] == nil {
					replacements[target.scene] = make(map[int]LearningHistoryExpression)
				}
				replacements[target.scene][target.expr] = merged
				result.AddWarning(ValidationError{
					File: file.path,
					Message: fmt.Sprintf("Consolidated etymology origin %q split across %d scenes into %q (session %s)",
						origin, len(distinct), hist.Scenes[target.scene].Metadata.Title, session),
				})
			}

			if len(removals) == 0 && len(replacements) == 0 {
				continue
			}

			// Rebuild scenes, applying replacements and removals, dropping
			// scenes that become empty.
			var newScenes []LearningScene
			for si := range hist.Scenes {
				scene := hist.Scenes[si]
				var kept []LearningHistoryExpression
				for ei := range scene.Expressions {
					if removals[si] != nil && removals[si][ei] {
						continue
					}
					if repl, ok := replacements[si][ei]; ok {
						kept = append(kept, repl)
						continue
					}
					kept = append(kept, scene.Expressions[ei])
				}
				if len(kept) == 0 {
					continue
				}
				scene.Expressions = kept
				newScenes = append(newScenes, scene)
			}
			hist.Scenes = newScenes
		}
	}
	return files
}

// consolidateDefinitionsScenes moves a definitions-book vocabulary word's
// learning history to the scene where the word is actually defined. An
// old vocab path wrote freeform/quiz logs under a synthetic scene named
// after the SESSION (e.g. a scene literally titled "Session 3") instead
// of the word's real definitions scene ("dexter (right hand)"). That left
// words mis-scened (logs invisible to the quiz, which reads the real
// scene) or duplicated (same word under both the synthetic scene and the
// real one — the "multiple dexterity records" symptom).
//
// For each session, every vocabulary expression whose definitions scene
// is known is merged into that canonical scene (logs + skip unioned),
// removing it from any other scene; emptied scenes are dropped. Words
// that can't be matched to a definitions scene, or that the book defines
// under more than one scene in the same session (ambiguous), are left
// untouched — the migration never guesses a home.
//
// Etymology origins are handled by consolidateEtymologyOriginScenes;
// this pass deliberately skips them (isEtymologyOriginEntry).
func (v *Validator) consolidateDefinitionsScenes(files []learningHistoryFile, result *ValidationResult) []learningHistoryFile {
	if len(v.definitionsDirs) == 0 {
		return files
	}
	_, defsRaw, _, err := NewDefinitionsMap(v.definitionsDirs)
	if err != nil {
		return files
	}

	type sessKey struct{ book, session, expr string }
	canonical := make(map[sessKey]string)
	ambiguous := make(map[sessKey]bool)
	recordExpr := func(book, session, raw, scene string) {
		key := strings.ToLower(strings.TrimSpace(raw))
		if key == "" || scene == "" {
			return
		}
		k := sessKey{book, session, key}
		if existing, ok := canonical[k]; ok {
			if normalizeQuotes(existing) != normalizeQuotes(scene) {
				ambiguous[k] = true
			}
			return
		}
		canonical[k] = scene
	}
	for book, fileDefs := range defsRaw {
		for _, def := range fileDefs {
			session := def.Metadata.Notebook
			if session == "" {
				session = def.Metadata.Title
			}
			for _, scene := range def.Scenes {
				st := scene.Metadata.Title
				for _, note := range scene.Expressions {
					recordExpr(book, session, note.Expression, st)
					recordExpr(book, session, note.Definition, st)
				}
			}
		}
	}

	for fileIdx := range files {
		file := &files[fileIdx]
		book := strings.TrimSuffix(filepath.Base(file.path), ".yml")
		for histIdx := range file.contents {
			hist := &file.contents[histIdx]
			session := hist.Metadata.Title

			type loc struct{ scene, expr int }
			locs := make(map[string][]loc)
			for si := range hist.Scenes {
				for ei := range hist.Scenes[si].Expressions {
					e := &hist.Scenes[si].Expressions[ei]
					if isEtymologyOriginEntry(e) {
						continue
					}
					key := strings.ToLower(strings.TrimSpace(e.Expression))
					locs[key] = append(locs[key], loc{si, ei})
				}
			}

			removals := make(map[int]map[int]bool)
			replacements := make(map[int]map[int]LearningHistoryExpression)
			additions := make(map[int][]LearningHistoryExpression)
			markRemove := func(s, e int) {
				if removals[s] == nil {
					removals[s] = make(map[int]bool)
				}
				removals[s][e] = true
			}
			sceneIdxByTitle := make(map[string]int)
			for si := range hist.Scenes {
				sceneIdxByTitle[normalizeQuotes(hist.Scenes[si].Metadata.Title)] = si
			}

			for exprKey, ls := range locs {
				k := sessKey{book, session, exprKey}
				canonScene, ok := canonical[k]
				if !ok || canonScene == "" || ambiguous[k] {
					continue // unmatched or ambiguous: leave untouched
				}
				canonNorm := normalizeQuotes(canonScene)

				// Nothing to do when the sole entry is already canonical.
				if len(ls) == 1 && normalizeQuotes(hist.Scenes[ls[0].scene].Metadata.Title) == canonNorm {
					continue
				}

				// Prefer an existing canonical-scene entry as the merge
				// target so its position is preserved; otherwise the word
				// is moved into the canonical scene wholesale.
				targetIdx := -1
				for i, l := range ls {
					if normalizeQuotes(hist.Scenes[l.scene].Metadata.Title) == canonNorm {
						targetIdx = i
						break
					}
				}

				if targetIdx >= 0 {
					target := ls[targetIdx]
					merged := hist.Scenes[target.scene].Expressions[target.expr]
					for i, l := range ls {
						if i == targetIdx {
							continue
						}
						other := hist.Scenes[l.scene].Expressions[l.expr]
						v.mergeOriginExpressionInto(&merged, &other)
						markRemove(l.scene, l.expr)
					}
					if replacements[target.scene] == nil {
						replacements[target.scene] = make(map[int]LearningHistoryExpression)
					}
					replacements[target.scene][target.expr] = merged
				} else {
					merged := hist.Scenes[ls[0].scene].Expressions[ls[0].expr]
					for _, l := range ls[1:] {
						other := hist.Scenes[l.scene].Expressions[l.expr]
						v.mergeOriginExpressionInto(&merged, &other)
					}
					for _, l := range ls {
						markRemove(l.scene, l.expr)
					}
					if ti, ok := sceneIdxByTitle[canonNorm]; ok {
						additions[ti] = append(additions[ti], merged)
					} else {
						hist.Scenes = append(hist.Scenes, LearningScene{
							Metadata:    LearningSceneMetadata{Title: canonScene},
							Expressions: []LearningHistoryExpression{merged},
						})
						sceneIdxByTitle[canonNorm] = len(hist.Scenes) - 1
					}
				}
				result.AddWarning(ValidationError{
					File: file.path,
					Message: fmt.Sprintf("Moved vocabulary %q to its definitions scene %q (session %s)",
						exprKey, canonScene, session),
				})
			}

			if len(removals) == 0 && len(replacements) == 0 && len(additions) == 0 {
				continue
			}

			var newScenes []LearningScene
			for si := range hist.Scenes {
				scene := hist.Scenes[si]
				var kept []LearningHistoryExpression
				for ei := range scene.Expressions {
					if removals[si] != nil && removals[si][ei] {
						continue
					}
					if repl, ok := replacements[si][ei]; ok {
						kept = append(kept, repl)
						continue
					}
					kept = append(kept, scene.Expressions[ei])
				}
				kept = append(kept, additions[si]...)
				if len(kept) == 0 {
					continue
				}
				scene.Expressions = kept
				newScenes = append(newScenes, scene)
			}
			hist.Scenes = newScenes
		}
	}
	return files
}

func (v *Validator) fixConsistency(
	learningFiles []learningHistoryFile,
	storyFiles []storyNotebookFile,
	result *ValidationResult,
) []learningHistoryFile {
	// Build index of expressions in story notebooks
	storyExpressions := make(map[string]map[string]bool) // map[sceneKey]map[expression]bool

	for _, file := range storyFiles {
		for _, notebook := range file.contents {
			for _, scene := range notebook.Scenes {
				sceneKey := fmt.Sprintf("%s::%s", notebook.Event, scene.Title)
				if storyExpressions[sceneKey] == nil {
					storyExpressions[sceneKey] = make(map[string]bool)
				}

				for _, def := range scene.Definitions {
					expr := strings.TrimSpace(def.Expression)
					if expr != "" {
						storyExpressions[sceneKey][expr] = true
					}
					if def.Definition != "" {
						storyExpressions[sceneKey][strings.TrimSpace(def.Definition)] = true
					}
				}
			}
		}
	}

	// Fix orphaned and mismatched expressions
	for _, file := range learningFiles {
		for histIdx := range file.contents {
			for sceneIdx := range file.contents[histIdx].Scenes {
				scene := &file.contents[histIdx].Scenes[sceneIdx]
				sceneKey := fmt.Sprintf("%s::%s", file.contents[histIdx].Metadata.Title, scene.Metadata.Title)

				// Filter out expressions that don't exist in story notebooks or have empty learned_logs
				validExpressions := make([]LearningHistoryExpression, 0)

				for _, expr := range scene.Expressions {
					expression := strings.TrimSpace(expr.Expression)
					if expression == "" {
						continue
					}

					// Check if expression exists in the correct scene
					existsInStory := false
					if sceneExprs, ok := storyExpressions[sceneKey]; ok && sceneExprs[expression] {
						existsInStory = true
					}

					// Only keep expressions that either:
					// 1. Have any logs (learned, reverse, or etymology), OR
					// 2. Exist in the story (even with empty logs), OR
					// 3. Are skipped from at least one quiz mode — the
					//    notebook detail page seeds skip-only stubs that
					//    would otherwise be dropped on the next --fix.
					hasLogs := len(expr.LearnedLogs) > 0 || len(expr.ReverseLogs) > 0 || len(expr.EtymologyBreakdownLogs) > 0 || len(expr.EtymologyAssemblyLogs) > 0
					hasSkip := expr.SkippedAt.IsSkippedAny()
					if hasLogs || existsInStory || hasSkip {
						validExpressions = append(validExpressions, expr)
					} else {
						// Remove orphaned expressions with no logs
						result.AddWarning(ValidationError{
							File:    file.path,
							Message: fmt.Sprintf("Removed orphaned expression %q with no learned_logs from scene %s", expression, sceneKey),
						})
					}
				}

				scene.Expressions = validExpressions
			}
		}
	}

	return learningFiles
}

// recalculateAllIntervals replays every expression's log slices through
// the configured SR calculator (RecalculateAll). The calculator threads
// the prior log's interval into each step so the early-review guard can
// preserve hard-earned intervals: an answer that comes before the
// previous interval elapsed will not grow OR shrink the interval based
// on a single short gap. This makes --fix able to correct data that was
// written under a buggy live-quiz code path (e.g. the old etymology
// quiz that wrote intervals without consulting prior state).
//
// Only logs whose interval_days actually changes get rewritten; matching
// values stay untouched so the YAML diff stays small. Each drift is
// reported as a warning so the user can audit what got corrected.
func (v *Validator) recalculateAllIntervals(files []learningHistoryFile, result *ValidationResult) []learningHistoryFile {
	if v.calculator == nil {
		return files
	}

	recalcSlice := func(logs []LearningRecord, file, exprLabel, slot string) []LearningRecord {
		if len(logs) == 0 {
			return logs
		}
		oldByTs := make(map[time.Time]int, len(logs))
		for _, log := range logs {
			oldByTs[log.LearnedAt.Time] = log.IntervalDays
		}
		_, recalc := v.calculator.RecalculateAll(logs)
		for _, log := range recalc {
			old, ok := oldByTs[log.LearnedAt.Time]
			if !ok || old == log.IntervalDays {
				continue
			}
			result.AddWarning(ValidationError{
				File: file,
				Message: fmt.Sprintf(
					"Recalculated interval_days for %q [%s] at %s: %d → %d",
					exprLabel, slot, log.LearnedAt.Time.Format("2006-01-02"), old, log.IntervalDays,
				),
			})
		}
		return recalc
	}

	for fileIdx := range files {
		file := &files[fileIdx]
		for histIdx := range file.contents {
			history := &file.contents[histIdx]
			recalcExpr := func(expr *LearningHistoryExpression) {
				label := expr.Expression
				expr.LearnedLogs = recalcSlice(expr.LearnedLogs, file.path, label, "learned_logs")
				expr.ReverseLogs = recalcSlice(expr.ReverseLogs, file.path, label, "reverse_logs")
				expr.EtymologyBreakdownLogs = recalcSlice(
					expr.EtymologyBreakdownLogs, file.path, label, "etymology_breakdown_logs")
				expr.EtymologyAssemblyLogs = recalcSlice(
					expr.EtymologyAssemblyLogs, file.path, label, "etymology_assembly_logs")
			}
			// Flashcard shape: top-level expressions.
			for eIdx := range history.Expressions {
				recalcExpr(&history.Expressions[eIdx])
			}
			// Story shape: nested under scenes.
			for sIdx := range history.Scenes {
				for eIdx := range history.Scenes[sIdx].Expressions {
					recalcExpr(&history.Scenes[sIdx].Expressions[eIdx])
				}
			}
		}
	}
	return files
}

// backfillQuizType sets quiz_type to "freeform" on any learning log that has
// status "usable" but no quiz_type. Older learning logs predate the quiz_type
// field; the freeform quiz is the only path that produces "usable" status on
// a correct answer (isKnownWord=false), so this backfill is safe.
func (v *Validator) backfillQuizType(learningFiles []learningHistoryFile, result *ValidationResult) []learningHistoryFile {
	for fileIdx := range learningFiles {
		file := &learningFiles[fileIdx]
		for histIdx := range file.contents {
			history := &file.contents[histIdx]
			fillFreeformLogs := func(logs []LearningRecord, expr string) {
				for i := range logs {
					if logs[i].Status == LearnedStatusCanBeUsed && logs[i].QuizType == "" {
						logs[i].QuizType = string(QuizTypeFreeform)
						result.AddWarning(ValidationError{
							File:    file.path,
							Message: fmt.Sprintf("Backfilled quiz_type=freeform on usable log for %q", expr),
						})
					}
				}
			}
			// Flashcard-style: top-level expressions
			for eIdx := range history.Expressions {
				fillFreeformLogs(history.Expressions[eIdx].LearnedLogs, history.Expressions[eIdx].Expression)
			}
			// Story-style: nested under scenes
			for sIdx := range history.Scenes {
				for eIdx := range history.Scenes[sIdx].Expressions {
					fillFreeformLogs(history.Scenes[sIdx].Expressions[eIdx].LearnedLogs, history.Scenes[sIdx].Expressions[eIdx].Expression)
				}
			}
		}
	}
	return learningFiles
}

func (v *Validator) fixMismatchedScenes(learningFiles []learningHistoryFile, storyFiles []storyNotebookFile, result *ValidationResult) []learningHistoryFile {
	// Build a map to find the correct scene for each expression
	// Map structure: event -> expression -> scene title
	expressionScenes := make(map[string]map[string]string)
	for _, storyFile := range storyFiles {
		for _, notebook := range storyFile.contents {
			eventKey := notebook.Event
			if expressionScenes[eventKey] == nil {
				expressionScenes[eventKey] = make(map[string]string)
			}
			for _, scene := range notebook.Scenes {
				for _, def := range scene.Definitions {
					expr := strings.TrimSpace(def.Expression)
					if expr != "" {
						expressionScenes[eventKey][expr] = scene.Title
					}
					if def.Definition != "" {
						defExpr := strings.TrimSpace(def.Definition)
						expressionScenes[eventKey][defExpr] = scene.Title
					}
				}
			}
		}
	}

	// Fix mismatched scenes
	for fileIdx := range learningFiles {
		file := &learningFiles[fileIdx]
		for histIdx := range file.contents {
			history := &file.contents[histIdx]
			eventKey := history.Metadata.Title

			// Group expressions by their correct scenes
			correctSceneExprs := make(map[string][]LearningHistoryExpression)

			for sceneIdx := range history.Scenes {
				scene := &history.Scenes[sceneIdx]
				currentSceneKey := scene.Metadata.Title

				var remainingExprs []LearningHistoryExpression

				for _, expr := range scene.Expressions {
					expression := strings.TrimSpace(expr.Expression)
					if expression == "" {
						remainingExprs = append(remainingExprs, expr)
						continue
					}

					// Check if this expression belongs to a different scene
					if correctScene, exists := expressionScenes[eventKey][expression]; exists {
						if correctScene != currentSceneKey {
							// Move to correct scene
							correctSceneExprs[correctScene] = append(correctSceneExprs[correctScene], expr)
							result.AddWarning(ValidationError{
								File:    file.path,
								Message: fmt.Sprintf("Moved expression %q from scene %q to correct scene %q", expression, currentSceneKey, correctScene),
							})
						} else {
							// Already in correct scene
							remainingExprs = append(remainingExprs, expr)
						}
					} else {
						// Expression not found in story, keep it in current scene
						remainingExprs = append(remainingExprs, expr)
					}
				}

				scene.Expressions = remainingExprs
			}

			// Add moved expressions to their correct scenes
			for correctSceneTitle, exprs := range correctSceneExprs {
				// Find or create the correct scene
				var targetScene *LearningScene
				for sceneIdx := range history.Scenes {
					if history.Scenes[sceneIdx].Metadata.Title == correctSceneTitle {
						targetScene = &history.Scenes[sceneIdx]
						break
					}
				}

				if targetScene == nil {
					// Create new scene
					newScene := LearningScene{
						Metadata: LearningSceneMetadata{
							Title: correctSceneTitle,
						},
						Expressions: exprs,
					}
					history.Scenes = append(history.Scenes, newScene)
				} else {
					// Add to existing scene
					targetScene.Expressions = append(targetScene.Expressions, exprs...)
				}
			}
		}
	}

	return learningFiles
}

func (v *Validator) fixDictionaryReferences(files []storyNotebookFile, result *ValidationResult) []storyNotebookFile {
	for fileIdx := range files {
		file := &files[fileIdx]
		for notebookIdx := range file.contents {
			notebook := &file.contents[notebookIdx]
			for sceneIdx := range notebook.Scenes {
				scene := &notebook.Scenes[sceneIdx]
				for defIdx := range scene.Definitions {
					def := &scene.Definitions[defIdx]

					if def.DictionaryNumber <= 0 {
						continue
					}

					// Determine the word to look up
					word := def.Definition
					if word == "" {
						word = def.Expression
					}
					word = strings.ToLower(strings.TrimSpace(word))

					// Check if dictionary file exists
					dictPath := filepath.Join(v.dictionaryDir, word+".json")
					if _, err := os.Stat(dictPath); os.IsNotExist(err) {
						// Remove dictionary_number field
						def.DictionaryNumber = 0
						result.AddWarning(ValidationError{
							File:    file.path,
							Message: fmt.Sprintf("Removed dictionary_number for word %q (dictionary file not found: %s)", word, dictPath),
						})
					}
				}
			}
		}
	}

	return files
}

func (v *Validator) fixExpressionNames(learningFiles []learningHistoryFile, storyFiles []storyNotebookFile, result *ValidationResult) []learningHistoryFile {
	// Build a map of story expressions to their definitions
	// Map structure: event -> scene -> expression -> definition
	storyDefinitions := make(map[string]map[string]map[string]string)
	for _, storyFile := range storyFiles {
		for _, notebook := range storyFile.contents {
			eventKey := notebook.Event
			if storyDefinitions[eventKey] == nil {
				storyDefinitions[eventKey] = make(map[string]map[string]string)
			}
			for _, scene := range notebook.Scenes {
				sceneKey := scene.Title
				if storyDefinitions[eventKey][sceneKey] == nil {
					storyDefinitions[eventKey][sceneKey] = make(map[string]string)
				}
				for _, def := range scene.Definitions {
					expr := strings.TrimSpace(def.Expression)
					if expr != "" && def.Definition != "" {
						definition := strings.TrimSpace(def.Definition)
						storyDefinitions[eventKey][sceneKey][expr] = definition
					}
				}
			}
		}
	}

	// Fix learning note expressions to use definitions from stories
	for fileIdx := range learningFiles {
		file := &learningFiles[fileIdx]
		for histIdx := range file.contents {
			history := &file.contents[histIdx]
			eventKey := history.Metadata.Title

			for sceneIdx := range history.Scenes {
				scene := &history.Scenes[sceneIdx]
				sceneKey := scene.Metadata.Title

				for exprIdx := range scene.Expressions {
					expr := &scene.Expressions[exprIdx]
					currentExpr := strings.TrimSpace(expr.Expression)

					// Check if this expression should use a definition instead
					if sceneDefs, ok := storyDefinitions[eventKey][sceneKey]; ok {
						if definition, exists := sceneDefs[currentExpr]; exists {
							// Update to use the definition
							if currentExpr != definition {
								expr.Expression = definition
								result.AddWarning(ValidationError{
									File:    file.path,
									Message: fmt.Sprintf("Updated expression %q to use definition %q in scene %s::%s", currentExpr, definition, eventKey, sceneKey),
								})
							}
						}
					}
				}
			}
		}
	}

	return learningFiles
}

func (v *Validator) createMissingLearningNotes(learningFiles []learningHistoryFile, storyFiles []storyNotebookFile, result *ValidationResult) []learningHistoryFile {
	// Build index of existing learning notes by event and scene
	existingNotes := make(map[string]map[string]map[string]bool) // event -> scene -> expression -> exists
	for _, file := range learningFiles {
		for _, history := range file.contents {
			eventKey := history.Metadata.Title
			if existingNotes[eventKey] == nil {
				existingNotes[eventKey] = make(map[string]map[string]bool)
			}
			for _, scene := range history.Scenes {
				sceneKey := scene.Metadata.Title
				if existingNotes[eventKey][sceneKey] == nil {
					existingNotes[eventKey][sceneKey] = make(map[string]bool)
				}
				for _, expr := range scene.Expressions {
					expression := strings.TrimSpace(expr.Expression)
					if expression != "" {
						existingNotes[eventKey][sceneKey][expression] = true
					}
				}
			}
		}
	}

	// Check for missing learning notes and create them
	for _, storyFile := range storyFiles {
		for _, notebook := range storyFile.contents {
			eventKey := notebook.Event
			for _, scene := range notebook.Scenes {
				sceneKey := scene.Title
				for _, def := range scene.Definitions {
					expr := strings.TrimSpace(def.Expression)
					if expr == "" {
						continue
					}

					// Determine which expression to use in learning notes
					// Prefer definition field if it exists, otherwise use expression
					learningExpr := expr
					if def.Definition != "" {
						learningExpr = strings.TrimSpace(def.Definition)
					}

					// Check if this expression has learning notes
					hasNote := false
					if eventScenes, ok := existingNotes[eventKey]; ok {
						if sceneExprs, ok := eventScenes[sceneKey]; ok {
							if sceneExprs[learningExpr] {
								hasNote = true
							}
						}
					}

					if !hasNote {
						// Find or create the learning history file and scene
						created := v.addMissingLearningNote(&learningFiles, &notebook, eventKey, sceneKey, learningExpr, result)
						if created {
							// Update the index
							if existingNotes[eventKey] == nil {
								existingNotes[eventKey] = make(map[string]map[string]bool)
							}
							if existingNotes[eventKey][sceneKey] == nil {
								existingNotes[eventKey][sceneKey] = make(map[string]bool)
							}
							existingNotes[eventKey][sceneKey][learningExpr] = true
						}
					}
				}
			}
		}
	}

	return learningFiles
}

func (v *Validator) addMissingLearningNote(learningFiles *[]learningHistoryFile, notebook *StoryNotebook, eventKey, sceneKey, expression string, result *ValidationResult) bool {
	// Find the appropriate learning history file based on event
	var targetFile *learningHistoryFile
	var targetHistory *LearningHistory
	var targetScene *LearningScene

	// Look for existing file and history with this event
	for fileIdx := range *learningFiles {
		file := &(*learningFiles)[fileIdx]
		for histIdx := range file.contents {
			history := &file.contents[histIdx]
			if history.Metadata.Title == eventKey {
				targetFile = file
				targetHistory = history

				// Look for existing scene
				for sceneIdx := range history.Scenes {
					if history.Scenes[sceneIdx].Metadata.Title == sceneKey {
						targetScene = &history.Scenes[sceneIdx]
						break
					}
				}
				break
			}
		}
		if targetFile != nil {
			break
		}
	}

	// If we didn't find the event, try to create it in the appropriate file
	if targetFile == nil || targetHistory == nil {
		// Try to find the file by matching the series/show name
		// For now, we'll look for a file that has at least one history entry
		// and assume new episodes belong to the same file
		for fileIdx := range *learningFiles {
			file := &(*learningFiles)[fileIdx]
			if len(file.contents) > 0 {
				// Check if the event key matches the pattern of existing events
				// (simple heuristic: if any existing event title is a substring or vice versa)
				for _, hist := range file.contents {
					if v.eventsRelated(hist.Metadata.Title, eventKey) {
						targetFile = file
						break
					}
				}
				if targetFile != nil {
					break
				}
			}
		}

		// If we still can't find a file, create a new one
		if targetFile == nil {
			// Derive notebook ID from the story metadata
			notebookID := deriveNotebookID(notebook)
			if notebookID == "" {
				return false
			}

			// Create a new learning history file
			newFilePath := filepath.Join(v.learningNotesDir, notebookID+".yml")
			newFile := learningHistoryFile{
				path:     newFilePath,
				contents: []LearningHistory{},
			}
			*learningFiles = append(*learningFiles, newFile)
			targetFile = &(*learningFiles)[len(*learningFiles)-1]

			result.AddWarning(ValidationError{
				File:    newFilePath,
				Message: fmt.Sprintf("Created new learning history file for notebook ID %q", notebookID),
			})
		}

		// Create the new event/episode structure
		notebookID := deriveNotebookID(notebook)
		if len(targetFile.contents) > 0 {
			notebookID = targetFile.contents[0].Metadata.NotebookID
		}
		newHistory := LearningHistory{
			Metadata: LearningHistoryMetadata{
				NotebookID: notebookID,
				Title:      eventKey,
			},
			Scenes: []LearningScene{},
		}
		targetFile.contents = append(targetFile.contents, newHistory)
		targetHistory = &targetFile.contents[len(targetFile.contents)-1]

		result.AddWarning(ValidationError{
			File:    targetFile.path,
			Message: fmt.Sprintf("Created new episode structure for %q", eventKey),
		})
	}

	// If scene doesn't exist, create it
	if targetScene == nil {
		newScene := LearningScene{
			Metadata: LearningSceneMetadata{
				Title: sceneKey,
			},
			Expressions: []LearningHistoryExpression{},
		}
		targetHistory.Scenes = append(targetHistory.Scenes, newScene)
		targetScene = &targetHistory.Scenes[len(targetHistory.Scenes)-1]
	}

	// Add the expression with empty learned_logs
	targetScene.Expressions = append(targetScene.Expressions, LearningHistoryExpression{
		Expression:  expression,
		LearnedLogs: []LearningRecord{},
	})

	result.AddWarning(ValidationError{
		File:    targetFile.path,
		Message: fmt.Sprintf("Created missing learning note for expression %q in scene %s::%s", expression, eventKey, sceneKey),
	})

	return true
}

// deriveNotebookID converts the story notebook metadata into a notebook ID.
// The series name is converted to lowercase with spaces replaced by hyphens.
func deriveNotebookID(notebook *StoryNotebook) string {
	if notebook.Metadata.Series != "" {
		// Convert series name to lowercase and replace spaces with hyphens
		return strings.ToLower(strings.ReplaceAll(notebook.Metadata.Series, " ", "-"))
	}
	return ""
}

// eventsRelated checks if two event titles are from the same series
// Simple heuristic: check if they share common keywords like "Episode", "Season", series name, etc.
func (v *Validator) eventsRelated(event1, event2 string) bool {
	// Extract common patterns
	e1Lower := strings.ToLower(event1)
	e2Lower := strings.ToLower(event2)

	// Check for common series patterns
	// If both contain "Episode" or "Season", they're likely from the same series
	if (strings.Contains(e1Lower, "episode") && strings.Contains(e2Lower, "episode")) ||
		(strings.Contains(e1Lower, "season") && strings.Contains(e2Lower, "season")) {
		// Extract potential series name (before "Episode" or "Season")
		series1 := extractSeriesName(e1Lower)
		series2 := extractSeriesName(e2Lower)

		// If both have same series name prefix, they're related
		if series1 != "" && series2 != "" && series1 == series2 {
			return true
		}
	}

	return false
}

// extractSeriesName extracts the series name from an event title
func extractSeriesName(eventLower string) string {
	// Look for "season" or "episode" and take everything before it
	if idx := strings.Index(eventLower, "season"); idx > 0 {
		return strings.TrimSpace(eventLower[:idx])
	}
	if idx := strings.Index(eventLower, "episode"); idx > 0 {
		return strings.TrimSpace(eventLower[:idx])
	}
	return ""
}

// validateDefinitionsInConversations checks that each definition appears at least once in conversations
func (v *Validator) validateDefinitionsInConversations(files []storyNotebookFile, result *ValidationResult) {
	for _, file := range files {
		for notebookIdx, notebook := range file.contents {
			notebookLocation := fmt.Sprintf("notebook[%d]: %s", notebookIdx, notebook.Event)

			// Call the Validate method on the StoryNotebook object
			errors := notebook.Validate(notebookLocation)
			for _, err := range errors {
				err.File = file.path
				result.AddError("consistency", err)
			}
		}
	}
}

// validateSeparateDefinitionsInConversations checks that definitions from separate
// definitions files appear in the matching story notebook conversations/statements.
func (v *Validator) validateSeparateDefinitionsInConversations(storyFiles []storyNotebookFile, result *ValidationResult) {
	defsMap, _, _, err := NewDefinitionsMap(v.definitionsDirs)
	if err != nil {
		return
	}

	// Build a lookup: bookID -> event -> scene index -> StoryScene
	type sceneKey struct {
		bookID string
		event  string
		index  int
	}
	sceneMap := make(map[sceneKey]*StoryScene)

	for _, file := range storyFiles {
		bookID := filepath.Base(filepath.Dir(file.path))
		for nbIdx := range file.contents {
			nb := &file.contents[nbIdx]
			for scIdx := range nb.Scenes {
				scene := &nb.Scenes[scIdx]
				key := sceneKey{bookID: bookID, event: nb.Event, index: scIdx}
				sceneMap[key] = scene
			}
		}
	}

	// Check each definition from definitions files
	for bookID, notebookDefs := range defsMap {
		for eventTitle, sceneDefs := range notebookDefs {
			for sceneIdxKey, notes := range sceneDefs {
				// Parse index from key (format: "__index_N")
				var sceneIdx int
				if _, err := fmt.Sscanf(sceneIdxKey, "__index_%d", &sceneIdx); err != nil {
					continue
				}
				scene, ok := sceneMap[sceneKey{bookID: bookID, event: eventTitle, index: sceneIdx}]
				if !ok {
					// Scene not found — skip
					continue
				}

				for _, note := range notes {
					if note.NotUsed {
						continue
					}
					expression := strings.TrimSpace(note.Expression)
					if expression == "" {
						continue
					}

					exprPattern := buildValidatePattern(expression)
					found := false
					for _, conv := range scene.Conversations {
						if exprPattern.MatchString(conv.Quote) {
							found = true
							break
						}
					}
					if !found {
						for _, stmt := range scene.Statements {
							if exprPattern.MatchString(stmt) {
								found = true
								break
							}
						}
					}

					if !found {
						result.AddError("consistency", ValidationError{
							Location: fmt.Sprintf("%s -> scene[%d] -> %s", eventTitle, sceneIdx, expression),
							Message:  fmt.Sprintf("expression %q from definitions file not found in any conversation or statement", expression),
							Suggestions: []string{
								"fix the expression to match the text in the conversation",
								"or mark it as not_used: true",
							},
						})
					}
				}
			}
		}
	}
}

// migrateEtymologyShape converts legacy etymology learning-history blocks
// (top-level metadata.title = notebook display name, type=etymology, with
// sessions as scenes) into the canonical per-session shape that
// standard/reverse/freeform writers also use:
//
//	BEFORE                                  AFTER
//	- metadata:                             - metadata:
//	    title: "Word Power Made Easy"           title: "Session 2"
//	    type: etymology                       scenes:
//	  scenes:                                   - metadata:
//	    - metadata:                                 title: "<scene>"
//	        title: "Session 2"                  expressions:
//	      expressions:                            - expression: "ana"
//	        - expression: "ana"                     etymology_breakdown_logs: [...]
//	          etymology_breakdown_logs: [...]
//
// For each origin in a legacy scene, the destination scene title comes
// from the matching definitions file: the validator builds the same
// origin → scene candidate map the Reader uses for legacy flat-shape
// source files. Same-notebook+session match wins; falls back to
// any-notebook globally; final fallback is the session title itself.
//
// When a per-session block already exists for the destination session
// (e.g. populated by standard quiz writes), the migrated origin merges
// into that block — same-name expressions consolidate their logs.
//
// Idempotent: running on already-migrated data is a no-op because no
// blocks carry type=etymology after the first pass.
func (v *Validator) migrateEtymologyShape(files []learningHistoryFile, result *ValidationResult) []learningHistoryFile {
	// Build the cross-notebook origin → scene candidate index using the
	// validator's own directory inputs. Reusing NewReader keeps the
	// projection logic identical to live read paths.
	candidates := v.buildOriginSceneIndexForValidator()

	for fileIdx := range files {
		file := &files[fileIdx]
		var legacy []LearningHistory
		var keep []LearningHistory
		for _, h := range file.contents {
			if h.Metadata.Type == "etymology" && len(h.Scenes) > 0 {
				legacy = append(legacy, h)
			} else {
				keep = append(keep, h)
			}
		}
		if len(legacy) == 0 {
			continue
		}

		for _, src := range legacy {
			notebookID := src.Metadata.NotebookID
			for _, legacyScene := range src.Scenes {
				sessionTitle := legacyScene.Metadata.Title
				targetIdx := -1
				for i := range keep {
					if normalizeQuotes(keep[i].Metadata.Title) == normalizeQuotes(sessionTitle) {
						targetIdx = i
						break
					}
				}
				if targetIdx < 0 {
					keep = append(keep, LearningHistory{
						Metadata: LearningHistoryMetadata{
							NotebookID: notebookID,
							Title:      sessionTitle,
						},
					})
					targetIdx = len(keep) - 1
				}

				for _, expr := range legacyScene.Expressions {
					// All entries pulled from a type=etymology block are
					// origins by definition; tag them so they don't merge
					// with vocab entries sharing the same name in the
					// destination scene.
					expr.Type = LearningExpressionTypeOrigin
					sceneTitle := pickBestSceneForOrigin(candidates, expr.Expression, notebookID, sessionTitle)
					if sceneTitle == "" {
						sceneTitle = sessionTitle
					}
					sceneIdx := -1
					for si := range keep[targetIdx].Scenes {
						if normalizeQuotes(keep[targetIdx].Scenes[si].Metadata.Title) == normalizeQuotes(sceneTitle) {
							sceneIdx = si
							break
						}
					}
					if sceneIdx < 0 {
						keep[targetIdx].Scenes = append(keep[targetIdx].Scenes, LearningScene{
							Metadata: LearningSceneMetadata{Title: sceneTitle},
						})
						sceneIdx = len(keep[targetIdx].Scenes) - 1
					}
					// Merge target: only an entry of the same type counts
					// as a duplicate. A type=origin and a type=vocabulary
					// entry with the same name coexist as separate records.
					mergeIdx := -1
					for ei := range keep[targetIdx].Scenes[sceneIdx].Expressions {
						existing := &keep[targetIdx].Scenes[sceneIdx].Expressions[ei]
						if existing.Expression != expr.Expression {
							continue
						}
						if existing.Type == LearningExpressionTypeOrigin {
							mergeIdx = ei
							break
						}
					}
					if mergeIdx < 0 {
						keep[targetIdx].Scenes[sceneIdx].Expressions = append(keep[targetIdx].Scenes[sceneIdx].Expressions, expr)
						continue
					}
					existing := &keep[targetIdx].Scenes[sceneIdx].Expressions[mergeIdx]
					existing.LearnedLogs = append(existing.LearnedLogs, expr.LearnedLogs...)
					existing.ReverseLogs = append(existing.ReverseLogs, expr.ReverseLogs...)
					existing.EtymologyBreakdownLogs = append(existing.EtymologyBreakdownLogs, expr.EtymologyBreakdownLogs...)
					existing.EtymologyAssemblyLogs = append(existing.EtymologyAssemblyLogs, expr.EtymologyAssemblyLogs...)
					for k, val := range expr.SkippedAt {
						if existing.SkippedAt == nil {
							existing.SkippedAt = make(SkippedAtMap)
						}
						if _, dup := existing.SkippedAt[k]; !dup {
							existing.SkippedAt[k] = val
						}
					}
				}
			}
			result.AddWarning(ValidationError{
				File:    file.path,
				Message: fmt.Sprintf("Migrated etymology shape for notebook %q (legacy title=%q): %d session(s) folded into per-session top-level blocks", src.Metadata.NotebookID, src.Metadata.Title, len(src.Scenes)),
			})
		}
		file.contents = keep
	}
	return files
}

// buildOriginSceneIndexForValidator constructs the same origin → scene
// candidate map the Reader produces, but using the validator's own
// directory inputs. Building a full Reader risks importing dictionary
// state we don't need; instead we walk the dirs directly and reuse the
// same per-source projection logic.
func (v *Validator) buildOriginSceneIndexForValidator() map[string][]OriginSceneCandidate {
	add := func(out map[string][]OriginSceneCandidate, origin, notebookID, sessionTitle, sceneTitle string) {
		key := strings.ToLower(strings.TrimSpace(origin))
		if key == "" {
			return
		}
		out[key] = append(out[key], OriginSceneCandidate{
			NotebookID:   notebookID,
			SessionTitle: sessionTitle,
			SceneTitle:   sceneTitle,
		})
	}
	out := make(map[string][]OriginSceneCandidate)

	storyIndexes := make(map[string]Index)
	for _, dir := range v.storyNotebooksDirs {
		_ = walkIndexFiles(dir, storyIndexes, false)
	}
	for storyID, idx := range storyIndexes {
		for _, nbPath := range idx.NotebookPaths {
			path := filepath.Join(idx.Path, nbPath)
			notebooks, err := readYamlFile[[]StoryNotebook](path)
			if err != nil {
				continue
			}
			for _, nb := range notebooks {
				for _, scene := range nb.Scenes {
					for _, def := range scene.Definitions {
						for _, op := range def.OriginParts {
							add(out, op.Origin, storyID, nb.Event, scene.Title)
						}
					}
				}
			}
		}
	}

	defsMap, defsRaw, _, err := NewDefinitionsMap(v.definitionsDirs)
	if err == nil {
		_ = defsMap
		for bookID, defs := range defsRaw {
			for _, fileDefs := range defs {
				session := fileDefs.Metadata.Title
				for _, scene := range fileDefs.Scenes {
					sceneTitle := scene.Metadata.Title
					for _, note := range scene.Expressions {
						for _, op := range note.OriginParts {
							add(out, op.Origin, bookID, session, sceneTitle)
						}
					}
				}
			}
		}
	}

	etymIndexes := make(map[string]EtymologyIndex)
	for _, dir := range v.etymologyDirs {
		_ = walkEtymologyIndexFiles(dir, etymIndexes)
	}
	for etymID, idx := range etymIndexes {
		for _, nbPath := range idx.NotebookPaths {
			path := filepath.Join(idx.Path, nbPath)
			wrapped, err := readYamlFile[etymologySessionFile](path)
			if err != nil {
				continue
			}
			session := wrapped.Metadata.Title
			for _, def := range wrapped.Definitions {
				for _, op := range def.OriginParts {
					add(out, op.Origin, etymID, session, session)
				}
			}
		}
	}

	return out
}

// migrateLegacySkipIntervals promotes the pre-Phase-2 skip representation
// (a single log with interval_days >= 3650 and no override_interval) into
// the per-quiz-type SkippedAtMap. The log's interval_days is then reset to
// zero so the SR-recalculate pass downstream computes the natural value.
//
// This runs unconditionally on every Fix() because it's a no-op once data
// is migrated. The threshold of 3650 is the magic value SkipWord used
// before the per-type rewrite — natural SM-2 intervals stay below ~1100
// days for typical study histories, so 3650 unambiguously means "skip".
func (v *Validator) migrateLegacySkipIntervals(files []learningHistoryFile, result *ValidationResult) []learningHistoryFile {
	const legacySkipThreshold = 3650

	promote := func(expr *LearningHistoryExpression, logs []LearningRecord, fileLabel, exprLabel string) []LearningRecord {
		if len(logs) == 0 {
			return logs
		}
		latest := &logs[0]
		if latest.IntervalDays < legacySkipThreshold || latest.OverrideInterval != 0 {
			return logs
		}
		quizType := QuizType(latest.QuizType)
		if quizType == "" {
			// Without a quiz_type we can't pick a slot; leave the
			// record alone and let backfillQuizType run first.
			return logs
		}
		at := latest.LearnedAt.Format("2006-01-02T15:04:05Z07:00")
		expr.SkippedAt = expr.SkippedAt.Set(quizType, at)
		latest.IntervalDays = 0
		result.AddWarning(ValidationError{
			File:    fileLabel,
			Message: fmt.Sprintf("Migrated legacy skip (interval_days=3650) for %q to skipped_at[%s]", exprLabel, quizType),
		})
		return logs
	}

	for fileIdx := range files {
		file := &files[fileIdx]
		for histIdx := range file.contents {
			history := &file.contents[histIdx]

			migrate := func(expr *LearningHistoryExpression) {
				expr.LearnedLogs = promote(expr, expr.LearnedLogs, file.path, expr.Expression)
				expr.ReverseLogs = promote(expr, expr.ReverseLogs, file.path, expr.Expression)
				expr.EtymologyBreakdownLogs = promote(expr, expr.EtymologyBreakdownLogs, file.path, expr.Expression)
				expr.EtymologyAssemblyLogs = promote(expr, expr.EtymologyAssemblyLogs, file.path, expr.Expression)
			}

			for ei := range history.Expressions {
				migrate(&history.Expressions[ei])
			}
			for si := range history.Scenes {
				for ei := range history.Scenes[si].Expressions {
					migrate(&history.Scenes[si].Expressions[ei])
				}
			}
		}
	}
	return files
}

// recalculateLearningLogs replays a log list through the configured SR
// calculator so interval_days reflects the prior-state chain (each log's
// interval feeds into the next via the calculator's RecalculateAll, which
// applies the early-review guard so a correct answer that came before the
// previous interval elapsed doesn't shrink the interval). The function
// also fixes the misunderstood+quality≥3 inconsistency and sorts
// newest-first to match storage convention. `changed` reports whether
// any interval_days or quality value differed from the input so callers
// can emit a warning only on actual drift.
//
// The first return value is unused by current callers but kept for ABI
// compatibility; it returns DefaultEasinessFactor as a stable placeholder.
func recalculateLearningLogs(logs []LearningRecord, calculator IntervalCalculator) (float64, []LearningRecord, bool) {
	if len(logs) == 0 {
		return DefaultEasinessFactor, logs, false
	}

	oldIntervals := make([]int, len(logs))
	oldQualities := make([]int, len(logs))
	oldKey := make(map[time.Time]int, len(logs))
	for i, log := range logs {
		oldIntervals[i] = log.IntervalDays
		oldQualities[i] = log.Quality
		oldKey[log.LearnedAt.Time] = i
	}

	if calculator == nil {
		calculator = &SM2Calculator{}
	}
	_, recalc := calculator.RecalculateAll(logs)

	changed := false
	for _, log := range recalc {
		oldIdx, ok := oldKey[log.LearnedAt.Time]
		if !ok {
			changed = true
			continue
		}
		if log.IntervalDays != oldIntervals[oldIdx] || log.Quality != oldQualities[oldIdx] {
			changed = true
			break
		}
	}
	return DefaultEasinessFactor, recalc, changed
}

// validateFlashcardNotebooks validates all flashcard notebook files
func (v *Validator) validateFlashcardNotebooks(files []flashcardNotebookFile, result *ValidationResult) {
	for _, file := range files {
		for notebookIdx, notebook := range file.contents {
			notebookLocation := fmt.Sprintf("notebook[%d]: %s", notebookIdx, notebook.Title)

			// Call the Validate method on the FlashcardNotebook object
			errors := notebook.Validate(notebookLocation)
			for _, err := range errors {
				err.File = file.path
				result.AddError("learning_notes", err)
			}
		}
	}
}
