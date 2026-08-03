package notebook

// QuizType represents the type of quiz used for a learning session
type QuizType string

const (
	QuizTypeFreeform QuizType = "freeform"
	QuizTypeNotebook QuizType = "notebook"
	QuizTypeReverse  QuizType = "reverse"
	// QuizTypeEtymologyOrigin is the single etymology quiz mode: one screen
	// per (origin, sense) that shows the origin together with the whole
	// session-scoped family of derived words, and asks the user to type each
	// word's meaning. It replaces the former breakdown/assembly/freeform
	// etymology modes; the origin carries exactly one learning-log series.
	QuizTypeEtymologyOrigin QuizType = "etymology_origin"
	QuizTypeGrammar         QuizType = "grammar"
)

// Quality represents the quality of a response in the SM-2 algorithm
type Quality int

const (
	QualityWrong        Quality = 1 // Incorrect answer
	QualityCorrectSlow  Quality = 3 // Correct but slow (struggled)
	QualityCorrect      Quality = 4 // Correct at normal speed
	QualityCorrectFast  Quality = 5 // Correct and fast (instant recall)
)
