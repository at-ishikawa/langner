package notebook

import (
	"fmt"
	"sort"
	"strings"
)

// MaxSenseIDLen is the maximum length of a note's sense_id (its YAML `id:`).
// It matches the notes.sense_id column width set by migration
// 023_widen_notes_sense_id (VARCHAR(380)), which in turn matches usage/entry
// (migration 013). Keep the two in lockstep: this const is the single source of
// truth the importer validates against so an over-long id is rejected with a
// readable message instead of a raw `value too long for type character
// varying(380)` (SQLSTATE 22001) deep in an insert.
const MaxSenseIDLen = 380

// OversizedSenseID names one note whose `id:` (sense_id) exceeds MaxSenseIDLen:
// the word, the notebook it came from, and the id's length.
type OversizedSenseID struct {
	Word       string
	NotebookID string
	Length     int
}

// OversizedSenseIDError aggregates every note whose id is too long for the
// notes.sense_id column, so the user can shorten them all in one pass.
type OversizedSenseIDError struct {
	Notes []OversizedSenseID
}

func (e *OversizedSenseIDError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d note id(s) exceed the %d-char sense_id limit (shorten the 'id:'):", len(e.Notes), MaxSenseIDLen)
	for _, n := range e.Notes {
		fmt.Fprintf(&b, "\n  - %q in %s (id is %d chars)", n.Word, n.NotebookID, n.Length)
	}
	return b.String()
}

// oversizedSenseIDTracker collects notes whose sense_id is too long during a
// read pass, deduped by sense_id (a note claimed by two notebooks is reported
// once). Like idlessConflictTracker, it never errors itself — the reader
// records, and only the importer gates on the result.
type oversizedSenseIDTracker struct {
	bySenseID map[string]OversizedSenseID
}

func newOversizedSenseIDTracker() *oversizedSenseIDTracker {
	return &oversizedSenseIDTracker{bySenseID: map[string]OversizedSenseID{}}
}

// record notes an id-bearing occurrence; it keeps only ids over the limit.
func (t *oversizedSenseIDTracker) record(senseID, word, notebookID string) {
	if len(senseID) <= MaxSenseIDLen {
		return
	}
	if _, ok := t.bySenseID[senseID]; ok {
		return
	}
	t.bySenseID[senseID] = OversizedSenseID{Word: word, NotebookID: notebookID, Length: len(senseID)}
}

// oversized returns the offending notes sorted by word for a stable message.
func (t *oversizedSenseIDTracker) oversized() []OversizedSenseID {
	out := make([]OversizedSenseID, 0, len(t.bySenseID))
	for _, v := range t.bySenseID {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Word != out[j].Word {
			return out[i].Word < out[j].Word
		}
		return out[i].NotebookID < out[j].NotebookID
	})
	return out
}
