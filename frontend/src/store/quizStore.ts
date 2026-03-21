import { create } from "zustand";

export type QuizType = "standard" | "reverse" | "freeform" | "etymology-breakdown" | "etymology-assembly" | "etymology-freeform";

export interface WordDetail {
  origin?: string;
  pronunciation?: string;
  partOfSpeech?: string;
  synonyms?: string[];
  antonyms?: string[];
  memo?: string;
}

interface Example {
  text: string;
  speaker: string;
}

export interface Flashcard {
  noteId: bigint;
  entry: string;
  examples: Example[];
}

export interface ReverseFlashcard {
  noteId: bigint;
  meaning: string;
  contexts: { context: string; maskedContext: string }[];
  notebookName: string;
  storyTitle: string;
  sceneTitle: string;
}

export interface EtymologyCard {
  cardId: bigint;
  expression: string;
  meaning: string;
  originParts: EtymologyQuizOrigin[];
  notebookName: string;
}

export interface EtymologyQuizOrigin {
  origin: string;
  type: string;
  language: string;
  meaning: string;
}

export interface OriginalValues {
  quality: number;
  status: string;
  intervalDays: number;
  easinessFactor: number;
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
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
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
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
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
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
}

export interface OriginGrade {
  userOrigin: string;
  userMeaning: string;
  originCorrect: boolean;
  meaningCorrect: boolean;
  correctOrigin?: { origin: string; meaning: string };
}

export interface EtymologyResult {
  noteId?: bigint;
  cardId?: bigint;
  expression: string;
  meaning: string;
  answer: string;
  correct: boolean;
  reason: string;
  originGrades: OriginGrade[];
  relatedDefinitions: { expression: string; meaning: string; notebookName: string }[];
  originParts?: EtymologyQuizOrigin[];
  nextReviewDate?: string;
  learnedAt?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
}

interface QuizState {
  quizType: QuizType;
  flashcards: Flashcard[];
  reverseFlashcards: ReverseFlashcard[];
  etymologyCards: EtymologyCard[];
  currentIndex: number;
  results: QuizResult[];
  reverseResults: ReverseQuizResult[];
  freeformResults: FreeformResult[];
  etymologyResults: EtymologyResult[];
  wordCount: number;
  freeformExpressions: string[];
  freeformNextReviewDates: Record<string, string>;
  etymologyFreeformExpressions: string[];
  etymologyFreeformNextReviewDates: Record<string, string>;
  setQuizType: (type: QuizType) => void;
  setFlashcards: (flashcards: Flashcard[]) => void;
  setReverseFlashcards: (flashcards: ReverseFlashcard[]) => void;
  setEtymologyCards: (cards: EtymologyCard[]) => void;
  setWordCount: (count: number) => void;
  setFreeformExpressions: (expressions: string[]) => void;
  setFreeformNextReviewDates: (dates: Record<string, string>) => void;
  setEtymologyFreeformExpressions: (expressions: string[]) => void;
  setEtymologyFreeformNextReviewDates: (dates: Record<string, string>) => void;
  submitResult: (result: QuizResult) => void;
  submitReverseResult: (result: ReverseQuizResult) => void;
  submitFreeformResult: (result: FreeformResult) => void;
  submitEtymologyResult: (result: EtymologyResult) => void;
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
  etymologyCards: [] as EtymologyCard[],
  currentIndex: 0,
  results: [] as QuizResult[],
  reverseResults: [] as ReverseQuizResult[],
  freeformResults: [] as FreeformResult[],
  etymologyResults: [] as EtymologyResult[],
  wordCount: 0,
  freeformExpressions: [] as string[],
  freeformNextReviewDates: {} as Record<string, string>,
  etymologyFreeformExpressions: [] as string[],
  etymologyFreeformNextReviewDates: {} as Record<string, string>,
};

function updateArrayItem<T>(arr: T[], index: number, patch: Partial<T>): T[] {
  return arr.map((item, i) => (i === index ? { ...item, ...patch } : item));
}

function isEtymologyType(qt: QuizType): boolean {
  return qt === "etymology-breakdown" || qt === "etymology-assembly" || qt === "etymology-freeform";
}

export const useQuizStore = create<QuizState>((set) => ({
  ...initialState,
  setQuizType: (quizType) => set({ quizType }),
  setFlashcards: (flashcards) => set({ flashcards }),
  setReverseFlashcards: (reverseFlashcards) => set({ reverseFlashcards }),
  setEtymologyCards: (etymologyCards) => set({ etymologyCards }),
  setWordCount: (wordCount) => set({ wordCount }),
  setFreeformExpressions: (freeformExpressions) => set({ freeformExpressions }),
  setFreeformNextReviewDates: (freeformNextReviewDates) => set({ freeformNextReviewDates }),
  setEtymologyFreeformExpressions: (etymologyFreeformExpressions) => set({ etymologyFreeformExpressions }),
  setEtymologyFreeformNextReviewDates: (etymologyFreeformNextReviewDates) => set({ etymologyFreeformNextReviewDates }),
  submitResult: (result) =>
    set((state) => ({ results: [...state.results, result] })),
  submitReverseResult: (result) =>
    set((state) => ({ reverseResults: [...state.reverseResults, result] })),
  submitFreeformResult: (result) =>
    set((state) => ({ freeformResults: [...state.freeformResults, result] })),
  submitEtymologyResult: (result) =>
    set((state) => ({ etymologyResults: [...state.etymologyResults, result] })),
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
      if (isEtymologyType(quizType)) {
        return { etymologyResults: updateArrayItem(state.etymologyResults, index, { correct: !state.etymologyResults[index].correct, isOverridden: true, nextReviewDate, originalValues }) };
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
      if (isEtymologyType(quizType)) {
        return { etymologyResults: updateArrayItem(state.etymologyResults, index, { correct, isOverridden: false, nextReviewDate, originalValues: undefined }) };
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
      if (isEtymologyType(quizType)) {
        return { etymologyResults: updateArrayItem(state.etymologyResults, index, { isSkipped: true }) };
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
      if (isEtymologyType(quizType)) {
        return { etymologyResults: updateArrayItem(state.etymologyResults, index, { isSkipped: false }) };
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
      if (isEtymologyType(quizType)) {
        return { etymologyResults: updateArrayItem(state.etymologyResults, index, { nextReviewDate: newDate }) };
      }
      return { freeformResults: updateArrayItem(state.freeformResults, index, { nextReviewDate: newDate }) };
    }),
}));
