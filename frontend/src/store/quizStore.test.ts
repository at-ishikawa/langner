import { describe, it, expect, beforeEach } from "vitest";
import { useQuizStore } from "./quizStore";
import type { Flashcard, QuizResult, EtymologyOriginCard, EtymologyOriginResult } from "./quizStore";

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

const mockEtymologyCards: EtymologyOriginCard[] = [
  {
    cardId: BigInt(10),
    origin: "bio",
    type: "root",
    language: "Greek",
    meaning: "life",
    notebookName: "Science Roots",
    sessionTitle: "Lesson 1",
    exampleWords: ["biology", "biopsy"],
  },
];

const mockEtymologyResult: EtymologyOriginResult = {
  noteId: BigInt(10),
  cardId: BigInt(10),
  origin: "bio",
  answer: "life",
  correct: true,
  reason: "Correct meaning identified",
  correctAnswer: "life",
  type: "root",
  language: "Greek",
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
    expect(state.feedbackInterval).toBe(10);
  });

  it("setFeedbackInterval updates feedbackInterval", () => {
    useQuizStore.getState().setFeedbackInterval(5);
    expect(useQuizStore.getState().feedbackInterval).toBe(5);
  });

  it("reset restores feedbackInterval to default", () => {
    useQuizStore.getState().setFeedbackInterval(3);
    useQuizStore.getState().reset();
    expect(useQuizStore.getState().feedbackInterval).toBe(10);
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
    });

    const state = useQuizStore.getState();
    expect(state.results[0].correct).toBe(false);
    expect(state.results[0].isOverridden).toBe(true);
    expect(state.results[0].nextReviewDate).toBe("2027-06-20");
    expect(state.results[0].originalValues).toEqual({
      quality: 5,
      status: "understood",
      intervalDays: 10,
    });
  });

  it("undoOverrideResult restores correct and clears isOverridden", () => {
    useQuizStore.getState().submitResult(mockResult);

    // First override
    useQuizStore.getState().overrideResult(0, "standard", "2027-06-20", {
      quality: 5,
      status: "understood",
      intervalDays: 10,
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

  // Etymology quiz store tests
  it("setEtymologyOriginCards updates etymologyCards", () => {
    useQuizStore.getState().setEtymologyOriginCards(mockEtymologyCards);
    expect(useQuizStore.getState().etymologyOriginCards).toEqual(mockEtymologyCards);
  });

  it("submitEtymologyOriginResult appends result", () => {
    useQuizStore.getState().submitEtymologyOriginResult(mockEtymologyResult);
    const state = useQuizStore.getState();
    expect(state.etymologyOriginResults).toHaveLength(1);
    expect(state.etymologyOriginResults[0].origin).toBe("bio");
  });

  it("overrideResult works for etymology-standard type", () => {
    useQuizStore.getState().submitEtymologyOriginResult(mockEtymologyResult);

    useQuizStore.getState().overrideResult(0, "etymology-standard", "2027-06-20", {
      quality: 5,
      status: "understood",
      intervalDays: 10,
    });

    const state = useQuizStore.getState();
    expect(state.etymologyOriginResults[0].correct).toBe(false);
    expect(state.etymologyOriginResults[0].isOverridden).toBe(true);
  });

  it("skipResult works for etymology-reverse type", () => {
    useQuizStore.getState().submitEtymologyOriginResult(mockEtymologyResult);

    useQuizStore.getState().skipResult(0, "etymology-reverse");

    const state = useQuizStore.getState();
    expect(state.etymologyOriginResults[0].isSkipped).toBe(true);
  });

  it("resumeResult clears isSkipped for etymology type", () => {
    useQuizStore.getState().submitEtymologyOriginResult(mockEtymologyResult);
    useQuizStore.getState().skipResult(0, "etymology-standard");
    useQuizStore.getState().resumeResult(0, "etymology-standard");

    const state = useQuizStore.getState();
    expect(state.etymologyOriginResults[0].isSkipped).toBe(false);
  });

  it("updateResultReviewDate works for etymology type", () => {
    useQuizStore.getState().submitEtymologyOriginResult(mockEtymologyResult);

    useQuizStore.getState().updateResultReviewDate(0, "etymology-standard", "2027-12-25");

    const state = useQuizStore.getState();
    expect(state.etymologyOriginResults[0].nextReviewDate).toBe("2027-12-25");
  });

  it("reset clears etymology state", () => {
    useQuizStore.getState().setEtymologyOriginCards(mockEtymologyCards);
    useQuizStore.getState().submitEtymologyOriginResult(mockEtymologyResult);

    useQuizStore.getState().reset();

    const state = useQuizStore.getState();
    expect(state.etymologyOriginCards).toEqual([]);
    expect(state.etymologyOriginResults).toEqual([]);
  });

  it("setEtymologyFreeformOrigins updates state", () => {
    useQuizStore.getState().setEtymologyFreeformOrigins(["biology", "geology"]);
    expect(useQuizStore.getState().etymologyFreeformOrigins).toEqual(["biology", "geology"]);
  });

  it("setEtymologyFreeformNextReviewDates updates state", () => {
    const dates = { biology: "2027-06-20" };
    useQuizStore.getState().setEtymologyFreeformNextReviewDates(dates);
    expect(useQuizStore.getState().etymologyFreeformNextReviewDates).toEqual(dates);
  });
});
