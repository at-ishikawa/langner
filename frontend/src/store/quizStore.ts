import { create } from "zustand";

export type QuizType = "standard" | "reverse" | "freeform";

export interface WordDetail {
  origin?: string;
  pronunciation?: string;
  partOfSpeech?: string;
  synonyms?: string[];
  antonyms?: string[];
  memo?: string;
  originParts?: {
    origin: string;
    type: string;
    language: string;
    meaning: string;
    forms?: { form: string; role: string; note?: string }[];
    fromForm?: string;
  }[];
}

interface Example {
  text: string;
  speaker: string;
}

export interface Flashcard {
  noteId: bigint;
  entry: string;
  originalEntry: string;
  examples: Example[];
  // Definitions-concept context. Empty conceptHead means the card is a
  // standalone vocabulary entry; non-empty means it represents a multi-
  // member concept whose member list is conceptMembers and umbrella
  // meaning is conceptMeaning.
  conceptHead?: string;
  conceptMembers?: string[];
  conceptMeaning?: string;
}

export interface ReverseFlashcard {
  noteId: bigint;
  meaning: string;
  contexts: { context: string; maskedContext: string }[];
  notebookName: string;
  storyTitle: string;
  sceneTitle: string;
  conceptHead?: string;
  conceptMembers?: string[];
  conceptMeaning?: string;
}

export interface OriginalValues {
  quality: number;
  status: string;
  intervalDays: number;
}

export interface QuizResult {
  noteId: bigint;
  entry: string;
  answer: string;
  correct: boolean;
  meaning: string;
  reason: string;
  contexts?: string[];
  wordDetail?: WordDetail;
  nextReviewDate?: string;
  learnedAt?: string;
  senseId?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
  images?: string[];
}

export interface ReverseQuizResult {
  noteId: bigint;
  answer: string;
  correct: boolean;
  expression: string;
  meaning: string;
  reason: string;
  contexts?: string[];
  wordDetail?: WordDetail;
  nextReviewDate?: string;
  learnedAt?: string;
  senseId?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
  images?: string[];
}

export interface FreeformResult {
  word: string;
  answer: string;
  correct: boolean;
  meaning: string;
  reason: string;
  notebookName: string;
  contexts?: string[];
  wordDetail?: WordDetail;
  nextReviewDate?: string;
  learnedAt?: string;
  senseId?: string;
  // noteId is required for per-result override/undo/skip on the
  // complete page and on the batch feedback that freeform now uses.
  // The freeform RPC returns it when the typed word matches a card.
  noteId?: bigint;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
  images?: string[];
}

interface QuizState {
  quizType: QuizType;
  flashcards: Flashcard[];
  reverseFlashcards: ReverseFlashcard[];
  currentIndex: number;
  results: QuizResult[];
  reverseResults: ReverseQuizResult[];
  freeformResults: FreeformResult[];
  wordCount: number;
  freeformExpressions: string[];
  freeformNextReviewDates: Record<string, string>;
  feedbackInterval: number;
  setFeedbackInterval: (n: number) => void;
  setQuizType: (type: QuizType) => void;
  setFlashcards: (flashcards: Flashcard[]) => void;
  setReverseFlashcards: (flashcards: ReverseFlashcard[]) => void;
  setWordCount: (count: number) => void;
  setFreeformExpressions: (expressions: string[]) => void;
  setFreeformNextReviewDates: (dates: Record<string, string>) => void;
  recordFreeformAnswered: (word: string, nextReviewDate: string) => void;
  submitResult: (result: QuizResult) => void;
  submitReverseResult: (result: ReverseQuizResult) => void;
  submitFreeformResult: (result: FreeformResult) => void;
  nextCard: () => void;
  reset: () => void;
  overrideResult: (index: number, quizType: QuizType, nextReviewDate: string, originalValues: OriginalValues) => void;
  undoOverrideResult: (index: number, quizType: QuizType, correct: boolean, nextReviewDate: string) => void;
  skipResult: (index: number, quizType: QuizType) => void;
  resumeResult: (index: number, quizType: QuizType) => void;
  updateResultReviewDate: (index: number, quizType: QuizType, newDate: string) => void;
}

