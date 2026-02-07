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
	isCorrect, isKnownWord bool,
) bool {
	// Determine if this is flashcard format based on story and scene titles
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""

	// First, try to find and update existing expression
	for hi, h := range u.history {
		if h.Metadata.Title != storyTitle {
			continue
		}

		// For flashcard type, search in expressions directly
		if isFlashcard || h.Metadata.Type == "flashcard" {
			for ei, exp := range h.Expressions {
				if exp.Expression != expression {
					continue
				}

				// Found existing expression - update it
				exp.AddRecord(isCorrect, isKnownWord)
				u.history[hi].Expressions[ei] = exp
				return true
			}
			continue
		}

		// For story type, search in scenes
		for si, s := range h.Scenes {
			if s.Metadata.Title != sceneTitle {
				continue
			}

			for ei, exp := range s.Expressions {
				if exp.Expression != expression {
					continue
				}

				// Found existing expression - update it
				exp.AddRecord(isCorrect, isKnownWord)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	// Expression not found - create new entry
	u.createNewExpression(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord)
	return false
}

// createNewExpression creates a new expression entry in the learning history
func (u *LearningHistoryUpdater) createNewExpression(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
) {
	// Determine if this is flashcard format
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""

	// Find or create the story entry
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, isFlashcard)

	// Create new expression entry
	newExpression := LearningHistoryExpression{
		Expression:  expression,
		LearnedLogs: []LearningRecord{},
	}
	newExpression.AddRecord(isCorrect, isKnownWord)

	// Only add the expression if it has at least one learning record
	if len(newExpression.LearnedLogs) == 0 {
		return
	}

	// For flashcard type, add expression directly to the history
	if isFlashcard || u.history[storyIndex].Metadata.Type == "flashcard" {
		u.history[storyIndex].Expressions = append(
			u.history[storyIndex].Expressions,
			newExpression,
		)
		return
	}

	// For story type, add expression to a scene
	sceneIndex := u.findOrCreateScene(storyIndex, sceneTitle)
	u.history[storyIndex].Scenes[sceneIndex].Expressions = append(
		u.history[storyIndex].Scenes[sceneIndex].Expressions,
		newExpression,
	)
}

// findOrCreateStory finds an existing story or creates a new one
func (u *LearningHistoryUpdater) findOrCreateStory(notebookID, storyTitle string, isFlashcard bool) int {
	// Try to find existing story
	for i, h := range u.history {
		if h.Metadata.Title == storyTitle {
			return i
		}
	}

	// Create new story with appropriate type
	newStory := LearningHistory{
		Metadata: LearningHistoryMetadata{
			NotebookID: notebookID,
			Title:      storyTitle,
		},
	}

	if isFlashcard {
		newStory.Metadata.Type = "flashcard"
		newStory.Expressions = []LearningHistoryExpression{}
	} else {
		newStory.Scenes = []LearningScene{}
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

// UpdateOrCreateExpressionWithQuality updates or creates an expression with SM-2 quality assessment
func (u *LearningHistoryUpdater) UpdateOrCreateExpressionWithQuality(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) bool {
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""

	// Try to find existing expression
	for hi, h := range u.history {
		if h.Metadata.Title != storyTitle {
			continue
		}

		// For flashcard type, search in expressions directly
		if isFlashcard || h.Metadata.Type == "flashcard" {
			for ei, exp := range h.Expressions {
				if exp.Expression != expression {
					continue
				}
				// Found existing expression - update it
				exp.AddRecordWithQuality(isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Expressions[ei] = exp
				return true
			}
			continue
		}

		// For story type, search in scenes
		for si, s := range h.Scenes {
			if s.Metadata.Title != sceneTitle {
				continue
			}

			for ei, exp := range s.Expressions {
				if exp.Expression != expression {
					continue
				}
				// Found existing expression - update it
				exp.AddRecordWithQuality(isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	// Expression not found - create new entry
	u.createNewExpressionWithQuality(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
	return false
}

// createNewExpressionWithQuality creates a new expression entry with quality data
func (u *LearningHistoryUpdater) createNewExpressionWithQuality(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, isFlashcard)

	newExpression := LearningHistoryExpression{
		Expression:     expression,
		LearnedLogs:    []LearningRecord{},
		EasinessFactor: DefaultEasinessFactor,
	}
	newExpression.AddRecordWithQuality(isCorrect, isKnownWord, quality, responseTimeMs, quizType)

	if len(newExpression.LearnedLogs) == 0 {
		return
	}

	if isFlashcard || u.history[storyIndex].Metadata.Type == "flashcard" {
		u.history[storyIndex].Expressions = append(
			u.history[storyIndex].Expressions,
			newExpression,
		)
		return
	}

	sceneIndex := u.findOrCreateScene(storyIndex, sceneTitle)
	u.history[storyIndex].Scenes[sceneIndex].Expressions = append(
		u.history[storyIndex].Scenes[sceneIndex].Expressions,
		newExpression,
	)
}
