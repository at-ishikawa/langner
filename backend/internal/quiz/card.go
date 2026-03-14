package quiz

import (
	"time"

	"github.com/at-ishikawa/langner/internal/inference"
)

// WordDetail holds rich metadata about a word.
type WordDetail struct {
	Origin        string
	Pronunciation string
	PartOfSpeech  string
	Synonyms      []string
	Antonyms      []string
	Memo          string
}

// Card represents a single quiz card shared between CLI and RPC.
type Card struct {
	NotebookName  string
	StoryTitle    string
	SceneTitle    string
	Entry         string // expression or definition form shown to user (Note.Definition if set, else Note.Expression)
	OriginalEntry string // original expression form (Note.Expression); empty if same as Entry
	Meaning       string
	Examples      []Example
	Contexts      []inference.Context
	WordDetail    WordDetail
}

// Example is a usage sentence for a card.
type Example struct {
	Text    string
	Speaker string // empty for flashcards
}

// NotebookSummary holds display info for one notebook.
type NotebookSummary struct {
	NotebookID         string
	Name               string
	ReviewCount        int
	ReverseReviewCount int
	LatestStoryDate    time.Time
	Kind               string
}

// GradeResult holds the outcome of grading a user's answer.
type GradeResult struct {
	Correct        bool
	Reason         string
	Quality        int
	Classification string // inference classification (e.g., "same_word", "synonym", "wrong")
}

// NotFoundError is returned when a requested notebook does not exist.
type NotFoundError struct {
	NotebookID string
}

func (e *NotFoundError) Error() string {
	return "notebook " + e.NotebookID + " not found"
}
