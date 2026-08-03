import { create } from "zustand";

export type QuizType = "standard" | "reverse" | "freeform" | "etymology-origin";

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

export interface EtymologyOriginForm {
  form: string;
  role: string;
  note?: string;
}

export interface EtymologyFamilyWord {
  wordId: bigint;
  expression: string;
  // pronunciation is per-word study context shown as a hint while answering.
  // Example sentences and the literal gloss are NOT carried on the card (they
  // would leak the meaning); they arrive on the feedback result instead.
  pronunciation?: string;
}

// EtymologyOriginCard is one quiz screen: a single origin (e.g. Latin
// `facio, facere, feci, factum`) shown with the full session-scoped family
// of words that derive from it. The user types each family word's meaning
// while the origin and its principal parts stay visible as context.
export interface EtymologyOriginCard {
  cardId: bigint;
  origin: string;
  type: string;
  language: string;
  // meaning is the origin's own English gloss, shown as header context.
  meaning: string;
  notebookName: string;
  sessionTitle: string;
  // sense disambiguates same-session multi-sense origins. Empty for the
  // vast majority of cards.
  sense: string;
  // forms are the origin's principal parts (e.g. facio / facere / feci /
  // factum). Rendered as context under the origin header.
  forms: EtymologyOriginForm[];
  // englishForms are the English combining-form spellings the origin surfaces
  // as inside English words (e.g. fac, fic, fect). Shown in the origin header
  // as study context; distinct from `forms` (source-language principal parts).
  englishForms?: string[];
  // note is the origin's free-text pedagogical hint. Study context.
  note?: string;
  // words is the full session-scoped word family the user gives meanings for.
  words: EtymologyFamilyWord[];
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

// EtymologyWordResult is the per-word feedback for one family word. The
// origin still carries a single learning-log series (correct/incorrect and
// excluded are overridable in place within that ONE record — see
// OverrideEtymologyWordResult on the backend — never a second series per
// word; invariants L1/L4).
export interface EtymologyWordResult {
  wordId?: bigint;
  expression: string;
  correct: boolean;
  correctMeaning: string;
  reason: string;
  // userAnswer is what the learner typed for this word (kept for the
  // feedback card's per-word answer chip). Empty on skipped screens.
  userAnswer: string;
  // originalCorrect is the as-graded value, captured once when the result is
  // first stored and never mutated afterward. Comparing it to `correct`
  // tells the feedback card whether this word has been overridden —
  // mirroring the origin-level originalValues pattern (invariant L3:
  // what's displayed must match what's stored).
  originalCorrect?: boolean;
  // isExcluded marks the word as excluded via the feedback card's per-word
  // Exclude action. Display-only annotation on the origin's one record.
  isExcluded?: boolean;
  // pronunciation, examples, and literal are per-word study context revealed
  // on the feedback screen alongside the graded meaning (examples and literal
  // are held back from the card so they don't leak the answer while typing).
  pronunciation?: string;
  examples?: string[];
  literal?: string;
}

// EtymologyOriginResult is the graded result of one origin screen. The origin
// is graded and tracked as ONE result: `correct` is the aggregate (true only
// when every family word was correct) and drives the single learning-log
// entry + override/skip machinery.
export interface EtymologyOriginResult {
  noteId?: bigint;
  cardId?: bigint;
  origin: string;
  // meaning is the origin's English gloss, shown as context on the card.
  meaning: string;
  type: string;
  language: string;
  // forms are the origin's principal parts, echoed from the card so the
  // feedback surface can still show them.
  forms?: EtymologyOriginForm[];
  // englishForms and note are the origin's study context, echoed from the
  // card so the feedback surface can show them next to the graded origin.
  englishForms?: string[];
  note?: string;
  // correct is the aggregate origin result.
  correct: boolean;
  // words holds the per-word results shown on the feedback card.
  words: EtymologyWordResult[];
  notebookName?: string;
  nextReviewDate?: string;
  learnedAt?: string;
  senseId?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
}

interface QuizState {
  quizType: QuizType;
  flashcards: Flashcard[];
  reverseFlashcards: ReverseFlashcard[];
  etymologyOriginCards: EtymologyOriginCard[];
  currentIndex: number;
  results: QuizResult[];
  reverseResults: ReverseQuizResult[];
  freeformResults: FreeformResult[];
  etymologyOriginResults: EtymologyOriginResult[];
  wordCount: number;
  freeformExpressions: string[];
  freeformNextReviewDates: Record<string, string>;
  feedbackInterval: number;
  setFeedbackInterval: (n: number) => void;
  setQuizType: (type: QuizType) => void;
  setFlashcards: (flashcards: Flashcard[]) => void;
  setReverseFlashcards: (flashcards: ReverseFlashcard[]) => void;
  setEtymologyOriginCards: (cards: EtymologyOriginCard[]) => void;
  setWordCount: (count: number) => void;
  setFreeformExpressions: (expressions: string[]) => void;
  setFreeformNextReviewDates: (dates: Record<string, string>) => void;
  recordFreeformAnswered: (word: string, nextReviewDate: string) => void;
  submitResult: (result: QuizResult) => void;
  submitReverseResult: (result: ReverseQuizResult) => void;
  submitFreeformResult: (result: FreeformResult) => void;
  submitEtymologyOriginResult: (result: EtymologyOriginResult) => void;
  nextCard: () => void;
  reset: () => void;
  overrideResult: (index: number, quizType: QuizType, nextReviewDate: string, originalValues: OriginalValues) => void;
  undoOverrideResult: (index: number, quizType: QuizType, correct: boolean, nextReviewDate: string) => void;
  skipResult: (index: number, quizType: QuizType) => void;
  resumeResult: (index: number, quizType: QuizType) => void;
  updateResultReviewDate: (index: number, quizType: QuizType, newDate: string) => void;
  // overrideEtymologyWord / excludeEtymologyWord flip ONE derived family
  // word's correct/excluded flag within the origin's existing result — never
  // by appending a new result (invariants L1/L4). wordExpression identifies
  // the word within etymologyOriginResults[index].words.
  overrideEtymologyWord: (index: number, wordExpression: string, correct: boolean) => void;
  excludeEtymologyWord: (index: number, wordExpression: string, excluded: boolean) => void;
}

const initialState = {
  quizType: "standard" as QuizType,
  flashcards: [] as Flashcard[],
  reverseFlashcards: [] as ReverseFlashcard[],
  etymologyOriginCards: [] as EtymologyOriginCard[],
  currentIndex: 0,
  results: [] as QuizResult[],
  reverseResults: [] as ReverseQuizResult[],
  freeformResults: [] as FreeformResult[],
  etymologyOriginResults: [] as EtymologyOriginResult[],
  wordCount: 0,
  freeformExpressions: [] as string[],
  freeformNextReviewDates: {} as Record<string, string>,
  feedbackInterval: 10,
};

function updateArrayItem<T>(arr: T[], index: number, patch: Partial<T>): T[] {
  return arr.map((item, i) => (i === index ? { ...item, ...patch } : item));
}

function isEtymologyType(qt: QuizType): boolean {
  return qt === "etymology-origin";
}

export const useQuizStore = create<QuizState>((set) => ({
  ...initialState,
  setFeedbackInterval: (feedbackInterval) => set({ feedbackInterval }),
  setQuizType: (quizType) => set({ quizType }),
  setFlashcards: (flashcards) => set({ flashcards }),
  setReverseFlashcards: (reverseFlashcards) => set({ reverseFlashcards }),
  setEtymologyOriginCards: (etymologyOriginCards) => set({ etymologyOriginCards }),
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
  submitEtymologyOriginResult: (result) =>
    set((state) => ({
      etymologyOriginResults: [
        ...state.etymologyOriginResults,
        // Capture each word's as-graded correctness once, up front, so later
        // per-word overrides can show "(overridden)" by comparing against it
        // (see EtymologyWordResult.originalCorrect).
        { ...result, words: result.words.map((w) => ({ ...w, originalCorrect: w.correct })) },
      ],
    })),
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
        return { etymologyOriginResults: updateArrayItem(state.etymologyOriginResults, index, { correct: !state.etymologyOriginResults[index].correct, isOverridden: true, nextReviewDate, originalValues }) };
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
        return { etymologyOriginResults: updateArrayItem(state.etymologyOriginResults, index, { correct, isOverridden: false, nextReviewDate, originalValues: undefined }) };
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
        return { etymologyOriginResults: updateArrayItem(state.etymologyOriginResults, index, { isSkipped: true }) };
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
        return { etymologyOriginResults: updateArrayItem(state.etymologyOriginResults, index, { isSkipped: false }) };
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
        return { etymologyOriginResults: updateArrayItem(state.etymologyOriginResults, index, { nextReviewDate: newDate }) };
      }
      return { freeformResults: updateArrayItem(state.freeformResults, index, { nextReviewDate: newDate }) };
    }),

  overrideEtymologyWord: (index, wordExpression, correct) =>
    set((state) => {
      const result = state.etymologyOriginResults[index];
      if (!result) return {};
      return {
        etymologyOriginResults: updateArrayItem(state.etymologyOriginResults, index, {
          words: result.words.map((w) => (w.expression === wordExpression ? { ...w, correct } : w)),
        }),
      };
    }),

  excludeEtymologyWord: (index, wordExpression, excluded) =>
    set((state) => {
      const result = state.etymologyOriginResults[index];
      if (!result) return {};
      return {
        etymologyOriginResults: updateArrayItem(state.etymologyOriginResults, index, {
          words: result.words.map((w) => (w.expression === wordExpression ? { ...w, isExcluded: excluded } : w)),
        }),
      };
    }),
}));
