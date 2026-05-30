package quiz

import (
	"time"

	"github.com/at-ishikawa/langner/internal/inference"
)

// WordOriginPart holds a resolved etymology origin with full details.
type WordOriginPart struct {
	Origin   string
	Type     string
	Language string
	Meaning  string
}

// WordDetail holds rich metadata about a word.
type WordDetail struct {
	Origin        string
	Pronunciation string
	PartOfSpeech  string
	Synonyms      []string
	Antonyms      []string
	Memo          string
	OriginParts   []WordOriginPart
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
	Images        []string

	// ConceptHead names the head expression of the definitions concept this
	// card belongs to, or "" when the card isn't a concept member. When set,
	// ConceptMembers lists all member expressions (head included) in YAML
	// declaration order, and ConceptMeaning carries the concept's umbrella
	// meaning — used as the grader target in standard quizzes.
	ConceptHead    string
	ConceptMembers []string
	ConceptMeaning string
}

// Example is a usage sentence for a card.
type Example struct {
	Text    string
	Speaker string // empty for flashcards
}

// NotebookSummary holds display info for one notebook.
type NotebookSummary struct {
	NotebookID                  string
	Name                        string
	ReviewCount                 int
	ReverseReviewCount          int
	EtymologyReviewCount        int
	EtymologyReverseReviewCount int
	LatestDate                  time.Time
	Kind                        string
	// HasContent is true when any scene in the notebook has statements or
	// conversations — i.e., there is prose/dialogue to read, not just flashcards.
	HasContent bool
	// Sections lists the per-section summaries (story events for vocabulary,
	// session titles for etymology) in document order. Empty for flashcards
	// and definitions-only books that have no section hierarchy.
	Sections []NotebookSectionSummary
}

// NotebookSectionSummary describes a single section within a notebook with
// per-mode review counts so the start screen can show counts both per
// section and per-notebook.
type NotebookSectionSummary struct {
	Title                       string
	ReviewCount                 int
	ReverseReviewCount          int
	EtymologyReviewCount        int
	EtymologyReverseReviewCount int
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
