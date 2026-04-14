import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { WordDetailView } from "./WordDetailView";
import type { WordDetail } from "@/store/quizStore";

function renderView(wordDetail?: WordDetail | null) {
  return render(
    <ChakraProvider value={defaultSystem}>
      <WordDetailView wordDetail={wordDetail} />
    </ChakraProvider>,
  );
}

describe("WordDetailView", () => {
  describe("renders nothing when", () => {
    it.each([
      { name: "wordDetail is undefined", input: undefined },
      { name: "wordDetail is null", input: null },
      { name: "wordDetail is empty object", input: {} },
      {
        name: "all fields are empty/blank",
        input: {
          origin: "",
          originParts: [],
          synonyms: [],
          antonyms: [],
          memo: "",
        },
      },
      {
        name: "origin is whitespace only",
        input: { origin: "   \n  " },
      },
    ])("$name", ({ input }) => {
      const { container } = renderView(input as WordDetail | null | undefined);
      expect(container.firstChild).toBeNull();
    });
  });

  describe("renders origin prose", () => {
    it("shows origin when provided", () => {
      renderView({
        origin:
          "The idiom originally refers to herbs cut and dried for medicinal use.",
      });
      expect(screen.getByText("Origin")).toBeInTheDocument();
      expect(
        screen.getByText(/herbs cut and dried for medicinal use/),
      ).toBeInTheDocument();
    });

    it("preserves multi-line origin text", () => {
      renderView({ origin: "Line one.\nLine two." });
      // Text is rendered in one element with whiteSpace preserved
      const el = screen.getByText(/Line one/);
      expect(el).toHaveTextContent("Line one. Line two.");
    });

    it("does not show Origin section when origin is empty", () => {
      renderView({ origin: "", synonyms: ["a"] });
      expect(screen.queryByText("Origin")).not.toBeInTheDocument();
    });
  });

  describe("renders origin parts (etymology)", () => {
    it("shows single origin part", () => {
      renderView({
        originParts: [
          { origin: "spect", type: "root", language: "Latin", meaning: "to see" },
        ],
      });
      expect(screen.getByText("Etymology")).toBeInTheDocument();
      expect(screen.getByText("spect")).toBeInTheDocument();
      expect(screen.getByText("(to see)")).toBeInTheDocument();
      expect(screen.getByText("Latin")).toBeInTheDocument();
    });

    it("shows multiple origin parts with plus separator", () => {
      renderView({
        originParts: [
          { origin: "com", type: "prefix", language: "Latin", meaning: "together" },
          { origin: "mence", type: "root", language: "Latin", meaning: "to begin" },
        ],
      });
      expect(screen.getByText("com")).toBeInTheDocument();
      expect(screen.getByText("mence")).toBeInTheDocument();
      expect(screen.getByText("+")).toBeInTheDocument();
    });

    it("omits language badge when language is empty", () => {
      renderView({
        originParts: [
          { origin: "root1", type: "root", language: "", meaning: "meaning1" },
        ],
      });
      expect(screen.getByText("root1")).toBeInTheDocument();
      expect(screen.getByText("(meaning1)")).toBeInTheDocument();
    });

    it("does not show Etymology section when originParts is empty", () => {
      renderView({ originParts: [], synonyms: ["x"] });
      expect(screen.queryByText("Etymology")).not.toBeInTheDocument();
    });
  });

  describe("renders synonyms", () => {
    it("shows single synonym", () => {
      renderView({ synonyms: ["happy"] });
      expect(screen.getByText("Synonyms")).toBeInTheDocument();
      expect(screen.getByText("happy")).toBeInTheDocument();
    });

    it("joins multiple synonyms with comma", () => {
      renderView({ synonyms: ["happy", "joyful", "pleased"] });
      expect(screen.getByText("happy, joyful, pleased")).toBeInTheDocument();
    });

    it("does not show Synonyms section when empty", () => {
      renderView({ synonyms: [], antonyms: ["a"] });
      expect(screen.queryByText("Synonyms")).not.toBeInTheDocument();
    });
  });

  describe("renders antonyms", () => {
    it("shows single antonym", () => {
      renderView({ antonyms: ["sad"] });
      expect(screen.getByText("Antonyms")).toBeInTheDocument();
      expect(screen.getByText("sad")).toBeInTheDocument();
    });

    it("joins multiple antonyms with comma", () => {
      renderView({ antonyms: ["sad", "unhappy"] });
      expect(screen.getByText("sad, unhappy")).toBeInTheDocument();
    });

    it("does not show Antonyms section when empty", () => {
      renderView({ antonyms: [], synonyms: ["a"] });
      expect(screen.queryByText("Antonyms")).not.toBeInTheDocument();
    });
  });

  describe("renders memo", () => {
    it("shows memo with Note label", () => {
      renderView({ memo: "Common in business contexts" });
      expect(screen.getByText("Note")).toBeInTheDocument();
      expect(
        screen.getByText("Common in business contexts"),
      ).toBeInTheDocument();
    });

    it("preserves multi-line memo", () => {
      renderView({ memo: "Point 1.\nPoint 2." });
      const el = screen.getByText(/Point 1/);
      expect(el).toHaveTextContent("Point 1. Point 2.");
    });

    it("does not show Note section when memo is whitespace only", () => {
      renderView({ memo: "  \n  ", synonyms: ["a"] });
      expect(screen.queryByText("Note")).not.toBeInTheDocument();
    });
  });

  describe("combined fields", () => {
    it("renders all fields together", () => {
      renderView({
        origin: "Old origin text",
        originParts: [
          { origin: "root", type: "root", language: "Latin", meaning: "base" },
        ],
        synonyms: ["s1"],
        antonyms: ["a1"],
        memo: "memo text",
      });
      expect(screen.getByText("Origin")).toBeInTheDocument();
      expect(screen.getByText("Etymology")).toBeInTheDocument();
      expect(screen.getByText("Synonyms")).toBeInTheDocument();
      expect(screen.getByText("Antonyms")).toBeInTheDocument();
      expect(screen.getByText("Note")).toBeInTheDocument();
    });

    it("renders only populated sections", () => {
      renderView({
        origin: "only origin",
        synonyms: [],
        antonyms: [],
      });
      expect(screen.getByText("Origin")).toBeInTheDocument();
      expect(screen.queryByText("Etymology")).not.toBeInTheDocument();
      expect(screen.queryByText("Synonyms")).not.toBeInTheDocument();
      expect(screen.queryByText("Antonyms")).not.toBeInTheDocument();
      expect(screen.queryByText("Note")).not.toBeInTheDocument();
    });
  });

  describe("real-world patterns", () => {
    it("idiom with origin prose but no origin parts (e.g. cut-and-dried)", () => {
      renderView({
        origin:
          "The idiom originally refers to herbs or plants that were cut and dried for medicinal or commercial use.",
      });
      expect(screen.getByText("Origin")).toBeInTheDocument();
      expect(screen.queryByText("Etymology")).not.toBeInTheDocument();
    });

    it("latin-root word with origin parts but no prose", () => {
      renderView({
        originParts: [
          { origin: "com", type: "prefix", language: "Latin", meaning: "together" },
          { origin: "mence", type: "root", language: "Latin", meaning: "to begin" },
        ],
      });
      expect(screen.getByText("Etymology")).toBeInTheDocument();
      expect(screen.queryByText("Origin")).not.toBeInTheDocument();
    });

    it("basic vocab word (no etymology info, just synonyms)", () => {
      renderView({
        synonyms: ["large", "huge", "enormous"],
        antonyms: ["small", "tiny"],
      });
      expect(screen.getByText("Synonyms")).toBeInTheDocument();
      expect(screen.getByText("Antonyms")).toBeInTheDocument();
      expect(screen.queryByText("Origin")).not.toBeInTheDocument();
      expect(screen.queryByText("Etymology")).not.toBeInTheDocument();
    });
  });
});
