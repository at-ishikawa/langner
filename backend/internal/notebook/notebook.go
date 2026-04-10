package notebook

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"gopkg.in/yaml.v3"
)

type Source string

const (
	SourceTVShow Source = "TV Show"
)

type Notebook struct {
	Series     string `yaml:"series,omitempty"`
	YouTubeURL string `yaml:"youtube_url,omitempty"`

	Source  Source    `yaml:"source,omitempty"`
	Season  int       `yaml:"season,omitempty"`
	Episode int       `yaml:"episode,omitempty"`
	Notes   []Note    `yaml:"notes,omitempty"`
	Date    time.Time `yaml:"date,omitempty"`
}

type LearnedStatus string

const (
	LearnedStatusLearning        LearnedStatus = ""
	LearnedStatusMisunderstood   LearnedStatus = "misunderstood"
	LearnedStatusUnderstood      LearnedStatus = "understood"
	LearnedStatusCanBeUsed       LearnedStatus = "usable"
	learnedStatusIntuitivelyUsed LearnedStatus = "intuitive"
)

type ExpressionLevel string

const (
	ExpressionLevelNew      ExpressionLevel = ""
	ExpressionLevelUnusable ExpressionLevel = "unusable"
)

type Note struct {
	notebookDate time.Time `yaml:"-"`

	ID         uint   `yaml:"id,omitempty"`
	Expression string `yaml:"expression,omitempty"`
	Definition string `yaml:"definition,omitempty"`

	// Either of them is required.
	Level    ExpressionLevel `yaml:"level,omitempty"`
	Meaning  string          `yaml:"meaning,omitempty"`
	Examples []string        `yaml:"examples,omitempty"`
	Images   []string        `yaml:"images,omitempty"`

	Origin        string   `yaml:"origin,omitempty"`
	Synonyms      []string `yaml:"synonyms,omitempty"`
	Antonyms      []string `yaml:"antonyms,omitempty"`
	PartOfSpeech  string   `yaml:"part_of_speech,omitempty"`
	Pronunciation string   `yaml:"pronunciation,omitempty"`
	Memo          string   `yaml:"memo,omitempty"`
	Note          string   `yaml:"note,omitempty"`

	// Deprecated: Use References
	Reference string `yaml:"reference,omitempty"`

	OriginParts []OriginPartRef `yaml:"origin_parts,omitempty"`
	Statements  []Phrase        `yaml:"statements,omitempty"`

	YouTubeTimeSeconds int         `yaml:"youtube_time_seconds,omitempty"`
	References         []Reference `yaml:"references,omitempty"`
	Links              []string    `yaml:"links,omitempty"` // what's this?

	// the index of a dictionary result + 1
	DictionaryNumber int `yaml:"dictionary_number,omitempty"`

	// Deprecated: This was moved to LearningHistory
	LearnedLogs []LearningRecord `yaml:"learned_logs,omitempty"`
	// ReverseLogs is hydrated by FilterStoryNotebooks from the learning
	// history's ReverseLogs track. Not serialised to YAML.
	ReverseLogs []LearningRecord `yaml:"-"`
	NotUsed     bool             `yaml:"not_used,omitempty"`

	// only for template rendering
	YoutubeURL string `yaml:",omitempty"`
}

// Date represents a timestamp for YAML serialization
// Stored as RFC3339 format to preserve timezone information
type Date struct {
	time.Time
}

