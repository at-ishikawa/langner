package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// graphAnswerMask is the placeholder shown in a reverse-quiz graph for a
// sibling node (or a meaning-text mention) whose origin is the answer to
// a LATER question in the same session. Mirrors the "[...]" marker the
// vocab reverse quiz uses for not-yet-asked words.
const graphAnswerMask = "[…]"

// maskEtymologyGraphFutureAnswers hides, in each card's graph, any origin
// that is the answer (blank) of a card LATER in the session — so a
// cluster/antonym graph rendered for card i never reveals the word card
// i+1 will ask. Already-answered (earlier) cards stay visible, matching
// the vocab reverse quiz's forward-mask. Masks three leak channels:
//   - a visible sibling ORIGIN node whose label IS a future answer,
//   - a future answer mentioned inside a visible sibling's meaning prose,
//   - a future answer mentioned inside the blank node's own prompt
//     (e.g. "written (past participle of scribo)" when scribo is asked
//     next) — the prompt stays usable, only the leaked origin is hidden.
//
// Cards must be in session order (StartEtymologyQuiz returns them that
// way and the frontend does not reshuffle etymology cards).
func maskEtymologyGraphFutureAnswers(cards []*apiv1.EtymologyQuizCard) {
	for i := range cards {
		gp := cards[i].GetGraphPrompt()
		if gp == nil {
			continue
		}
		future := make(map[string]bool)
		for j := i + 1; j < len(cards); j++ {
			o := strings.ToLower(strings.TrimSpace(cards[j].GetOrigin()))
			if o != "" {
				future[o] = true
			}
		}
		if len(future) == 0 {
			continue
		}
		for _, n := range gp.Nodes {
			if n.GetKind() != apiv1.GraphNode_ORIGIN {
				continue
			}
			if n.GetId() == gp.GetBlankNodeId() {
				// Keep the prompt, but scrub any future-answer origin it names.
				n.Hint = maskFutureOriginsInText(n.GetHint(), future)
				continue
			}
			if future[strings.ToLower(strings.TrimSpace(n.GetLabel()))] {
				n.Label = graphAnswerMask
				n.Meaning = ""
				continue
			}
			n.Meaning = maskFutureOriginsInText(n.GetMeaning(), future)
		}
	}
}

// maskFutureOriginsInText replaces whole-word, case-insensitive
// occurrences of each future-answer origin in text with the answer mask.
func maskFutureOriginsInText(text string, future map[string]bool) string {
	if text == "" || len(future) == 0 {
		return text
	}
	for origin := range future {
		re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(origin) + `\b`)
		if err != nil {
			continue
		}
		text = re.ReplaceAllString(text, graphAnswerMask)
	}
	return text
}

// graphConceptInfo aggregates per-book concept data loaded from the YAML
// source: members (with their session) and outgoing relations. Built
// once per StartEtymologyQuiz request via loadBookConcepts so the
// per-card builders can share it.
type graphConceptInfo struct {
	key       string
	meaning   string
	members   []graphMemberInfo
	relations []graphRelationInfo
}

type graphMemberInfo struct {
	origin       string
	language     string
	sessionTitle string
	// meaning is the YAML prose gloss for this origin (e.g. "written
	// (past participle of scribo)") — populated by joining each member
	// against the etymology origins source so the cluster / antonym-pair
	// graph can show how members relate to each other, not just that
	// they share a concept.
	meaning string
}

type graphRelationInfo struct {
	relType    string
	toKey      string
	isDirected bool
}

