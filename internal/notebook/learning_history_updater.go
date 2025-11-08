package notebook

// LearningHistoryUpdater provides methods to update learning history
type LearningHistoryUpdater struct {
	history []LearningHistory
}

// NewLearningHistoryUpdater creates a new updater with the given history
func NewLearningHistoryUpdater(history []LearningHistory) *LearningHistoryUpdater {
	return &LearningHistoryUpdater{
		history: history,
	}
}

// updateOrCreateExpression is the internal implementation
func (u *LearningHistoryUpdater) UpdateOrCreateExpression(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord, alwaysRecord bool,
) bool {
	// First, try to find and update existing expression
	for hi, h := range u.history {
		if h.Metadata.Title != storyTitle {
			continue
		}

		for si, s := range h.Scenes {
			if s.Metadata.Title != sceneTitle {
				continue
			}

			for ei, exp := range s.Expressions {
				if exp.Expression != expression {
					continue
				}

				// Found existing expression - update it
				if alwaysRecord {
					exp.AddRecordAlways(isCorrect, isKnownWord)
				} else {
					exp.AddRecord(isCorrect, isKnownWord)
				}
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	// Expression not found - create new entry
	u.createNewExpression(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, alwaysRecord)
	return false
}

// createNewExpression creates a new expression entry in the learning history
func (u *LearningHistoryUpdater) createNewExpression(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord, alwaysRecord bool,
) {
	// Find or create the story entry
	storyIndex := u.findOrCreateStory(notebookID, storyTitle)

	// Find or create the scene entry
	sceneIndex := u.findOrCreateScene(storyIndex, sceneTitle)

	// Create new expression entry
	newExpression := LearningHistoryExpression{
		Expression:  expression,
		LearnedLogs: []LearningRecord{},
	}
	if alwaysRecord {
		newExpression.AddRecordAlways(isCorrect, isKnownWord)
	} else {
		newExpression.AddRecord(isCorrect, isKnownWord)
	}

	// Only add the expression if it has at least one learning record
	if len(newExpression.LearnedLogs) > 0 {
		u.history[storyIndex].Scenes[sceneIndex].Expressions = append(
			u.history[storyIndex].Scenes[sceneIndex].Expressions,
			newExpression,
		)
	}
}

// findOrCreateStory finds an existing story or creates a new one
func (u *LearningHistoryUpdater) findOrCreateStory(notebookID, storyTitle string) int {
	// Try to find existing story
	for i, h := range u.history {
		if h.Metadata.Title == storyTitle {
			return i
		}
	}

	// Create new story
	newStory := LearningHistory{
		Metadata: LearningHistoryMetadata{
			NotebookID: notebookID,
			Title:      storyTitle,
		},
		Scenes: []LearningScene{},
	}
	u.history = append(u.history, newStory)
	return len(u.history) - 1
}

// findOrCreateScene finds an existing scene or creates a new one
func (u *LearningHistoryUpdater) findOrCreateScene(storyIndex int, sceneTitle string) int {
	// Try to find existing scene
	for i, s := range u.history[storyIndex].Scenes {
		if s.Metadata.Title == sceneTitle {
			return i
		}
	}

	// Create new scene
	newScene := LearningScene{
		Metadata: LearningSceneMetadata{
			Title: sceneTitle,
		},
		Expressions: []LearningHistoryExpression{},
	}
	u.history[storyIndex].Scenes = append(u.history[storyIndex].Scenes, newScene)
	return len(u.history[storyIndex].Scenes) - 1
}

// GetHistory returns the updated history
func (u *LearningHistoryUpdater) GetHistory() []LearningHistory {
	return u.history
}
