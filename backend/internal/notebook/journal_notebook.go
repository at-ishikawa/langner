package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// JournalNotebook represents a collection of free-text journal entries.
// Unlike vocabulary notebooks, the content here is the entry prose itself;
// grammar mistakes are annotations on spans inside each entry.
type JournalNotebook struct {
	Title       string         `yaml:"title"`
	Description string         `yaml:"description,omitempty"`
	Date        time.Time      `yaml:"date"`
	Entries     []JournalEntry `yaml:"entries"`
}

// JournalEntry is a single dated journal entry with its grammar mistakes.
type JournalEntry struct {
	ID       string    `yaml:"id"`
	Date     time.Time `yaml:"date,omitempty"`
	Text     string    `yaml:"text"`
	Mistakes []Mistake `yaml:"mistakes,omitempty"`
}

// Mistake is a single grammar mistake annotated on a span of an entry's text.
// Incorrect is the exact substring of the entry text that is wrong, Correct is
// its fix, and Category is a free-form label (e.g. "article", "preposition")
// used to rank which kinds of mistakes occur most. ID is stable so the
// mistake's spaced-repetition history is tracked independently.
type Mistake struct {
	ID        string `yaml:"id"`
	Incorrect string `yaml:"incorrect"`
	Correct   string `yaml:"correct"`
	Category  string `yaml:"category,omitempty"`
	Note      string `yaml:"note,omitempty"`
}

// JournalIndex represents an index file for journal directories.
// It defines a collection of journal notebooks that can be loaded together.
type JournalIndex struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	NotebookPaths []string `yaml:"notebooks"`

	// internal fields (not loaded from YAML)
	Path      string            `yaml:"-"` // directory containing this index
	Notebooks []JournalNotebook `yaml:"-"` // loaded notebooks (populated by reader)
}

// Validate validates a JournalNotebook and returns any validation errors.
// It checks that the notebook has a title, every entry has an id and text,
// and every mistake has an id, a correct fix, and an incorrect span that
// actually appears in the entry text (so it can be located for the quiz).
func (notebook *JournalNotebook) Validate(location string) []ValidationError {
	var errors []ValidationError

	if strings.TrimSpace(notebook.Title) == "" {
		errors = append(errors, ValidationError{
			Location:    location,
			Message:     "title is empty",
			Suggestions: []string{"add a title to the journal notebook"},
		})
	}

	seenMistakeIDs := make(map[string]struct{})
	for entryIdx, entry := range notebook.Entries {
		entryLocation := fmt.Sprintf("%s -> entry[%d]: %s", location, entryIdx, entry.ID)

		if strings.TrimSpace(entry.ID) == "" {
			errors = append(errors, ValidationError{
				Location:    entryLocation,
				Message:     "entry id is empty",
				Suggestions: []string{"add a unique id to the entry"},
			})
		}
		if strings.TrimSpace(entry.Text) == "" {
			errors = append(errors, ValidationError{
				Location:    entryLocation,
				Message:     "entry text is empty",
				Suggestions: []string{"add the journal text to the entry"},
			})
		}

		for mistakeIdx, mistake := range entry.Mistakes {
			mistakeLocation := fmt.Sprintf("%s -> mistake[%d]: %s", entryLocation, mistakeIdx, mistake.ID)

			if strings.TrimSpace(mistake.ID) == "" {
				errors = append(errors, ValidationError{
					Location:    mistakeLocation,
					Message:     "mistake id is empty",
					Suggestions: []string{"add a unique id to the mistake"},
				})
			} else if _, ok := seenMistakeIDs[mistake.ID]; ok {
				errors = append(errors, ValidationError{
					Location:    mistakeLocation,
					Message:     fmt.Sprintf("duplicate mistake id %q", mistake.ID),
					Suggestions: []string{"give each mistake a unique id"},
				})
			} else {
				seenMistakeIDs[mistake.ID] = struct{}{}
			}

			if strings.TrimSpace(mistake.Incorrect) == "" {
				errors = append(errors, ValidationError{
					Location:    mistakeLocation,
					Message:     "incorrect span is empty",
					Suggestions: []string{"add the incorrect text from the entry"},
				})
			} else if !strings.Contains(entry.Text, mistake.Incorrect) {
				errors = append(errors, ValidationError{
					Location:    mistakeLocation,
					Message:     fmt.Sprintf("incorrect span %q not found in entry text", mistake.Incorrect),
					Suggestions: []string{"make the incorrect span an exact substring of the entry text"},
				})
			}

			if strings.TrimSpace(mistake.Correct) == "" {
				errors = append(errors, ValidationError{
					Location:    mistakeLocation,
					Message:     "correct fix is empty",
					Suggestions: []string{"add the corrected text"},
				})
			} else if mistake.Correct == mistake.Incorrect {
				errors = append(errors, ValidationError{
					Location:    mistakeLocation,
					Message:     "correct fix is identical to the incorrect span",
					Suggestions: []string{"the correction must differ from the mistake"},
				})
			}
		}
	}

	return errors
}

