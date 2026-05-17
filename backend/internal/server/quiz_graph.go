package server

import (
	"context"
	"fmt"
	"strings"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// buildClusterGraphPromptForCard returns a CLUSTER-shape GraphPrompt for
// the given reverse-mode card, or nil when the card's origin doesn't
// participate in a usable concept (at least 2 members across languages).
//
// Cluster shape: the concept is a CONCEPT node; every member origin is an
// ORIGIN node connected via a `member_of` edge. The blank is the node for
// the card's (origin, sessionTitle) sense; its `hint` is the language so
// the learner knows which language to produce.
func buildClusterGraphPromptForCard(
	ctx context.Context,
	reader *notebook.Reader,
	card quiz.EtymologyOriginCard,
) *apiv1.GraphPrompt {
	conceptSrc := notebook.NewYAMLSemanticConceptSource(reader)
	rows, err := conceptSrc.FindAll(ctx)
	if err != nil {
		return nil
	}

	// Group by concept_key book-scoped to this card's notebook, merging
	// per-session declarations (matching the API hydration logic).
	type memberKey struct {
		origin       string
		language     string
		sessionTitle string
	}
	type conceptInfo struct {
		key     string
		meaning string
		members []memberKey
	}
	concepts := make(map[string]*conceptInfo)
	for _, r := range rows {
		if r.NotebookID != card.NotebookName {
			continue
		}
		c, ok := concepts[r.Key]
		if !ok {
			c = &conceptInfo{key: r.Key, meaning: r.Meaning}
			concepts[r.Key] = c
		}
		for _, m := range r.Members {
			c.members = append(c.members, memberKey{
				origin:       m.Origin,
				language:     m.Language,
				sessionTitle: r.SessionTitle,
			})
		}
	}

	// Find candidate concepts containing this card's origin (case-
	// insensitive). Prefer the cluster with the most members and the
	// widest language coverage — that gives the richest learning signal.
	cardOriginLower := strings.ToLower(strings.TrimSpace(card.Origin))
	var best *conceptInfo
	for _, c := range concepts {
		var containsCard bool
		var distinctLangs = make(map[string]bool)
		for _, m := range c.members {
			if strings.ToLower(strings.TrimSpace(m.origin)) == cardOriginLower &&
				m.sessionTitle == card.SessionTitle {
				containsCard = true
			}
			distinctLangs[m.language] = true
		}
		if !containsCard || len(c.members) < 2 {
			continue
		}
		if best == nil {
			best = c
			continue
		}
		// Prefer more members; tie-break by more distinct languages.
		bestLangs := make(map[string]bool)
		for _, bm := range best.members {
			bestLangs[bm.language] = true
		}
		if len(c.members) > len(best.members) ||
			(len(c.members) == len(best.members) && len(distinctLangs) > len(bestLangs)) {
			best = c
		}
	}
	if best == nil {
		return nil
	}

	// Build the graph: concept node + member nodes + member_of edges.
	conceptNodeID := "concept"
	nodes := []*apiv1.GraphNode{
		{
			Id:    conceptNodeID,
			Kind:  apiv1.GraphNode_CONCEPT,
			Label: best.meaning,
			Hint:  best.key,
		},
	}
	var edges []*apiv1.GraphEdge
	blankID := ""
	seenMember := make(map[string]bool) // (origin, language) dedupe for nodes
	for i, m := range best.members {
		dedup := strings.ToLower(strings.TrimSpace(m.origin)) + "|" + m.language
		if seenMember[dedup] {
			continue
		}
		seenMember[dedup] = true
		nodeID := fmt.Sprintf("m%d", i)
		isBlank := strings.ToLower(strings.TrimSpace(m.origin)) == cardOriginLower &&
			m.sessionTitle == card.SessionTitle
		label := m.origin
		hint := ""
		if isBlank {
			label = "" // blank reveals nothing
			hint = m.language
			blankID = nodeID
		}
		nodes = append(nodes, &apiv1.GraphNode{
			Id:       nodeID,
			Kind:     apiv1.GraphNode_ORIGIN,
			Label:    label,
			Language: m.language,
			Hint:     hint,
		})
		edges = append(edges, &apiv1.GraphEdge{
			From: nodeID,
			To:   conceptNodeID,
			Type: "member_of",
		})
	}
	if blankID == "" {
		return nil
	}
	return &apiv1.GraphPrompt{
		Shape:       apiv1.GraphPrompt_CLUSTER,
		Nodes:       nodes,
		Edges:       edges,
		BlankNodeId: blankID,
	}
}
