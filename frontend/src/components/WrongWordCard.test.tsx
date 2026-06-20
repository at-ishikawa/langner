import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { WrongWordCard } from "./WrongWordCard";
import type { WrongWord } from "@/lib/client";

function baseWord(overrides: Partial<WrongWord> = {}): WrongWord {
  return {
    $typeName: "api.v1.WrongWord",
    noteId: BigInt(0),
    expression: "ephemeral",
    notebookId: "demo",
    notebookTitle: "Demo Notebook",
    sceneTitle: "",
    quizType: "notebook",
    recentPattern: [],
    currentWrongStreak: 1,
    previousCorrectStreak: 0,
    currentStatus: "misunderstood",
    meaning: "",
    exampleSentence: "",
    notebookKind: "flashcard",
    skipped: false,
    relatedGroups: [],
    ...overrides,
  } as WrongWord;
}

function renderCard(word: WrongWord) {
  return render(
    <ChakraProvider value={defaultSystem}>
      <WrongWordCard word={word} />
    </ChakraProvider>,
  );
}

describe("WrongWordCard", () => {
  it("renders an Excluded badge when the word is currently skipped", () => {
    renderCard(baseWord({ skipped: true }));
    expect(screen.getByTestId("excluded-badge")).toHaveTextContent("Excluded");
  });

  it("omits the Excluded badge when the word is not skipped", () => {
    renderCard(baseWord({ skipped: false }));
    expect(screen.queryByTestId("excluded-badge")).toBeNull();
  });

  it("renders chip preview for related groups when collapsed", () => {
    renderCard(
      baseWord({
        relatedGroups: [
          {
            $typeName: "api.v1.RelatedGroup",
            kind: "concept",
            label: "clumsy, tactless behavior, especially in social situations",
            members: ["gaucherie"],
          },
          {
            $typeName: "api.v1.RelatedGroup",
            kind: "antonym",
            label: "rightness — right",
            members: ["dexter (Latin) — right hand", "droit (French) — right hand"],
          },
          {
            $typeName: "api.v1.RelatedGroup",
            kind: "origin_family",
            label: "leftness — left",
            members: ["sinister (Latin) — left hand"],
          },
        ] as unknown as WrongWord["relatedGroups"],
      }),
    );
    const preview = screen.getByTestId("related-chip-preview");
    // Card stays collapsed → only 2 chips render so the row stays compact.
    expect(preview.children.length).toBe(2);
    expect(screen.getByTestId("related-chip-concept")).toHaveTextContent("same sense: gaucherie");
    expect(screen.getByTestId("related-chip-antonym")).toHaveTextContent(
      "antonym: dexter (Latin) — right hand, droit (French) — right hand",
    );
    // Expanded-only block must not be visible yet.
    expect(screen.queryByTestId("related-groups-expanded")).toBeNull();
  });

  it("expands the related-groups section when the card opens", async () => {
    const { rerender } = renderCard(
      baseWord({
        relatedGroups: [
          {
            $typeName: "api.v1.RelatedGroup",
            kind: "concept",
            label: "clumsy, tactless behavior, especially in social situations",
            members: ["gaucherie"],
          },
          {
            $typeName: "api.v1.RelatedGroup",
            kind: "antonym",
            label: "rightness — right",
            members: ["dexter (Latin) — right hand", "droit (French) — right hand"],
          },
        ] as unknown as WrongWord["relatedGroups"],
      }),
    );
    const card = screen.getByTestId("wrong-word-ephemeral-notebook");
    const button = card.querySelector("button[aria-expanded]");
    expect(button).not.toBeNull();
    button!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    rerender(
      <ChakraProvider value={defaultSystem}>
        <WrongWordCard
          word={baseWord({
            relatedGroups: [
              {
                $typeName: "api.v1.RelatedGroup",
                kind: "concept",
                label: "clumsy, tactless behavior, especially in social situations",
                members: ["gaucherie"],
              },
              {
                $typeName: "api.v1.RelatedGroup",
                kind: "antonym",
                label: "rightness — right",
                members: ["dexter (Latin) — right hand", "droit (French) — right hand"],
              },
            ] as unknown as WrongWord["relatedGroups"],
          })}
        />
      </ChakraProvider>,
    );
    const expanded = await screen.findByTestId("related-groups-expanded");
    expect(expanded).toHaveTextContent("Related words");
    const concept = screen.getByTestId("related-group-concept");
    expect(concept).toHaveTextContent("Same sense");
    expect(concept).toHaveTextContent("gaucherie");
    const antonym = screen.getByTestId("related-group-antonym");
    expect(antonym).toHaveTextContent("antonym");
    expect(antonym).toHaveTextContent("dexter (Latin) — right hand · droit (French) — right hand");
  });

  it("omits both chip preview and expanded block when relatedGroups is empty", () => {
    renderCard(baseWord({ relatedGroups: [] }));
    expect(screen.queryByTestId("related-chip-preview")).toBeNull();
    expect(screen.queryByTestId("related-groups-expanded")).toBeNull();
  });

  it("collapses a multi-paragraph scene title to a single short line", () => {
    // Story-style notebooks (Speak English Like an American etc.) declare
    // the scene title as the lesson's multi-paragraph plot summary. The
    // raw value was rendering as a wall of text under the expression and
    // the user reported it as a wrong "example sentence". The breadcrumb
    // must collapse it.
    const longSummary =
      "Mark, Ron, and Steve work at Gourmet International, a small food company specializing in ethnic frozen foods.\n" +
      "Mark, a marketing manager at the company, tells his boss Ron and his co-worker Steve that Grand Foods, a large food company, is going to start competing with them in the frozen Chinese meals market.\n" +
      "Mark got this information from an ex-girlfriend.";
    renderCard(
      baseWord({
        notebookTitle: "LESSON 1: GOURMET INTERNATIONAL GETS NEW COMPETITION",
        sceneTitle: longSummary,
      }),
    );
    const crumb = screen.getByTestId("wrong-word-breadcrumb");
    expect(crumb.textContent ?? "").not.toContain("Mark, a marketing manager");
    expect(crumb.textContent ?? "").not.toContain("\n");
    expect((crumb.textContent ?? "").length).toBeLessThanOrEqual(180);
    expect(crumb.textContent).toContain("LESSON 1");
  });
});
