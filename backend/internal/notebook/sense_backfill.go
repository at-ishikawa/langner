package notebook

import "strings"

// BackfillSensesResult reports the outcome of a sense-backfill pass over a
// set of learning histories.
type BackfillSensesResult struct {
	// Tagged is the number of learning-history entries that were stamped
	// with a part_of_speech because exactly one sense matched.
	Tagged int
	// LeftLegacy is the number of entries left untagged because the
	// expression resolves to a genuine homograph (2+ differing senses) —
	// guessing would risk splitting or merging the wrong series.
	LeftLegacy int
}

// senseKey identifies an expression within a notebook for sense lookup.
type senseKey struct {
	notebookID string
	name       string // lowercased + trimmed
}

// BackfillSenses stamps each vocabulary learning-history entry's
// PartOfSpeech from its source note, but only when the resolution is
// unambiguous. It mutates histories in place and never moves logs between
// entries — it only fills the PartOfSpeech field on entries that are
// currently empty.
//
// Resolution rule (safe, best-effort — see design.md "Migration of
// existing YAML history"):
//
//   - Resolve the source notes for the entry's expression within the same
//     notebook (matching either the note's Entry or Usage, case-insensitive).
//   - If those notes carry exactly one distinct, non-empty sense, stamp it.
//     Non-homographs (the overwhelming majority) get full continuity and
//     key identically to future writes.
//   - If they carry two or more distinct senses (a real homograph), leave
//     the entry untagged. The commingled history stays a legacy entry; new
//     sense-tagged answers create fresh per-sense series going forward.
//
// Entries that already carry a PartOfSpeech, non-vocabulary entries (e.g.
// etymology origins), and entries with no matching source note are left
// untouched.
func BackfillSenses(histories []LearningHistory, notes []NoteRecord) BackfillSensesResult {
	index := buildSenseIndex(notes)

	var result BackfillSensesResult
	for hi := range histories {
		nbID := histories[hi].Metadata.NotebookID
		for ei := range histories[hi].Expressions {
			backfillEntry(&histories[hi].Expressions[ei], nbID, index, &result)
		}
		for si := range histories[hi].Scenes {
			for ei := range histories[hi].Scenes[si].Expressions {
				backfillEntry(&histories[hi].Scenes[si].Expressions[ei], nbID, index, &result)
			}
		}
	}
	return result
}

// buildSenseIndex maps (notebookID, name) to the set of senses declared for
// that expression in that notebook. The value maps a normalized sense token
// to the first original-cased sense string seen, so a stamp preserves the
// author's casing.
func buildSenseIndex(notes []NoteRecord) map[senseKey]map[string]string {
	index := make(map[senseKey]map[string]string)
	add := func(nbID, name, pos string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			return
		}
		key := senseKey{notebookID: nbID, name: name}
		set := index[key]
		if set == nil {
			set = make(map[string]string)
			index[key] = set
		}
		norm := normalizePartOfSpeech(pos)
		if _, ok := set[norm]; !ok {
			set[norm] = pos
		}
	}
	for i := range notes {
		n := &notes[i]
		for _, nn := range n.NotebookNotes {
			add(nn.NotebookID, n.Entry, n.PartOfSpeech)
			add(nn.NotebookID, n.Usage, n.PartOfSpeech)
		}
	}
	return index
}

// backfillEntry applies the resolution rule to a single expression entry.
func backfillEntry(entry *LearningHistoryExpression, nbID string, index map[senseKey]map[string]string, result *BackfillSensesResult) {
	// Only vocabulary entries participate; skip etymology origins and any
	// entry the user (or a prior pass) already tagged.
	if !MatchesExpressionType(entry, LearningExpressionTypeVocabulary) {
		return
	}
	if entry.PartOfSpeech != "" {
		return
	}

	set := index[senseKey{notebookID: nbID, name: strings.ToLower(strings.TrimSpace(entry.Expression))}]
	if len(set) == 0 {
		return // no matching source note — nothing to resolve
	}

	// Collect distinct senses. More than one distinct token (including the
	// empty/unspecified token) means we can't safely pick one.
	var single string
	distinct := 0
	for norm, orig := range set {
		distinct++
		if norm != "" {
			single = orig
		}
	}
	if distinct != 1 {
		result.LeftLegacy++
		return
	}
	if single == "" {
		// The lone sense is the empty/unspecified one — nothing to stamp.
		return
	}
	entry.PartOfSpeech = single
	result.Tagged++
}
