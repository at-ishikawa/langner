import { create } from "zustand";

export interface Example {
  text: string;
  speaker: string;
}

export interface Flashcard {
  noteId: bigint;
  entry: string;
  examples: Example[];
}

export interface QuizResult {
  noteId: bigint;
  entry: string;
  answer: string;
  correct: boolean;
  meaning: string;
  reason: string;
}

interface QuizState {
  flashcards: Flashcard[];
  currentIndex: number;
  results: QuizResult[];

  setFlashcards: (flashcards: Flashcard[]) => void;
  submitResult: (result: QuizResult) => void;
  nextCard: () => void;
  reset: () => void;
}

export const useQuizStore = create<QuizState>((set) => ({
  flashcards: [],
  currentIndex: 0,
  results: [],

  setFlashcards: (flashcards) => set({ flashcards, currentIndex: 0, results: [] }),

  submitResult: (result) =>
    set((state) => ({ results: [...state.results, result] })),

  nextCard: () =>
    set((state) => ({ currentIndex: state.currentIndex + 1 })),

  reset: () => set({ flashcards: [], currentIndex: 0, results: [] }),
}));
