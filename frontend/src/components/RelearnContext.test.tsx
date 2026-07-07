import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import RelearnContext, { highlightEntry } from "./RelearnContext";
import type { RelearnContextScene } from "@/lib/client";
import type { GraphPrompt } from "@/gen-protos/api/v1/quiz_pb";

vi.mock("./RelationGraph", () => ({
  RelationGraph: () => <div data-testid="relation-graph" />,
}));

function renderCtx(props: Parameters<typeof RelearnContext>[0]) {
  return render(
    <ChakraProvider value={defaultSystem}>
      <RelearnContext {...props} />
    </ChakraProvider>,
  );
}

const scene = (over: Partial<RelearnContextScene> = {}): RelearnContextScene =>
  ({ notebookName: "nb", sceneTitle: "", statements: [], conversations: [], ...over }) as RelearnContextScene;

describe("highlightEntry", () => {
  it("bolds case-insensitive occurrences of the entry", () => {
    const nodes = highlightEntry("The Ice was broken", "ice");
    // Splitting keeps the matched token as its own node.
    const text = nodes.map((n) => (typeof n === "string" ? n : "")).join("");
    expect(text).toBe(""); // all wrapped in <Text as="span">
    expect(nodes.length).toBeGreaterThan(1);
  });

  it("returns the text unchanged when the entry is blank", () => {
    expect(highlightEntry("hello", "  ")).toEqual(["hello"]);
  });
});

describe("RelearnContext", () => {
  it("renders nothing when there is no context at all", () => {
    const { container } = renderCtx({ entry: "x", scenes: [], exampleWords: [] });
    expect(container).toBeEmptyDOMElement();
  });

  it("renders statements and conversations under 'Where it appears'", () => {
    renderCtx({
      entry: "ice",
      scenes: [scene({ sceneTitle: "Scene 1", statements: ["Break the ice quickly."], conversations: [{ speaker: "Amy", quote: "The ice cracked." } as RelearnContextScene["conversations"][number]] })],
      exampleWords: [],
    });
    expect(screen.getByText("Where it appears")).toBeInTheDocument();
    expect(screen.getByText("Scene 1")).toBeInTheDocument();
    expect(screen.getByText("Amy:")).toBeInTheDocument();
  });

  it("renders related words chips", () => {
    renderCtx({ entry: "x", scenes: [], exampleWords: ["alpha", "beta"] });
    expect(screen.getByText("Related words")).toBeInTheDocument();
    expect(screen.getByText("alpha")).toBeInTheDocument();
    expect(screen.getByText("beta")).toBeInTheDocument();
  });

  it("renders the relation graph when graphContext is present", () => {
    renderCtx({ entry: "x", scenes: [], exampleWords: [], graphContext: {} as GraphPrompt });
    expect(screen.getByText("Origin")).toBeInTheDocument();
    expect(screen.getByTestId("relation-graph")).toBeInTheDocument();
  });
});
