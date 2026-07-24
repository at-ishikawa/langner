import { create } from "zustand";

// The Grammar Quiz drills grammar mistakes annotated in journal notebooks. It
// is a linear walk over a fixed list of cards (like the standard quiz) but is
// kept in its own store because grammar cards are string-keyed and don't share
// the vocabulary quiz's override/skip machinery.

export interface GrammarCard {
  notebookId: string;
  cardId: string;
  entryId: string;
  sentence: string;
  incorrect: string;
  category: string;
  note: string;
  status: string;
}

export interface GrammarResult {
  cardId: string;
  sentence: string;
  incorrect: string;
  category: string;
  answer: string;
  correct: boolean;
  correctAnswer: string;
  reason: string;
  nextReviewDate: string;
}

interface GrammarState {
  cards: GrammarCard[];
  currentIndex: number;
  results: GrammarResult[];
  seedCards: (cards: GrammarCard[]) => void;
  submitResult: (result: GrammarResult) => void;
  nextCard: () => void;
  reset: () => void;
}

const initialState = {
  cards: [] as GrammarCard[],
  currentIndex: 0,
  results: [] as GrammarResult[],
};

export const useGrammarStore = create<GrammarState>((set) => ({
  ...initialState,
  seedCards: (cards) => set({ cards: [...cards], currentIndex: 0, results: [] }),
  submitResult: (result) => set((state) => ({ results: [...state.results, result] })),
  nextCard: () => set((state) => ({ currentIndex: state.currentIndex + 1 })),
  reset: () => set(initialState),
}));
