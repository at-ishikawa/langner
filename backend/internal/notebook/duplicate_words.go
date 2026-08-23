package notebook

import (
	"fmt"
	"sort"
	"strings"
)

// DuplicateWordOccurrence is one place an id-less word was declared: the
// notebook it came from and the meaning given there.
type DuplicateWordOccurrence struct {
	NotebookID string
	Meaning    string
}

// DuplicateWord is an id-less word (no `id:`) declared in more than one place
// with DIFFERENT meanings — a homograph the author forgot to disambiguate. The
// DB is unique on (usage, entry) for id-less rows, so these cannot coexist;
// they must be given distinct `id:`s. Identical-meaning duplicates are NOT
// listed here — they legitimately dedup to one note.
type DuplicateWord struct {
	Word        string
	Occurrences []DuplicateWordOccurrence
}

// DuplicateWordsError aggregates every undisambiguated id-less duplicate found
// in one import so the user can fix them all in a single pass. Its message
// names each word and every location + meaning, and tells the user the fix
// (add a distinct `id:`).
type DuplicateWordsError struct {
	Words []DuplicateWord
}

func (e *DuplicateWordsError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d undisambiguated duplicate word(s) (same spelling, no id, different meanings). Add a distinct 'id:' to each:", len(e.Words))
	for _, w := range e.Words {
		parts := make([]string, 0, len(w.Occurrences))
		for _, o := range w.Occurrences {
			parts = append(parts, fmt.Sprintf("%s (meaning: %q)", o.NotebookID, o.Meaning))
		}
		fmt.Fprintf(&b, "\n  - %q: %s", w.Word, strings.Join(parts, " vs "))
	}
	return b.String()
}

// idlessConflictTracker accumulates id-less word occurrences during a read pass
// and, at the end, reports the ones declared with more than one distinct
// meaning. Kept separate from the KEY rule (CanonicalNoteKey) so the reader
// still collapses duplicates for callers like assign-ids; only the importer
// gates on the reported conflicts.
type idlessConflictTracker struct {
	// word -> ordered distinct occurrences (deduped by notebook+meaning)
	occ  map[string][]DuplicateWordOccurrence
	seen map[string]map[string]bool // word -> "notebookID\x00meaning" -> true
}

func newIdlessConflictTracker() *idlessConflictTracker {
	return &idlessConflictTracker{
		occ:  map[string][]DuplicateWordOccurrence{},
		seen: map[string]map[string]bool{},
	}
}

// record notes one id-less occurrence of word (its notebook and meaning).
func (t *idlessConflictTracker) record(word, notebookID, meaning string) {
	if t.seen[word] == nil {
		t.seen[word] = map[string]bool{}
	}
	sig := notebookID + "\x00" + meaning
	if t.seen[word][sig] {
		return
	}
	t.seen[word][sig] = true
	t.occ[word] = append(t.occ[word], DuplicateWordOccurrence{NotebookID: notebookID, Meaning: meaning})
}

// conflicts returns, sorted by word, every id-less word declared with >= 2
// distinct trimmed meanings.
func (t *idlessConflictTracker) conflicts() []DuplicateWord {
	var out []DuplicateWord
	for word, occs := range t.occ {
		distinct := map[string]bool{}
		for _, o := range occs {
			distinct[strings.TrimSpace(o.Meaning)] = true
		}
		if len(distinct) < 2 {
			continue
		}
		sorted := append([]DuplicateWordOccurrence(nil), occs...)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].NotebookID != sorted[j].NotebookID {
				return sorted[i].NotebookID < sorted[j].NotebookID
			}
			return sorted[i].Meaning < sorted[j].Meaning
		})
		out = append(out, DuplicateWord{Word: word, Occurrences: sorted})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Word < out[j].Word })
	return out
}
