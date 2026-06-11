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
});
