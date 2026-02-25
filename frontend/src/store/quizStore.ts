import { create } from "zustand";

interface Example {
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

const initialState = {
  flashcards: [] as Flashcard[],
  currentIndex: 0,
  results: [] as QuizResult[],
};

export const useQuizStore = create<QuizState>((set) => ({
  ...initialState,
  setFlashcards: (flashcards) => set({ flashcards }),
  submitResult: (result) =>
    set((state) => ({ results: [...state.results, result] })),
  nextCard: () =>
    set((state) => ({ currentIndex: state.currentIndex + 1 })),
  reset: () => set(initialState),
}));
