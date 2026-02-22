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

interface QuizState {
  flashcards: Flashcard[];
  setFlashcards: (flashcards: Flashcard[]) => void;
  reset: () => void;
}

const initialState = {
  flashcards: [] as Flashcard[],
};

export const useQuizStore = create<QuizState>((set) => ({
  ...initialState,
  setFlashcards: (flashcards) => set({ flashcards }),
  reset: () => set(initialState),
}));
