package quiz

import (
	"github.com/at-ishikawa/langner/internal/notebook"
)

// conceptInfo holds the per-concept fields needed when collapsing cards.
type conceptInfo struct {
	Head    string
	Meaning string
	Members []string
}

// buildConceptIndex returns a member-expression -> *conceptInfo map for a
// single definitions book. Used by the card loaders to collapse multiple
// member expressions into one concept card. Returns nil when the book has
// no concepts: block, so callers can branch cheaply.
//
// On the rare case where an expression is claimed by two concepts, the
// first declaration in YAML order wins (the validator emits a warning at
// build time; quiz behaviour stays deterministic).
func buildConceptIndex(reader *notebook.Reader, notebookID string) map[string]*conceptInfo {
	books, ok := reader.GetDefinitionsBook(notebookID)
	if !ok {
		return nil
	}
	var index map[string]*conceptInfo
	for _, book := range books {
		for _, c := range book.Concepts {
			if c.Head == "" || len(c.Expressions) == 0 {
				continue
			}
			members := make([]string, 0, len(c.Expressions))
			for _, e := range c.Expressions {
				if e == "" {
					continue
				}
				members = append(members, e)
			}
			info := &conceptInfo{Head: c.Head, Meaning: c.Meaning, Members: members}
			if index == nil {
				index = make(map[string]*conceptInfo)
			}
			for _, e := range members {
				if _, already := index[e]; already {
					continue
				}
				index[e] = info
			}
		}
	}
	return index
}

// collapseConceptCards returns a single card per concept (preferring the
// head's row) plus all non-concept cards unchanged. Concept-decorated
// fields (ConceptHead/Members/Meaning) are populated on the surviving row.
// Order within the input slice is preserved for non-concept entries; the
// concept entry appears at the position of its first encountered member,
// which yields deterministic shuffle-input order before s.disableShuffle
// random.Shuffle runs in the calling loader.
func collapseConceptCards(cards []Card, indexByNotebook map[string]map[string]*conceptInfo) []Card {
	if len(indexByNotebook) == 0 {
		return cards
	}
	type entryKey struct {
		notebook string
		head     string
	}
	seen := make(map[entryKey]int)
	result := make([]Card, 0, len(cards))
	for _, card := range cards {
		index := indexByNotebook[card.NotebookName]
		if index == nil {
			result = append(result, card)
			continue
		}
		info, ok := index[card.Entry]
		if !ok && card.OriginalEntry != "" {
			info, ok = index[card.OriginalEntry]
		}
		if !ok {
			result = append(result, card)
			continue
		}
		key := entryKey{notebook: card.NotebookName, head: info.Head}
		if idx, already := seen[key]; already {
			// Already added a member of this concept. If this card IS the
			// head (more accurate display) and the existing one isn't,
			// swap in the head's card, preserving the concept fields.
			if existingIsHead(result[idx], info.Head) {
				continue
			}
			if cardIsHead(card, info.Head) {
				card.ConceptHead = info.Head
				card.ConceptMeaning = info.Meaning
				card.ConceptMembers = info.Members
				result[idx] = card
			}
			continue
		}
		card.ConceptHead = info.Head
		card.ConceptMeaning = info.Meaning
		card.ConceptMembers = info.Members
		seen[key] = len(result)
		result = append(result, card)
	}
	return result
}

// collapseConceptReverseCards is the ReverseCard analogue of collapseConceptCards.
func collapseConceptReverseCards(cards []ReverseCard, indexByNotebook map[string]map[string]*conceptInfo) []ReverseCard {
	if len(indexByNotebook) == 0 {
		return cards
	}
	type entryKey struct {
		notebook string
		head     string
	}
	seen := make(map[entryKey]int)
	result := make([]ReverseCard, 0, len(cards))
	for _, card := range cards {
		index := indexByNotebook[card.NotebookName]
		if index == nil {
			result = append(result, card)
			continue
		}
		info, ok := index[card.Expression]
		if !ok && card.AltForm != "" {
			info, ok = index[card.AltForm]
		}
		if !ok {
			result = append(result, card)
			continue
		}
		key := entryKey{notebook: card.NotebookName, head: info.Head}
		if idx, already := seen[key]; already {
			if result[idx].Expression == info.Head || result[idx].AltForm == info.Head {
				continue
			}
			if card.Expression == info.Head || card.AltForm == info.Head {
				card.ConceptHead = info.Head
				card.ConceptMeaning = info.Meaning
				card.ConceptMembers = info.Members
				result[idx] = card
			}
			continue
		}
		card.ConceptHead = info.Head
		card.ConceptMeaning = info.Meaning
		card.ConceptMembers = info.Members
		seen[key] = len(result)
		result = append(result, card)
	}
	return result
}

func cardIsHead(c Card, head string) bool {
	return c.Entry == head || c.OriginalEntry == head
}

func existingIsHead(c Card, head string) bool {
	return cardIsHead(c, head)
}

// buildAllConceptIndexes builds a per-notebook map of concept indexes for
// the requested notebookIDs. Notebooks with no concepts: block are absent
// from the result (the value would just be nil), so callers can iterate
// the returned map without per-key nil checks.
func buildAllConceptIndexes(reader *notebook.Reader, notebookIDs []string) map[string]map[string]*conceptInfo {
	result := make(map[string]map[string]*conceptInfo, len(notebookIDs))
	for _, id := range notebookIDs {
		if idx := buildConceptIndex(reader, id); idx != nil {
			result[id] = idx
		}
	}
	return result
}
