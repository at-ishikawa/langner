package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	dictionaryDir      string
}

// NewValidator creates a new validator
func NewValidator(learningNotesDir string, storyNotebooksDirs []string, flashcardsDirs []string, dictionaryDir string) *Validator {
	return &Validator{
		learningNotesDir:   learningNotesDir,
		storyNotebooksDirs: storyNotebooksDirs,
		flashcardsDirs:     flashcardsDirs,
		dictionaryDir:      dictionaryDir,
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

	// Validate learning notes structure
	v.validateLearningNotesStructure(learningHistories, result)

	// Validate cross-notebook consistency
	v.validateConsistency(learningHistories, storyNotebooks, result)

	// Validate dictionary references
	v.validateDictionaryReferences(storyNotebooks, result)

	// Validate definitions appear in conversations
	v.validateDefinitionsInConversations(storyNotebooks, result)

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

	// Fix mismatched scenes by moving expressions to correct scenes (do this first)
	fixedLearning := v.fixMismatchedScenes(learningHistories, storyNotebooks, result)

	// Fix expression names to match story definitions (do this before merging duplicates)
	fixedLearning = v.fixExpressionNames(fixedLearning, storyNotebooks, result)

	// Fix learning notes structure issues (including duplicate merging - do this after renaming)
	fixedLearning = v.fixLearningNotesStructure(fixedLearning, result)

	// Fix cross-notebook consistency issues
	fixedLearning = v.fixConsistency(fixedLearning, storyNotebooks, result)

	// Create missing learning note entries
	fixedLearning = v.createMissingLearningNotes(fixedLearning, storyNotebooks, result)

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
		files, err := loadYamlFiles[[]StoryNotebook](dir, func(path string, info os.FileInfo) bool {
			return !info.IsDir() && filepath.Ext(path) == ".yml" && filepath.Base(path) != "index.yml"
		})
		if err != nil {
			return nil, fmt.Errorf("loadYamlFiles(%s) > %w", dir, err)
		}
		allFiles = append(allFiles, files...)
	}
	return allFiles, nil
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

					// Check if expression exists in story notebooks
					locations, found := storyExpressions[expression]
					if !found {
						result.AddError("consistency", ValidationError{
							File:     file.path,
							Location: exprLocation,
							Message:  fmt.Sprintf("orphaned learning note: expression %q not found in any story notebook", expression),
							Suggestions: []string{
								"remove this learning note or add the expression to a story notebook",
							},
						})
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
	for _, file := range files {
		for histIdx := range file.contents {
			for sceneIdx := range file.contents[histIdx].Scenes {
				scene := &file.contents[histIdx].Scenes[sceneIdx]

				// First, merge duplicate expressions in the same scene
				expressionMap := make(map[string]int) // map expression to index in mergedExpressions
				var mergedExpressions []LearningHistoryExpression

				for _, expr := range scene.Expressions {
					exprKey := strings.TrimSpace(expr.Expression)
					if exprKey == "" {
						continue
					}

					if existingIdx, found := expressionMap[exprKey]; found {
						// Merge learning logs into the existing expression
						mergedExpressions[existingIdx].LearnedLogs = append(mergedExpressions[existingIdx].LearnedLogs, expr.LearnedLogs...)
						result.AddWarning(ValidationError{
							File:    file.path,
							Message: fmt.Sprintf("Merged duplicate expression %q in scene %s::%s", exprKey, file.contents[histIdx].Metadata.Title, scene.Metadata.Title),
						})
					} else {
						expressionMap[exprKey] = len(mergedExpressions)
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

			// Then, merge duplicate expressions across different scenes in the same episode
			episodeExpressions := make(map[string]int) // expression -> scene index
			for sceneIdx := range file.contents[histIdx].Scenes {
				scene := &file.contents[histIdx].Scenes[sceneIdx]

				// Track expressions and find duplicates
				expressionsToRemove := make(map[int]bool) // indices to remove
				for exprIdx := range scene.Expressions {
					expr := &scene.Expressions[exprIdx]
					exprKey := strings.TrimSpace(expr.Expression)
					if exprKey == "" {
						continue
					}

					if firstSceneIdx, found := episodeExpressions[exprKey]; found {
						// This expression already exists in another scene - merge into the first occurrence
						firstScene := &file.contents[histIdx].Scenes[firstSceneIdx]

						// Find the first occurrence expression in the first scene
						for firstExprIdx := range firstScene.Expressions {
							if strings.TrimSpace(firstScene.Expressions[firstExprIdx].Expression) == exprKey {
								// Merge learning logs
								firstScene.Expressions[firstExprIdx].LearnedLogs = append(
									firstScene.Expressions[firstExprIdx].LearnedLogs,
									expr.LearnedLogs...,
								)

								// Mark this duplicate for removal
								expressionsToRemove[exprIdx] = true

								result.AddWarning(ValidationError{
									File:    file.path,
									Message: fmt.Sprintf("Merged duplicate expression %q from scene %s into scene %s in episode %s",
										exprKey,
										scene.Metadata.Title,
										firstScene.Metadata.Title,
										file.contents[histIdx].Metadata.Title),
								})
								break
							}
						}
					} else {
						// First occurrence of this expression in the episode
						episodeExpressions[exprKey] = sceneIdx
					}
				}

				// Remove duplicates from this scene
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
					// 1. Have learned_logs, OR
					// 2. Exist in the story (even with empty learned_logs)
					if len(expr.LearnedLogs) > 0 || existsInStory {
						validExpressions = append(validExpressions, expr)
					} else {
						// Remove orphaned expressions with no learned_logs
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