// walkJournalIndexFiles walks a directory tree and loads every journal
// index.yml into indexMap, keyed by index ID. It mirrors
// walkEtymologyIndexFiles: journal directories are a distinct domain loaded
// separately from story/flashcard notebooks.
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
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "index.yml" {
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

// LoadJournalNotebooks scans the given directories for journal index files
// and registers them on the reader. It is called separately from NewReader so
// journal support is opt-in and callers that don't need it are unaffected.
func (f Reader) LoadJournalNotebooks(journalDirectories []string) error {
	for _, dir := range journalDirectories {
		if err := walkJournalIndexFiles(dir, f.journalIndexes); err != nil {
			return fmt.Errorf("walkJournalIndexFiles(%s) > %w", dir, err)
		}
	}
	return nil
}

// ReadJournalNotebooks loads all journal notebooks for the given index ID.
func (f Reader) ReadJournalNotebooks(journalID string) ([]JournalNotebook, error) {
	index, ok := f.journalIndexes[journalID]
	if !ok {
		return nil, fmt.Errorf("journal %s not found", journalID)
	}

	result := make([]JournalNotebook, 0)
	for _, notebookPath := range index.NotebookPaths {
		path := filepath.Join(index.Path, notebookPath)

		notebooks, err := readYamlFile[[]JournalNotebook](path)
		if err != nil {
			return nil, fmt.Errorf("readYamlFile(%s) > %w", path, err)
		}

		index.Notebooks = append(index.Notebooks, notebooks...)
		result = append(result, notebooks...)
	}
	f.journalIndexes[journalID] = index
	return result, nil
}

// GetJournalIndexes returns the registered journal indexes keyed by ID.
func (f Reader) GetJournalIndexes() map[string]JournalIndex {
	return f.journalIndexes
}

// MistakeCard pairs a mistake with the entry context it was found in.
// It is the unit a grammar quiz drills: the sentence is shown with the
// incorrect span, and the user must produce the corrected text.
type MistakeCard struct {
	NotebookTitle string
	EntryID       string
	Mistake       Mistake
}

// CategoryCount reports how many mistakes fall under a category.
type CategoryCount struct {
	Category string
	Count    int
}

// CategoryCounts tallies mistakes by category across the given notebooks,
// sorted by count descending (ties broken by category name) so callers can
// show which kinds of mistakes occur most. Mistakes without a category are
// grouped under "uncategorized".
func CategoryCounts(notebooks []JournalNotebook) []CategoryCount {
	counts := make(map[string]int)
	for _, notebook := range notebooks {
		for _, entry := range notebook.Entries {
			for _, mistake := range entry.Mistakes {
				category := mistake.Category
				if strings.TrimSpace(category) == "" {
					category = "uncategorized"
				}
				counts[category]++
			}
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
