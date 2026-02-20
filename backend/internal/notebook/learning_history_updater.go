package notebook

import "strings"

// normalizeTitle normalizes a title for comparison by trimming whitespace
// and normalizing internal whitespace (newlines, multiple spaces -> single space)
func normalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join(strings.Fields(s), " ")
}

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

// findOrCreateStory finds an existing story or creates a new one
func (u *LearningHistoryUpdater) findOrCreateStory(notebookID, storyTitle string, isFlashcard bool) int {
	for i, h := range u.history {
		if h.Metadata.Title == storyTitle {
			return i
		}
	}

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
	for i, s := range u.history[storyIndex].Scenes {
		if s.Metadata.Title == sceneTitle {
			return i
		}
	}

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

	for hi, h := range u.history {
		if h.Metadata.Title != storyTitle {
			continue
		}

		if isFlashcard || h.Metadata.Type == "flashcard" {
			for ei, exp := range h.Expressions {
				if exp.Expression != expression {
					continue
				}
				exp.AddRecordWithQuality(isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Expressions[ei] = exp
				return true
			}
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
				exp.AddRecordWithQuality(isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	u.createNewExpressionWithQuality(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
	return false
}

// UpdateOrCreateExpressionWithQualityForReverse updates or creates an expression with SM-2 quality assessment for reverse quiz
func (u *LearningHistoryUpdater) UpdateOrCreateExpressionWithQualityForReverse(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
) bool {
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""
	normalizedSceneTitle := normalizeTitle(sceneTitle)

	for hi, h := range u.history {
		if h.Metadata.Title != storyTitle {
			continue
		}

		if isFlashcard || h.Metadata.Type == "flashcard" {
			for ei, exp := range h.Expressions {
				if exp.Expression != expression {
					continue
				}
				exp.AddRecordWithQualityForReverse(isCorrect, isKnownWord, quality, responseTimeMs)
				u.history[hi].Expressions[ei] = exp
				return true
			}
			continue
		}

		for si, s := range h.Scenes {
			if normalizeTitle(s.Metadata.Title) != normalizedSceneTitle {
				continue
			}

			for ei, exp := range s.Expressions {
				if exp.Expression != expression {
					continue
				}
				exp.AddRecordWithQualityForReverse(isCorrect, isKnownWord, quality, responseTimeMs)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	u.createNewExpressionWithQualityForReverse(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, quality, responseTimeMs)
	return false
}

// createNewExpressionWithQualityForReverse creates a new expression entry with quality data for reverse quiz
func (u *LearningHistoryUpdater) createNewExpressionWithQualityForReverse(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
) {
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, isFlashcard)

	newExpression := LearningHistoryExpression{
		Expression:            expression,
		LearnedLogs:           []LearningRecord{},
		EasinessFactor:        DefaultEasinessFactor,
		ReverseLogs:           []LearningRecord{},
		ReverseEasinessFactor: DefaultEasinessFactor,
	}
	newExpression.AddRecordWithQualityForReverse(isCorrect, isKnownWord, quality, responseTimeMs)

	if len(newExpression.ReverseLogs) == 0 {
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
