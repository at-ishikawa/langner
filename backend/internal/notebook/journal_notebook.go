package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// JournalEntry is a single journal post: prose content only. Its grammar
// mistakes live in a separate corrections notebook, merged by ID — the same
// way a book's prose lives apart from its definitions.
type JournalEntry struct {
	ID    string    `yaml:"id"`
	Title string    `yaml:"title,omitempty"`
	Date  time.Time `yaml:"date,omitempty"`
	Text  string    `yaml:"text"`
}

// Correction is one grammar fix on a span of a journal post's text: the
// incorrect span (an exact substring of the post, so the quiz can highlight
// it), its fix, a free-form category, and the reason. Line is the 1-based line
// number in the drafted post, used for display and to disambiguate a span that
// appears more than once.
type Correction struct {
	// ID is the stable spaced-repetition identity. When empty it is derived
	// as "<postID>-L<line>-<seq>" so history survives as long as the line and
	// order are stable; set it explicitly to make history edit-proof.
	ID        string `yaml:"id,omitempty"`
	Line      int    `yaml:"line,omitempty"`
	Incorrect string `yaml:"incorrect"`
	Correct   string `yaml:"correct"`
	Category  string `yaml:"category,omitempty"`
	Reason    string `yaml:"reason,omitempty"`
}

// JournalCorrections holds every correction for one journal post, keyed by the
// post's ID. It is the "definitions" half of the journal/corrections split.
type JournalCorrections struct {
	ID          string       `yaml:"id"`
	Corrections []Correction `yaml:"corrections"`
}

// JournalIndex represents an index file for a journal directory (either prose
// or corrections — both use id + notebooks list).
type JournalIndex struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name,omitempty"`
	NotebookPaths []string `yaml:"notebooks"`

	Path string `yaml:"-"` // directory containing this index (internal)
}

// DerivedID returns the correction's stable id, deriving one from the post id,
// line, and per-line sequence when no explicit id is set.
func (c Correction) DerivedID(postID string, seq int) string {
	if strings.TrimSpace(c.ID) != "" {
		return c.ID
	}
	return fmt.Sprintf("%s-L%d-%d", postID, c.Line, seq)
}

// ValidateJournalEntries checks a list of journal posts: each needs a unique id
// and non-empty text.
func ValidateJournalEntries(entries []JournalEntry, location string) []ValidationError {
	var errors []ValidationError
	seen := make(map[string]struct{})
	for i, entry := range entries {
		loc := fmt.Sprintf("%s -> entry[%d]: %s", location, i, entry.ID)
		if strings.TrimSpace(entry.ID) == "" {
			errors = append(errors, ValidationError{Location: loc, Message: "entry id is empty",
				Suggestions: []string{"add a unique id to the journal post"}})
		} else if _, ok := seen[entry.ID]; ok {
			errors = append(errors, ValidationError{Location: loc,
				Message: fmt.Sprintf("duplicate entry id %q", entry.ID)})
		} else {
			seen[entry.ID] = struct{}{}
		}
		if strings.TrimSpace(entry.Text) == "" {
			errors = append(errors, ValidationError{Location: loc, Message: "entry text is empty",
				Suggestions: []string{"add the post text to the entry"}})
		}
	}
	return errors
}

// ValidateJournalCorrections checks a corrections set against its post's text:
// every incorrect span must appear in the text, the fix must differ, and each
// derived id must be unique.
func ValidateJournalCorrections(set JournalCorrections, text, location string) []ValidationError {
	var errors []ValidationError
	seen := make(map[string]struct{})
	perLine := make(map[int]int)
	for i, c := range set.Corrections {
		loc := fmt.Sprintf("%s -> correction[%d] (line %d)", location, i, c.Line)
		perLine[c.Line]++
		id := c.DerivedID(set.ID, perLine[c.Line])
		if _, ok := seen[id]; ok {
			errors = append(errors, ValidationError{Location: loc,
				Message:     fmt.Sprintf("duplicate correction id %q", id),
				Suggestions: []string{"set an explicit unique id, or a distinct line"}})
		} else {
			seen[id] = struct{}{}
		}
		if strings.TrimSpace(c.Incorrect) == "" {
			errors = append(errors, ValidationError{Location: loc, Message: "incorrect span is empty"})
		} else if text != "" && !strings.Contains(text, c.Incorrect) {
			errors = append(errors, ValidationError{Location: loc,
				Message:     fmt.Sprintf("incorrect span %q not found in post text", c.Incorrect),
				Suggestions: []string{"make the incorrect span an exact substring of the post"}})
		}
		if strings.TrimSpace(c.Correct) == "" {
			errors = append(errors, ValidationError{Location: loc, Message: "correct fix is empty"})
		} else if c.Correct == c.Incorrect {
			errors = append(errors, ValidationError{Location: loc,
				Message: "correct fix is identical to the incorrect span"})
		}
	}
	return errors
}