// loadBookConcepts reads every concept and relation declared for the
// given notebook, merging same-key concepts across sessions and attaching
// each concept's outgoing relations. Symmetric relations are surfaced on
// BOTH endpoints so the per-card builder can scan a single concept's
// relations to find its antonym partner.
func loadBookConcepts(ctx context.Context, reader *notebook.Reader, notebookID string) map[string]*graphConceptInfo {
	conceptSrc := notebook.NewYAMLSemanticConceptSource(reader)
	conceptRows, err := conceptSrc.FindAll(ctx)
	if err != nil {
		return nil
	}
	relSrc := notebook.NewYAMLConceptRelationSource(reader)
	relRows, _ := relSrc.FindAll(ctx)

	// Build a lookup of (origin, language, sessionTitle) → meaning prose
	// so members get their YAML gloss attached. Falling back to (origin,
	// language) without session covers concepts whose members reference
	// origins introduced in an earlier session of the same book.
	originSrc := notebook.NewYAMLEtymologyOriginSource(reader)
	originRows, _ := originSrc.FindAll(ctx)
	meaningByMember := make(map[string]string)
	for _, o := range originRows {
		if o.NotebookID != notebookID {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(o.Origin)) + "|" + o.Language + "|" + o.SessionTitle
		meaningByMember[key] = o.Meaning
		fallback := strings.ToLower(strings.TrimSpace(o.Origin)) + "|" + o.Language
		if _, ok := meaningByMember[fallback]; !ok {
			meaningByMember[fallback] = o.Meaning
		}
	}

	concepts := make(map[string]*graphConceptInfo)
	for _, r := range conceptRows {
		if r.NotebookID != notebookID {
			continue
		}
		c, ok := concepts[r.Key]
		if !ok {
			c = &graphConceptInfo{key: r.Key, meaning: r.Meaning}
			concepts[r.Key] = c
		}
		for _, m := range r.Members {
			memberKey := strings.ToLower(strings.TrimSpace(m.Origin)) + "|" + m.Language + "|" + r.SessionTitle
			meaning := meaningByMember[memberKey]
			if meaning == "" {
				meaning = meaningByMember[strings.ToLower(strings.TrimSpace(m.Origin))+"|"+m.Language]
			}
			c.members = append(c.members, graphMemberInfo{
				origin:       m.Origin,
				language:     m.Language,
				sessionTitle: r.SessionTitle,
				meaning:      meaning,
			})
		}
	}
	for _, rel := range relRows {
		if rel.NotebookID != notebookID {
			continue
		}
		if c, ok := concepts[rel.FromKey]; ok {
			c.relations = append(c.relations, graphRelationInfo{
				relType: rel.Type, toKey: rel.ToKey, isDirected: rel.IsDirected,
			})
		}
		if !rel.IsDirected {
			if c, ok := concepts[rel.ToKey]; ok {
				c.relations = append(c.relations, graphRelationInfo{
					relType: rel.Type, toKey: rel.FromKey, isDirected: false,
				})
			}
		}
	}
	return concepts
}

// conceptsContaining returns concepts (in deterministic order) whose
// member set includes the card's (origin, language, sessionTitle).
func conceptsContaining(card quiz.EtymologyOriginCard, concepts map[string]*graphConceptInfo) []*graphConceptInfo {
	lowerOrigin := strings.ToLower(strings.TrimSpace(card.Origin))
	var matches []*graphConceptInfo
	for _, c := range concepts {
		for _, m := range c.members {
			if strings.ToLower(strings.TrimSpace(m.origin)) == lowerOrigin &&
				m.language == card.Language && m.sessionTitle == card.SessionTitle {
				matches = append(matches, c)
				break
			}
		}
	}
	// Stable order by key for deterministic test/prod behavior.
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].key < matches[i].key {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
	return matches
}

// buildGraphContextForCard returns the same graph shape buildGraphPromptForCard
// would, but with the blank filled in — used as elaborative scaffold in
// the standard-mode feedback card after the user has already answered.
// The card's origin appears as a normal labelled node rather than a
// blank input; the frontend highlights it for the learner's eye.
func buildGraphContextForCard(
	ctx context.Context,
	reader *notebook.Reader,
	card quiz.EtymologyOriginCard,
	concepts map[string]*graphConceptInfo,
) *apiv1.GraphPrompt {
	prompt := buildGraphPromptForCard(ctx, reader, card, concepts)
	if prompt == nil || prompt.BlankNodeId == "" {
		return prompt
	}
	for _, n := range prompt.Nodes {
		if n.Id == prompt.BlankNodeId {
			n.Label = card.Origin
			n.Hint = ""
		}
	}
	prompt.BlankNodeId = ""
	return prompt
}

