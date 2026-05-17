package server

import (
	"context"
	"fmt"
	"strings"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

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
			c.members = append(c.members, graphMemberInfo{
				origin:       m.Origin,
				language:     m.Language,
				sessionTitle: r.SessionTitle,
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

// buildGraphPromptForCard picks the richest applicable graph shape for
// the card and returns its GraphPrompt, or nil if none applies.
// Preference: ANTONYM_PAIR > CLUSTER. (FORM_BRANCH is selected separately
// by buildFormBranchGraphPromptForCard when concepts don't apply.)
func buildGraphPromptForCard(
	card quiz.EtymologyOriginCard,
	concepts map[string]*graphConceptInfo,
) *apiv1.GraphPrompt {
	if p := buildAntonymPairGraphPromptForCard(card, concepts); p != nil {
		return p
	}
	if p := buildClusterGraphPromptForCard(card, concepts); p != nil {
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
		label, hint := m.origin, ""
		if isBlank {
			label = ""
			hint = m.language
			blankID = nodeID
		}
		nodes = append(nodes, &apiv1.GraphNode{
			Id: nodeID, Kind: apiv1.GraphNode_ORIGIN,
			Label: label, Language: m.language, Hint: hint,
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
			label, hint := m.origin, ""
			if isBlank {
				label = ""
				hint = m.language
				blankID = nodeID
			}
			nodes = append(nodes, &apiv1.GraphNode{
				Id: nodeID, Kind: apiv1.GraphNode_ORIGIN,
				Label: label, Language: m.language, Hint: hint,
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