// walkJournalIndexFiles walks a directory tree and loads every index.yml into
// indexMap, keyed by index ID. Shared by the prose and corrections domains.
func walkJournalIndexFiles(rootDir string, indexMap map[string]JournalIndex) error {
	if rootDir == "" {
		return nil
	}
	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Base(path) != "index.yml" {
			return nil
		}
		index, err := readYamlFile[JournalIndex](path)
		if err != nil {
			return err
		}
		index.Path = filepath.Dir(path)
		indexMap[index.ID] = index
		return nil
	})
}

// LoadJournalNotebooks registers journal prose index files from the given
// directories. Opt-in, like the other domain loaders.
func (f Reader) LoadJournalNotebooks(journalDirectories []string) error {
	for _, dir := range journalDirectories {
		if err := walkJournalIndexFiles(dir, f.journalIndexes); err != nil {
			return fmt.Errorf("walkJournalIndexFiles(%s) > %w", dir, err)
		}
	}
	return nil
}

// LoadJournalCorrections registers journal-correction index files.
func (f Reader) LoadJournalCorrections(correctionsDirectories []string) error {
	for _, dir := range correctionsDirectories {
		if err := walkJournalIndexFiles(dir, f.journalCorrectionIndexes); err != nil {
			return fmt.Errorf("walkJournalIndexFiles(corrections, %s) > %w", dir, err)
		}
	}
	return nil
}

// ReadJournalEntries loads all journal posts (prose) for the given index ID.
func (f Reader) ReadJournalEntries(journalID string) ([]JournalEntry, error) {
	index, ok := f.journalIndexes[journalID]
	if !ok {
		return nil, fmt.Errorf("journal %s not found", journalID)
	}
	result := make([]JournalEntry, 0)
	for _, notebookPath := range index.NotebookPaths {
		path := filepath.Join(index.Path, notebookPath)
		entries, err := readYamlFile[[]JournalEntry](path)
		if err != nil {
			return nil, fmt.Errorf("readYamlFile(%s) > %w", path, err)
		}
		result = append(result, entries...)
	}
	return result, nil
}

// ReadJournalCorrections loads all correction sets for the given index ID,
// returned as a map keyed by post id. A journal index with no matching
// corrections index yields an empty map (posts simply have no drills yet).
func (f Reader) ReadJournalCorrections(journalID string) (map[string]JournalCorrections, error) {
	result := make(map[string]JournalCorrections)
	index, ok := f.journalCorrectionIndexes[journalID]
	if !ok {
		return result, nil
	}
	for _, notebookPath := range index.NotebookPaths {
		path := filepath.Join(index.Path, notebookPath)
		sets, err := readYamlFile[[]JournalCorrections](path)
		if err != nil {
			return nil, fmt.Errorf("readYamlFile(%s) > %w", path, err)
		}
		for _, set := range sets {
			result[set.ID] = set
		}
	}
	return result, nil
}

// GetJournalIndexes returns the registered journal prose indexes keyed by ID.
func (f Reader) GetJournalIndexes() map[string]JournalIndex {
	return f.journalIndexes
}

// CategoryCount reports how many corrections fall under a category.
type CategoryCount struct {
	Category string
	Count    int
}

// CorrectionCategoryCounts tallies corrections by category, most-frequent
// first (ties broken by name). Corrections without a category are grouped as
// "uncategorized".
func CorrectionCategoryCounts(sets []JournalCorrections) []CategoryCount {
	counts := make(map[string]int)
	for _, set := range sets {
		for _, c := range set.Corrections {
			category := c.Category
			if strings.TrimSpace(category) == "" {
				category = "uncategorized"
			}
			counts[category]++
		}
	}
	result := make([]CategoryCount, 0, len(counts))
	for category, count := range counts {
		result = append(result, CategoryCount{Category: category, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Category < result[j].Category
	})
	return result
}