// buildGraphPromptForCard picks the richest applicable graph shape for
// the card and returns its GraphPrompt, or nil if none applies.
// Preference: ANTONYM_PAIR > CLUSTER > FORM_BRANCH — antonym pairs
// reinforce cross-concept contrast (highest learning value), clusters
// reinforce membership in one concept, form branches teach the Latin →
// English derivation pipeline (useful when no concept membership exists).
func buildGraphPromptForCard(
	ctx context.Context,
	reader *notebook.Reader,
	card quiz.EtymologyOriginCard,
	concepts map[string]*graphConceptInfo,
) *apiv1.GraphPrompt {
	if p := buildAntonymPairGraphPromptForCard(card, concepts); p != nil {
		return p
	}
	if p := buildClusterGraphPromptForCard(card, concepts); p != nil {
		return p
	}
	if p := buildFormBranchGraphPromptForCard(ctx, reader, card); p != nil {
		return p
	}
	return nil
}

// buildClusterGraphPromptForCard returns a CLUSTER shape when the card's
// origin belongs to a concept with at least 2 members. The concept node
// is centred; each member origin is a node connected via member_of. The
// card's own origin is the blank.
func buildClusterGraphPromptForCard(
	card quiz.EtymologyOriginCard,
	concepts map[string]*graphConceptInfo,
) *apiv1.GraphPrompt {
	cards := conceptsContaining(card, concepts)
	// Prefer the concept with the most members + widest language coverage.
	var best *graphConceptInfo
	for _, c := range cards {
		if len(c.members) < 2 {
			continue
		}
		if best == nil {
			best = c
			continue
		}
		if len(c.members) > len(best.members) {
			best = c
		} else if len(c.members) == len(best.members) {
			if distinctLanguages(c) > distinctLanguages(best) {
				best = c
			}
		}
	}
	if best == nil {
		return nil
	}
	return clusterPrompt(card, best)
}

// distinctLanguages counts unique languages across a concept's members.
func distinctLanguages(c *graphConceptInfo) int {
	seen := make(map[string]bool)
	for _, m := range c.members {
		seen[m.language] = true
	}
	return len(seen)
}

// clusterPrompt assembles a CLUSTER GraphPrompt for a specific concept
// with the card's origin as the blank node.
func clusterPrompt(card quiz.EtymologyOriginCard, c *graphConceptInfo) *apiv1.GraphPrompt {
	lowerOrigin := strings.ToLower(strings.TrimSpace(card.Origin))
	conceptNodeID := "concept"
	nodes := []*apiv1.GraphNode{
		{Id: conceptNodeID, Kind: apiv1.GraphNode_CONCEPT, Label: c.meaning, Hint: c.key},
	}
	var edges []*apiv1.GraphEdge
	blankID := ""
	seen := make(map[string]bool)
	for i, m := range c.members {
		dedup := strings.ToLower(strings.TrimSpace(m.origin)) + "|" + m.language
		if seen[dedup] {
			continue
		}
		seen[dedup] = true
		nodeID := fmt.Sprintf("m%d", i)
		isBlank := strings.ToLower(strings.TrimSpace(m.origin)) == lowerOrigin &&
			m.language == card.Language && m.sessionTitle == card.SessionTitle
		label, hint, meaning := m.origin, "", m.meaning
		if isBlank {
			label = ""
			// The blank shows the meaning as the prompt (the user types
			// the origin from the meaning) — without it, a concept with
			// many members is impossible to disambiguate. card.Meaning is
			// the blanked origin's gloss. meaning stays empty so it isn't
			// also rendered as the annotation line below the input.
			hint = card.Meaning
			meaning = ""
			blankID = nodeID
		}
		nodes = append(nodes, &apiv1.GraphNode{
			Id: nodeID, Kind: apiv1.GraphNode_ORIGIN,
			Label: label, Language: m.language, Hint: hint, Meaning: meaning,
		})
		edges = append(edges, &apiv1.GraphEdge{From: nodeID, To: conceptNodeID, Type: "member_of"})
	}
	if blankID == "" {
		return nil
	}
	return &apiv1.GraphPrompt{
		Shape: apiv1.GraphPrompt_CLUSTER, Nodes: nodes, Edges: edges, BlankNodeId: blankID,
	}
}

