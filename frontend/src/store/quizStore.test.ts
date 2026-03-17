import { describe, it, expect, beforeEach } from "vitest";
import { useQuizStore } from "./quizStore";
import type { Flashcard, QuizResult } from "./quizStore";

const mockFlashcards: Flashcard[] = [
  {
    noteId: BigInt(1),
    entry: "break the ice",
    examples: [{ text: "She told a joke to break the ice.", speaker: "Rachel" }],
  },
  {
    noteId: BigInt(2),
    entry: "lose one's temper",
    examples: [],
  },
];

const mockResult: QuizResult = {
  noteId: BigInt(1),
  entry: "break the ice",
  answer: "start a conversation",
  correct: true,
  meaning: "to initiate social interaction",
  reason: "The answer captures the core meaning",
};

describe("useQuizStore", () => {
  beforeEach(() => {
    useQuizStore.getState().reset();
  });

  it("has correct initial state", () => {
    const state = useQuizStore.getState();
    expect(state.flashcards).toEqual([]);
    expect(state.currentIndex).toBe(0);
    expect(state.results).toEqual([]);
  });

  it("setFlashcards updates flashcards", () => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    expect(useQuizStore.getState().flashcards).toEqual(mockFlashcards);
  });

  it("submitResult appends result to results", () => {
    useQuizStore.getState().submitResult(mockResult);
    const state = useQuizStore.getState();
    expect(state.results).toHaveLength(1);
    expect(state.results[0]).toEqual(mockResult);
  });

  it("submitResult accumulates multiple results", () => {
    const secondResult: QuizResult = {
      ...mockResult,
      noteId: BigInt(2),
      entry: "lose one's temper",
      correct: false,
    };

    useQuizStore.getState().submitResult(mockResult);
    useQuizStore.getState().submitResult(secondResult);

    const state = useQuizStore.getState();
    expect(state.results).toHaveLength(2);
    expect(state.results[0].correct).toBe(true);
    expect(state.results[1].correct).toBe(false);
  });

  it("nextCard increments currentIndex", () => {
    useQuizStore.getState().nextCard();
    expect(useQuizStore.getState().currentIndex).toBe(1);

    useQuizStore.getState().nextCard();
    expect(useQuizStore.getState().currentIndex).toBe(2);
  });

  it("overrideResult flips correct and sets isOverridden", () => {
    useQuizStore.getState().submitResult(mockResult);

    useQuizStore.getState().overrideResult(0, "standard", "2027-06-20", {
      quality: 5,
      status: "understood",
      intervalDays: 10,
      easinessFactor: 2.5,
    });

    const state = useQuizStore.getState();
    expect(state.results[0].correct).toBe(false);
    expect(state.results[0].isOverridden).toBe(true);
    expect(state.results[0].nextReviewDate).toBe("2027-06-20");
    expect(state.results[0].originalValues).toEqual({
      quality: 5,
      status: "understood",
      intervalDays: 10,
      easinessFactor: 2.5,
    });
  });

  it("undoOverrideResult restores correct and clears isOverridden", () => {
    useQuizStore.getState().submitResult(mockResult);

    // First override
    useQuizStore.getState().overrideResult(0, "standard", "2027-06-20", {
      quality: 5,
      status: "understood",
      intervalDays: 10,
      easinessFactor: 2.5,
    });

    // Then undo
    useQuizStore.getState().undoOverrideResult(0, "standard", true, "2027-06-15");

    const state = useQuizStore.getState();
    expect(state.results[0].correct).toBe(true);
    expect(state.results[0].isOverridden).toBe(false);
    expect(state.results[0].nextReviewDate).toBe("2027-06-15");
    expect(state.results[0].originalValues).toBeUndefined();
  });

  it("skipResult sets isSkipped", () => {
    useQuizStore.getState().submitResult(mockResult);

    useQuizStore.getState().skipResult(0, "standard");

    const state = useQuizStore.getState();
    expect(state.results[0].isSkipped).toBe(true);
  });

  it("updateResultReviewDate updates nextReviewDate", () => {
    useQuizStore.getState().submitResult(mockResult);

    useQuizStore.getState().updateResultReviewDate(0, "standard", "2027-12-25");

    const state = useQuizStore.getState();
    expect(state.results[0].nextReviewDate).toBe("2027-12-25");
  });

  it("reset clears all fields to initial state", () => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    useQuizStore.getState().submitResult(mockResult);
    useQuizStore.getState().nextCard();

    useQuizStore.getState().reset();

    const state = useQuizStore.getState();
    expect(state.flashcards).toEqual([]);
    expect(state.currentIndex).toBe(0);
    expect(state.results).toEqual([]);
  });
});