func (d Date) MarshalYAML() (interface{}, error) {
	return d.Format(time.RFC3339), nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface
func (d *Date) UnmarshalYAML(value *yaml.Node) error {
	// First try the new YYYY-MM-DD format
	t, err := time.Parse("2006-01-02", value.Value)
	if err == nil {
		d.Time = t
		return nil
	}

	// If that fails, try the old RFC3339 timestamp format
	t, err = time.Parse(time.RFC3339, value.Value)
	if err == nil {
		d.Time = t
		return nil
	}

	// If that fails, try RFC3339Nano format (with nanoseconds)
	t, err = time.Parse(time.RFC3339Nano, value.Value)
	if err == nil {
		d.Time = t
		return nil
	}

	// If all formats fail, return the original error
	return fmt.Errorf("unable to parse date '%s': expected YYYY-MM-DD, RFC3339, or RFC3339Nano format", value.Value)
}

// NewDate creates a new Date from the current time or a provided time
func NewDate(t ...time.Time) Date {
	if len(t) > 0 {
		return Date{Time: t[0]}
	}
	return Date{Time: time.Now()}
}

func (note Note) getLearnScore() int {
	score := 0
	for _, learnedLog := range note.LearnedLogs {
		switch learnedLog.Status {
		case LearnedStatusLearning:
		case LearnedStatusMisunderstood:
			// Misunderstood has negative impact on score
			score -= 5
		case LearnedStatusUnderstood:
			score += 10
		case LearnedStatusCanBeUsed:
			score += 1_000
		case learnedStatusIntuitivelyUsed:
			score += 100_000
		}
	}

	current := time.Now()
	days := current.Sub(note.lastLearnedAt()).Hours() / 24
	notebookDays := current.Sub(note.notebookDate).Hours() / 24

	// prioritize
	// 1. a word which is old words
	// 1. a word which is learned very before
	return score - int(days) - int(notebookDays)
}

func (note Note) lastLearnedAt() time.Time {
	if len(note.LearnedLogs) == 0 {
		return time.Time{}
	}
	return note.LearnedLogs[0].LearnedAt.Time
}

func (note *Note) SetDetails(dictionaryMap map[string]rapidapi.Response, youTubeURL string) error {
	def := note.Definition
	if def == "" {
		def = note.Expression
	}

	word, ok := dictionaryMap[def]
	if ok && note.DictionaryNumber > 0 {
		if note.DictionaryNumber > len(word.Results) {
			return fmt.Errorf("dictionary number %d is out of range for word: %+v", note.DictionaryNumber, note)
		}

		// copy definition from a dictionary
		definition := word.Results[note.DictionaryNumber-1]

		note.PartOfSpeech = definition.PartOfSpeech
		note.Pronunciation = word.Pronunciation.All
		note.Meaning = definition.Definition
		note.Synonyms = definition.Synonyms
		if len(note.Examples) == 0 {
			note.Examples = definition.Examples
		}
		if note.YouTubeTimeSeconds > 0 {
			note.YoutubeURL = fmt.Sprintf("%s?t=%d", youTubeURL, note.YouTubeTimeSeconds)
		}
	} else if len(note.Statements) == 0 {
		if note.Level == ExpressionLevelNew && note.Meaning == "" && len(note.Images) == 0 && len(note.Synonyms) == 0 {
			return fmt.Errorf("there is no meaning, images, nor statements for word: %+v", note)
		}
	}
	return nil
}

type Reference struct {
	URL         string `yaml:"url,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// OriginPartRef references an etymology origin by origin name and language.
type OriginPartRef struct {
	Origin   string `yaml:"origin"`
	Language string `yaml:"language,omitempty"`
}

type Phrase struct {
	Actor   string `yaml:"actor,omitempty"`
	Remarks string `yaml:"remarks"`
}

func (note *Note) needsToLearn() bool {
	if len(note.LearnedLogs) == 0 {
		return true
	}
	sort.Slice(note.LearnedLogs, func(i, j int) bool {
		return note.LearnedLogs[i].LearnedAt.After(note.LearnedLogs[j].LearnedAt.Time)
	})
	lastLearnedResult := note.LearnedLogs[0]

	// Always include misunderstood expressions for review
	if lastLearnedResult.Status == LearnedStatusMisunderstood {
		return true
	}

	// Use stored interval if available, otherwise use legacy calculation
	threshold := lastLearnedResult.IntervalDays
	if threshold == 0 {
		threshold = note.getNextLearningThresholdDays()
	}
	now := time.Now()
	return now.After(lastLearnedResult.LearnedAt.Add(time.Duration(threshold) * time.Hour * 24))
}

// needsToLearnInNotebook returns true if the note should be shown in notebook
// output / PDF. A word is included if ANY quiz track needs attention:
//   - Forward track: no correct answers yet, or latest is misunderstood
//   - Reverse track: latest is misunderstood (when ReverseLogs are populated)
func (note *Note) needsToLearnInNotebook() bool {
	// Forward track check
	forwardNeedsLearn := false
	if !note.hasAnyCorrectAnswer() {
		forwardNeedsLearn = true
	} else if len(note.LearnedLogs) > 0 {
		sort.Slice(note.LearnedLogs, func(i, j int) bool {
			return note.LearnedLogs[i].LearnedAt.After(note.LearnedLogs[j].LearnedAt.Time)
		})
		forwardNeedsLearn = note.LearnedLogs[0].Status == LearnedStatusMisunderstood
	}

	if forwardNeedsLearn {
		return true
	}

	// Reverse track check (only when logs are populated by the caller)
	if len(note.ReverseLogs) > 0 {
		sort.Slice(note.ReverseLogs, func(i, j int) bool {
			return note.ReverseLogs[i].LearnedAt.After(note.ReverseLogs[j].LearnedAt.Time)
		})
		if note.ReverseLogs[0].Status == LearnedStatusMisunderstood {
			return true
		}
	}

	return false
}

func (note Note) hasAnyCorrectAnswer() bool {
	if len(note.LearnedLogs) == 0 {
		return false
	}

	for _, log := range note.LearnedLogs {
		if log.Status == LearnedStatusUnderstood ||
			log.Status == LearnedStatusCanBeUsed ||
			log.Status == learnedStatusIntuitivelyUsed {
			return true
		}
	}

	return false
}

// hasFreeformAnswer returns true if any LearnedLog entry was recorded by the
// freeform quiz. Vocabulary words must be answered in freeform mode first before
// becoming eligible for standard or reverse quizzes.
func (note Note) hasFreeformAnswer() bool {
	for _, log := range note.LearnedLogs {
		if log.QuizType == string(QuizTypeFreeform) {
			return true
		}
	}
	return false
}

// GetThresholdDaysFromCount returns the number of days until next review
// based on the number of correct answers. This implements the spaced repetition
// algorithm used across all quiz types.
func GetThresholdDaysFromCount(count int) int {
	thresholds := map[int]int{
		1:  3,
		2:  7,
		3:  14,
		4:  30,
		5:  60,
		6:  90,
		7:  180,
		8:  270,
		9:  365,
		10: 540,
		11: 730,
		12: 1095,
	}
	threshold, exists := thresholds[count]
	if exists {
		return threshold
	}
	if count > 12 {
		return math.MaxInt
	}
	return 0
}

func (note Note) getNextLearningThresholdDays() int {
	learnedLogs := note.LearnedLogs

	count := 0
	for _, learnedLog := range learnedLogs {
		if learnedLog.Status == LearnedStatusLearning || learnedLog.Status == LearnedStatusMisunderstood {
			continue
		}
		count++
	}

	return GetThresholdDaysFromCount(count)
}

type Template struct {
	Notebooks []Notebook
}