// buildAntonymPairGraphPromptForCard returns an ANTONYM_PAIR shape when
// the card's origin is in a concept that has an `antonym` relation to
// another concept in the same book. Both concepts are rendered with
// their members; the card's own origin is the blank.
func buildAntonymPairGraphPromptForCard(
	card quiz.EtymologyOriginCard,
	concepts map[string]*graphConceptInfo,
) *apiv1.GraphPrompt {
	cards := conceptsContaining(card, concepts)
	var thisConcept, otherConcept *graphConceptInfo
	for _, c := range cards {
		for _, rel := range c.relations {
			if rel.relType != "antonym" {
				continue
			}
			other, ok := concepts[rel.toKey]
			if !ok {
				continue
			}
			if len(other.members) == 0 {
				continue
			}
			thisConcept = c
			otherConcept = other
			break
		}
		if thisConcept != nil {
			break
		}
	}
	if thisConcept == nil || otherConcept == nil {
		return nil
	}
	return antonymPairPrompt(card, thisConcept, otherConcept)
}

// antonymPairPrompt assembles two concept nodes connected by an antonym
// edge, each with its member origins below. The card's origin is the
// blank node.
func antonymPairPrompt(card quiz.EtymologyOriginCard, this, other *graphConceptInfo) *apiv1.GraphPrompt {
	lowerOrigin := strings.ToLower(strings.TrimSpace(card.Origin))
	thisConceptID, otherConceptID := "concept_a", "concept_b"
	nodes := []*apiv1.GraphNode{
		{Id: thisConceptID, Kind: apiv1.GraphNode_CONCEPT, Label: this.meaning, Hint: this.key},
		{Id: otherConceptID, Kind: apiv1.GraphNode_CONCEPT, Label: other.meaning, Hint: other.key},
	}
	edges := []*apiv1.GraphEdge{
		{From: thisConceptID, To: otherConceptID, Type: "antonym"},
	}
	blankID := ""
	addMembers := func(c *graphConceptInfo, conceptID, prefix string) {
		seen := make(map[string]bool)
		for i, m := range c.members {
			dedup := strings.ToLower(strings.TrimSpace(m.origin)) + "|" + m.language
			if seen[dedup] {
				continue
			}
			seen[dedup] = true
			nodeID := fmt.Sprintf("%s%d", prefix, i)
			isBlank := strings.ToLower(strings.TrimSpace(m.origin)) == lowerOrigin &&
				m.language == card.Language && m.sessionTitle == card.SessionTitle
			label, hint, meaning := m.origin, "", m.meaning
			if isBlank {
				label = ""
				// Show the blanked origin's meaning as the prompt; see
				// clusterPrompt for the rationale.
				hint = card.Meaning
				meaning = ""
				blankID = nodeID
			}
			nodes = append(nodes, &apiv1.GraphNode{
				Id: nodeID, Kind: apiv1.GraphNode_ORIGIN,
				Label: label, Language: m.language, Hint: hint, Meaning: meaning,
			})
			edges = append(edges, &apiv1.GraphEdge{From: nodeID, To: conceptID, Type: "member_of"})
		}
	}
	addMembers(this, thisConceptID, "a")
	addMembers(other, otherConceptID, "b")
	if blankID == "" {
		return nil
	}
	return &apiv1.GraphPrompt{
		Shape: apiv1.GraphPrompt_ANTONYM_PAIR, Nodes: nodes, Edges: edges, BlankNodeId: blankID,
	}
}

