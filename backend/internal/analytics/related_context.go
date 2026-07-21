package analytics

import (
	"fmt"
	"strings"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// computeVocabRelatedGroups builds the concept-graph context for a
// vocabulary card. It returns groups in this order:
//
//  1. The definitions-book concept the word belongs to (sibling
//     expressions sharing the same umbrella meaning).
//  2. The etymology concepts the word's origin_parts belong to
//     (sibling origins), and concepts connected via relations.
//
// Empty when the notebook declares no concepts or the word has no
// origin_parts to anchor on. Order matters because the analytics card
// shows the first group as the collapsed-state chip preview.
func (r *NotebookMetadataResolver) computeVocabRelatedGroups(notebookID string, note notebook.Note) []RelatedGroup {
	var out []RelatedGroup
	if g, ok := r.definitionConceptGroup(notebookID, note); ok {
		out = append(out, g)
	}
	out = append(out, r.etymologyContextGroups(notebookID, note.OriginParts)...)
	return out
}

// computeOriginRelatedGroups builds the concept-graph context for an
// etymology-origin card. The card sits squarely on the etymology side,
// so only origin-family + relation groups apply — there is no
// definitions-book concept to consult.
func (r *NotebookMetadataResolver) computeOriginRelatedGroups(notebookID, origin string) []RelatedGroup {
	return r.etymologyContextGroups(notebookID, []notebook.OriginPartRef{{Origin: origin}})
}

// definitionConceptGroup returns the "concept" group for the matched
// note's expression. When the word is the SOLE member of its concept
// (i.e. there are no siblings to show), the group is dropped because a
// single-member chip adds noise rather than context.
func (r *NotebookMetadataResolver) definitionConceptGroup(notebookID string, note notebook.Note) (RelatedGroup, bool) {
	byMember := r.reader.GetDefinitionsBookConceptByMember(notebookID)
	if len(byMember) == 0 {
		return RelatedGroup{}, false
	}
	info, ok := lookupConceptForNote(byMember, note)
	if !ok {
		return RelatedGroup{}, false
	}
	var siblings []string
	for _, m := range info.Members {
		if strings.EqualFold(m, note.Expression) || strings.EqualFold(m, note.Definition) {
			continue
		}
		siblings = append(siblings, m)
	}
	if len(siblings) == 0 {
		return RelatedGroup{}, false
	}
	return RelatedGroup{
		Kind:    "concept",
		Label:   info.Meaning,
		Members: siblings,
	}, true
}

// lookupConceptForNote consults the per-member concept index for either
// the note's expression or its definition alias — mirrors matchExpression
// in the resolver so the same word-form drift the meaning lookup
// absorbs also resolves the concept membership.
func lookupConceptForNote(byMember map[string]notebook.DefinitionConceptInfo, note notebook.Note) (notebook.DefinitionConceptInfo, bool) {
	if info, ok := byMember[note.Expression]; ok {
		return info, true
	}
	if note.Definition != "" {
		if info, ok := byMember[note.Definition]; ok {
			return info, true
		}
	}
	for k, info := range byMember {
		if strings.EqualFold(k, note.Expression) || (note.Definition != "" && strings.EqualFold(k, note.Definition)) {
			return info, true
		}
	}
	return notebook.DefinitionConceptInfo{}, false
}

// etymologyContextGroups returns the origin-family + relation groups
// for the given origin parts. For each origin part we find the
// etymology concept it belongs to (if any) and emit:
//
//   - "origin_family" group with sibling origins (other origins under
//     the same concept).
//   - One group per relation the concept participates in, kind set to
//     the relation type ("antonym", "synonym", "hyponym", …) with the
//     other concept's origin members.
//
// Concepts already covered for a previous origin_part are skipped so a
// word with two parts under the same concept doesn't show duplicates.
func (r *NotebookMetadataResolver) etymologyContextGroups(notebookID string, originParts []notebook.OriginPartRef) []RelatedGroup {
	concepts, relations := r.reader.GetEtymologyConceptsAndRelations(notebookID)
	if len(concepts) == 0 {
		return nil
	}
	byKey := make(map[string]notebook.Concept, len(concepts))
	for _, c := range concepts {
		byKey[c.Key] = c
	}
	// originMeaning lookup so members can render "sinister (Latin) — left".
	originMeaning := make(map[string]string)
	if origins, err := r.reader.ReadEtymologyNotebook(notebookID); err == nil {
		for _, o := range origins {
			originMeaning[o.Origin] = o.Meaning
		}
	}
	conceptForOrigin := make(map[string]string) // origin -> concept key
	for _, c := range concepts {
		for _, m := range c.Members {
			if _, already := conceptForOrigin[m.Origin]; already {
				continue
			}
			conceptForOrigin[m.Origin] = c.Key
		}
	}
	covered := make(map[string]bool)
	var out []RelatedGroup
	for _, op := range originParts {
		key, ok := conceptForOrigin[op.Origin]
		if !ok || covered[key] {
			continue
		}
		covered[key] = true
		concept := byKey[key]
		family := siblingOriginGroup(concept, op.Origin, originMeaning)
		if family != nil {
			out = append(out, *family)
		}
		out = append(out, relationGroupsFor(concept.Key, concepts, byKey, relations, originMeaning, op.Origin)...)
	}
	return out
}

// siblingOriginGroup builds the "origin_family" group for a concept,
// dropping the originating origin so the user sees only the SIBLINGS
// they didn't quiz on. Returns nil when the concept has no siblings
// (single-member concept).
func siblingOriginGroup(concept notebook.Concept, selfOrigin string, originMeaning map[string]string) *RelatedGroup {
	var members []string
	for _, m := range concept.Members {
		if m.Origin == selfOrigin {
			continue
		}
		members = append(members, formatOriginMember(m, originMeaning))
	}
	if len(members) == 0 {
		return nil
	}
	return &RelatedGroup{
		Kind:    "origin_family",
		Label:   labelForConcept(concept),
		Members: members,
	}
}

// relationGroupsFor walks every relation that involves the given
// concept key and emits one group per related concept. Symmetric
// relations (between) and directed relations (from / to) are both
// honored. Each group's members are the other concept's origin members,
// rendered with language + meaning where available. selfOrigin is
// excluded so a word's own origin doesn't appear in its own relation
// group (only relevant for self-relations, which are rare but possible).
func relationGroupsFor(
	conceptKey string,
	allConcepts []notebook.Concept,
	byKey map[string]notebook.Concept,
	relations []notebook.Relation,
	originMeaning map[string]string,
	selfOrigin string,
) []RelatedGroup {
	var out []RelatedGroup
	for _, rel := range relations {
		other, ok := otherEndpoint(rel, conceptKey)
		if !ok {
			continue
		}
		otherConcept, ok := byKey[other]
		if !ok {
			continue
		}
		var members []string
		for _, m := range otherConcept.Members {
			if m.Origin == selfOrigin {
				continue
			}
			members = append(members, formatOriginMember(m, originMeaning))
		}
		if len(members) == 0 {
			continue
		}
		out = append(out, RelatedGroup{
			Kind:    rel.Type,
			Label:   labelForConcept(otherConcept),
			Members: members,
		})
	}
	_ = allConcepts // reserved for hypothetical multi-hop expansion later
	return out
}

// otherEndpoint returns the concept key on the other side of the
// relation. For directed relations, only the From-side concept gets an
// outbound edge (so "leftness antonym rightness" surfaces under
// leftness, not rightness). Symmetric "between" relations surface from
// either side.
func otherEndpoint(rel notebook.Relation, self string) (string, bool) {
	if rel.IsDirected() {
		if rel.From == self {
			return rel.To, true
		}
		return "", false
	}
	if len(rel.Between) != 2 {
		return "", false
	}
	if rel.Between[0] == self {
		return rel.Between[1], true
	}
	if rel.Between[1] == self {
		return rel.Between[0], true
	}
	return "", false
}

// formatOriginMember renders a ConceptMember as the analytics-card
// string the frontend just prints verbatim: "origin (Language) — meaning".
// Language and meaning collapse out when missing so a concept member
// declared as bare {origin: sinister} still renders cleanly.
func formatOriginMember(m notebook.ConceptMember, originMeaning map[string]string) string {
	out := m.Origin
	if m.Language != "" {
		out = fmt.Sprintf("%s (%s)", out, m.Language)
	}
	if meaning := originMeaning[m.Origin]; meaning != "" {
		out = fmt.Sprintf("%s — %s", out, meaning)
	}
	return out
}

// labelForConcept builds the human-readable header for a related
// group's card. The umbrella meaning is preferred over the bare key
// because keys are slug-like ("leftness", "kinship") and the meaning
// reads as natural language. When key and meaning are the same word
// (e.g. an "eye" concept whose meaning is also "eye"), the em-dash
// pair would render as "eye — eye" — collapse to just the key.
func labelForConcept(c notebook.Concept) string {
	switch {
	case c.Meaning != "" && c.Key != "" && !strings.EqualFold(strings.TrimSpace(c.Key), strings.TrimSpace(c.Meaning)):
		return fmt.Sprintf("%s — %s", c.Key, c.Meaning)
	case c.Meaning != "":
		return c.Meaning
	default:
		return c.Key
	}
}
