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