// buildFormBranchGraphPromptForCard returns a FORM_BRANCH shape when the
// card's origin has at least one form with at least one English
// derivation. The origin headword is the blank; the user types the
// origin given the form tree and the English words it produces as
// context. This teaches "which Latin word produced these English
// derivations?" — the inverse of "which form produced this English word?"
func buildFormBranchGraphPromptForCard(
	ctx context.Context,
	reader *notebook.Reader,
	card quiz.EtymologyOriginCard,
) *apiv1.GraphPrompt {
	originSrc := notebook.NewYAMLEtymologyOriginSource(reader)
	originRows, err := originSrc.FindAll(ctx)
	if err != nil {
		return nil
	}
	// Find the card's origin record so we can read its forms.
	lowerOrigin := strings.ToLower(strings.TrimSpace(card.Origin))
	var cardForms []notebook.EtymologyOriginForm
	for _, r := range originRows {
		if r.NotebookID != card.NotebookName {
			continue
		}
		if r.SessionTitle != card.SessionTitle {
			continue
		}
		if r.Sense != card.Sense {
			continue
		}
		if strings.ToLower(strings.TrimSpace(r.Origin)) != lowerOrigin {
			continue
		}
		if r.Language != card.Language {
			continue
		}
		// Re-read the YAML to pick up Forms (the source struct drops them).
		// Cheaper alternative: have the source emit Forms; for v1 fetch from
		// the per-origin reader. The forms data lives on EtymologyOrigin in
		// the reader's parsed view.
		break
	}
	// Forms come from the parsed origins (the source struct above doesn't
	// carry them). Read them from the reader directly.
	origins, err := reader.ReadEtymologyNotebook(card.NotebookName)
	if err != nil {
		return nil
	}
	for _, o := range origins {
		if o.SessionTitle == card.SessionTitle &&
			strings.ToLower(strings.TrimSpace(o.Origin)) == lowerOrigin &&
			o.Language == card.Language && o.Sense == card.Sense {
			cardForms = o.Forms
			break
		}
	}
	if len(cardForms) == 0 {
		return nil
	}

	// Collect English derivations for each form by scanning definitions
	// whose origin_parts pin a from_form on this origin.
	englishByForm := make(map[string][]string)
	bookIDs := reader.GetDefinitionsBookIDs()
	for _, bookID := range bookIDs {
		if bookID != card.NotebookName {
			continue
		}
		defs, ok := reader.GetDefinitionsNotes(bookID)
		if !ok {
			continue
		}
		for sessionTitle, sceneDefs := range defs {
			if sessionTitle != card.SessionTitle {
				continue
			}
			for _, notes := range sceneDefs {
				for _, note := range notes {
					for _, ref := range note.OriginParts {
						if strings.ToLower(strings.TrimSpace(ref.Origin)) != lowerOrigin {
							continue
						}
						if ref.Language != card.Language {
							continue
						}
						if ref.FromForm == "" {
							continue
						}
						expr := note.Expression
						if expr == "" {
							expr = note.Definition
						}
						if expr == "" {
							continue
						}
						englishByForm[ref.FromForm] = append(englishByForm[ref.FromForm], expr)
					}
				}
			}
		}
	}
	// Require at least one form with at least one English derivation.
	hasAny := false
	for _, ws := range englishByForm {
		if len(ws) > 0 {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return nil
	}

	// Build the graph: origin (blank) → forms → English words.
	originNodeID := "origin"
	nodes := []*apiv1.GraphNode{
		{
			Id: originNodeID, Kind: apiv1.GraphNode_ORIGIN,
			Label: "", Language: card.Language, Hint: card.Meaning,
		},
	}
	edges := []*apiv1.GraphEdge{}
	// Cap English derivations per form to keep the prompt readable.
	const maxEnglishPerForm = 3
	for i, f := range cardForms {
		formNodeID := fmt.Sprintf("form%d", i)
		nodes = append(nodes, &apiv1.GraphNode{
			Id: formNodeID, Kind: apiv1.GraphNode_FORM,
			Label: f.Form, Hint: f.Role,
		})
		edges = append(edges, &apiv1.GraphEdge{From: originNodeID, To: formNodeID, Type: "has_form"})
		english := englishByForm[f.Form]
		if len(english) > maxEnglishPerForm {
			english = english[:maxEnglishPerForm]
		}
		for j, expr := range english {
			engNodeID := fmt.Sprintf("eng%d_%d", i, j)
			nodes = append(nodes, &apiv1.GraphNode{
				Id: engNodeID, Kind: apiv1.GraphNode_ENGLISH_WORD, Label: expr,
			})
			edges = append(edges, &apiv1.GraphEdge{From: formNodeID, To: engNodeID, Type: "derives"})
		}
	}
	return &apiv1.GraphPrompt{
		Shape: apiv1.GraphPrompt_FORM_BRANCH, Nodes: nodes, Edges: edges, BlankNodeId: originNodeID,
	}
}