const initialState = {
  quizType: "standard" as QuizType,
  flashcards: [] as Flashcard[],
  reverseFlashcards: [] as ReverseFlashcard[],
  currentIndex: 0,
  results: [] as QuizResult[],
  reverseResults: [] as ReverseQuizResult[],
  freeformResults: [] as FreeformResult[],
  wordCount: 0,
  freeformExpressions: [] as string[],
  freeformNextReviewDates: {} as Record<string, string>,
  feedbackInterval: 10,
};

function updateArrayItem<T>(arr: T[], index: number, patch: Partial<T>): T[] {
  return arr.map((item, i) => (i === index ? { ...item, ...patch } : item));
}

export const useQuizStore = create<QuizState>((set) => ({
  ...initialState,
  setFeedbackInterval: (feedbackInterval) => set({ feedbackInterval }),
  setQuizType: (quizType) => set({ quizType }),
  setFlashcards: (flashcards) => set({ flashcards }),
  setReverseFlashcards: (reverseFlashcards) => set({ reverseFlashcards }),
  setWordCount: (wordCount) => set({ wordCount }),
  setFreeformExpressions: (freeformExpressions) => set({ freeformExpressions }),
  setFreeformNextReviewDates: (freeformNextReviewDates) => set({ freeformNextReviewDates }),
  recordFreeformAnswered: (word, nextReviewDate) =>
    set((state) => ({
      freeformNextReviewDates: {
        ...state.freeformNextReviewDates,
        [word.trim().toLowerCase()]: nextReviewDate || new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
      },
    })),
  submitResult: (result) =>
    set((state) => ({ results: [...state.results, result] })),
  submitReverseResult: (result) =>
    set((state) => ({ reverseResults: [...state.reverseResults, result] })),
  submitFreeformResult: (result) =>
    set((state) => ({ freeformResults: [...state.freeformResults, result] })),
  nextCard: () =>
    set((state) => ({ currentIndex: state.currentIndex + 1 })),
  reset: () => set(initialState),

  overrideResult: (index, quizType, nextReviewDate, originalValues) =>
    set((state) => {
      if (quizType === "standard") {
        return { results: updateArrayItem(state.results, index, { correct: !state.results[index].correct, isOverridden: true, nextReviewDate, originalValues }) };
      }
      if (quizType === "reverse") {
        return { reverseResults: updateArrayItem(state.reverseResults, index, { correct: !state.reverseResults[index].correct, isOverridden: true, nextReviewDate, originalValues }) };
      }
      return { freeformResults: updateArrayItem(state.freeformResults, index, { correct: !state.freeformResults[index].correct, isOverridden: true, nextReviewDate, originalValues }) };
    }),

  undoOverrideResult: (index, quizType, correct, nextReviewDate) =>
    set((state) => {
      if (quizType === "standard") {
        return { results: updateArrayItem(state.results, index, { correct, isOverridden: false, nextReviewDate, originalValues: undefined }) };
      }
      if (quizType === "reverse") {
        return { reverseResults: updateArrayItem(state.reverseResults, index, { correct, isOverridden: false, nextReviewDate, originalValues: undefined }) };
      }
      return { freeformResults: updateArrayItem(state.freeformResults, index, { correct, isOverridden: false, nextReviewDate, originalValues: undefined }) };
    }),

  skipResult: (index, quizType) =>
    set((state) => {
      if (quizType === "standard") {
        return { results: updateArrayItem(state.results, index, { isSkipped: true }) };
      }
      if (quizType === "reverse") {
        return { reverseResults: updateArrayItem(state.reverseResults, index, { isSkipped: true }) };
      }
      return { freeformResults: updateArrayItem(state.freeformResults, index, { isSkipped: true }) };
    }),

  resumeResult: (index, quizType) =>
    set((state) => {
      if (quizType === "standard") {
        return { results: updateArrayItem(state.results, index, { isSkipped: false }) };
      }
      if (quizType === "reverse") {
        return { reverseResults: updateArrayItem(state.reverseResults, index, { isSkipped: false }) };
      }
      return { freeformResults: updateArrayItem(state.freeformResults, index, { isSkipped: false }) };
    }),

  updateResultReviewDate: (index, quizType, newDate) =>
    set((state) => {
      if (quizType === "standard") {
        return { results: updateArrayItem(state.results, index, { nextReviewDate: newDate }) };
      }
      if (quizType === "reverse") {
        return { reverseResults: updateArrayItem(state.reverseResults, index, { nextReviewDate: newDate }) };
      }
      return { freeformResults: updateArrayItem(state.freeformResults, index, { nextReviewDate: newDate }) };
    }),

}));
