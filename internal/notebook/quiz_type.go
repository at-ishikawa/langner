package notebook

// QuizType represents the type of quiz used for a learning session
type QuizType string

const (
	QuizTypeFreeform QuizType = "freeform"
	QuizTypeNotebook QuizType = "notebook"
)

// Quality represents the quality of a response in the SM-2 algorithm
type Quality int

const (
	QualityWrong        Quality = 1 // Incorrect answer
	QualityCorrectSlow  Quality = 3 // Correct but slow (struggled)
	QualityCorrect      Quality = 4 // Correct at normal speed
	QualityCorrectFast  Quality = 5 // Correct and fast (instant recall)
)
