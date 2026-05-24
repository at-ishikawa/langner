import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { create } from "@bufbuild/protobuf";
import {
  GraphPromptSchema,
  GraphNodeSchema,
  GraphEdgeSchema,
  GraphPrompt_Shape,
  GraphNode_Kind,
} from "@/gen-protos/api/v1/quiz_pb";
import { RelationGraph } from "./RelationGraph";

function renderWithChakra(ui: React.ReactElement) {
  return render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);
}

// Tests target the mobile-first invariants we want to keep stable as the
// component evolves. Two specific failure modes are pinned: iOS Safari
// auto-zoom on inputs <16px font-size, and the < / > 480px (sm
// breakpoint) layout pivot for ANTONYM_PAIR. The viewport switch is a
// CSS media query — RTL can't simulate viewport sizes — but BOTH text
// variants of the arrow ("⇄" desktop and "⇅" mobile) ship in the DOM and
// CSS picks one. Asserting both render at all catches future refactors
// that drop the responsive markup.

describe("RelationGraph mobile-friendliness", () => {
  it("ANTONYM_PAIR renders both desktop and mobile arrow variants", () => {
    const prompt = create(GraphPromptSchema, {
      shape: GraphPrompt_Shape.ANTONYM_PAIR,
      blankNodeId: "b1",
      nodes: [
        create(GraphNodeSchema, {
          id: "concept_a",
          kind: GraphNode_Kind.CONCEPT,
          label: "rightness",
        }),
        create(GraphNodeSchema, {
          id: "concept_b",
          kind: GraphNode_Kind.CONCEPT,
          label: "leftness",
        }),
        create(GraphNodeSchema, {
          id: "a1",
          kind: GraphNode_Kind.ORIGIN,
          label: "dexter",
          language: "Latin",
        }),
        create(GraphNodeSchema, {
          id: "b1",
          kind: GraphNode_Kind.ORIGIN,
          label: "sinister",
          language: "Latin",
        }),
      ],
      edges: [
        create(GraphEdgeSchema, { type: "antonym", from: "concept_a", to: "concept_b" }),
      ],
    });
    renderWithChakra(
      <RelationGraph prompt={prompt} value="" onValueChange={() => {}} />,
    );
    // Both arrow spans are present; CSS hides one based on viewport.
    // Without both, the mobile-stacked layout would show a horizontal
    // arrow and look misaligned, or vice versa.
    expect(screen.getByText("⇄ antonym")).toBeInTheDocument();
    expect(screen.getByText("⇅ antonym")).toBeInTheDocument();
  });

  it("blank input uses 16px+ font-size to prevent iOS Safari auto-zoom", () => {
    const prompt = create(GraphPromptSchema, {
      shape: GraphPrompt_Shape.CLUSTER,
      blankNodeId: "m1",
      nodes: [
        create(GraphNodeSchema, {
          id: "concept",
          kind: GraphNode_Kind.CONCEPT,
          label: "test",
        }),
        create(GraphNodeSchema, {
          id: "m1",
          kind: GraphNode_Kind.ORIGIN,
          label: "blank",
          language: "Latin",
        }),
      ],
      edges: [],
    });
    renderWithChakra(
      <RelationGraph prompt={prompt} value="" onValueChange={() => {}} />,
    );
    const input = screen.getByPlaceholderText("???");
    // iOS Safari auto-zooms when a focused input's computed font-size
    // is below 16px. Anchor the value so a future "let's use size='sm'
    // everywhere" refactor can't silently re-introduce the auto-zoom
    // regression on iPhone.
    expect(input).toHaveStyle({ fontSize: "16px" });
  });
});
