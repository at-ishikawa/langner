import { describe, it, expect, beforeEach } from "vitest";
import { useQuizStore } from "./quizStore";
import type { Flashcard, QuizResult } from "./quizStore";

describe("quizStore", () => {
  beforeEach(() => {
    useQuizStore.getState().reset();
  });

  it("starts with empty state", () => {
    const state = useQuizStore.getState();
    expect(state.flashcards).toEqual([]);
    expect(state.currentIndex).toBe(0);
    expect(state.results).toEqual([]);
  });

  it("sets flashcards and resets index and results", () => {
    const flashcards: Flashcard[] = [
      { noteId: BigInt(1), entry: "hello", examples: [] },
      { noteId: BigInt(2), entry: "world", examples: [] },
    ];

    useQuizStore.getState().setFlashcards(flashcards);

    const state = useQuizStore.getState();
    expect(state.flashcards).toEqual(flashcards);
    expect(state.currentIndex).toBe(0);
    expect(state.results).toEqual([]);
  });

  it("advances to the next card", () => {
    const flashcards: Flashcard[] = [
      { noteId: BigInt(1), entry: "hello", examples: [] },
      { noteId: BigInt(2), entry: "world", examples: [] },
    ];

    useQuizStore.getState().setFlashcards(flashcards);
    useQuizStore.getState().nextCard();

    expect(useQuizStore.getState().currentIndex).toBe(1);
  });

  it("submits a result", () => {
    const result: QuizResult = {
      noteId: BigInt(1),
      entry: "hello",
      answer: "greeting",
      correct: true,
      meaning: "a greeting",
      reason: "correct meaning",
    };

    useQuizStore.getState().submitResult(result);

    const state = useQuizStore.getState();
    expect(state.results).toHaveLength(1);
    expect(state.results[0]).toEqual(result);
  });

  it("resets state", () => {
    const flashcards: Flashcard[] = [
      { noteId: BigInt(1), entry: "hello", examples: [] },
    ];

    useQuizStore.getState().setFlashcards(flashcards);
    useQuizStore.getState().nextCard();
    useQuizStore.getState().submitResult({
      noteId: BigInt(1),
      entry: "hello",
      answer: "greeting",
      correct: true,
      meaning: "a greeting",
      reason: "correct meaning",
    });

    useQuizStore.getState().reset();

    const state = useQuizStore.getState();
    expect(state.flashcards).toEqual([]);
    expect(state.currentIndex).toBe(0);
    expect(state.results).toEqual([]);
  });
});
