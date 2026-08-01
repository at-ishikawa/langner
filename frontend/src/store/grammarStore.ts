import { create } from "zustand";
import type { GrammarPostCard } from "@/lib/client";
import type { OriginalValues } from "@/store/quizStore";

// The Grammar quiz drills a journal notebook one POST at a time. Each post is
// shown in full; its due mistakes are corrected inline and submitted together,
// then reviewed one blank at a time. Results accumulate across every post and
// feed the shared completion screen.

// GrammarResultState is one graded blank enriched with the user's answer and
// mutable override state. It is both the review-sheet data source and the
// accumulator element the complete screen renders.
export interface GrammarResultState {
  postIndex: number; // which post produced it (review filters on this)
  noteId: bigint; // stable session identity for override/skip
  senseId: string;
  incorrect: string;
  answer: string; // the user's typed correction
  correct: boolean;
  correctAnswer: string;
  reason: string;
  category: string;
  nextReviewDate: string;
  learnedAt: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
}

interface GrammarState {
  posts: GrammarPostCard[];
  currentPostIndex: number;
  inputs: Record<string, string>; // key = noteId string; current post only
  results: GrammarResultState[]; // accumulator across all posts
  submittedPostIndices: number[]; // posts already graded (drives the phase)
  reviewedKeys: string[]; // noteId strings reviewed in the current post
  selectedKey: string | null; // selected blank (noteId string) in review

  seedPosts: (posts: GrammarPostCard[]) => void;
  setInput: (key: string, value: string) => void;
  markPostSubmitted: (postIndex: number) => void;
  recordPostResults: (results: GrammarResultState[]) => void;
  selectBlank: (key: string | null) => void;
  markReviewed: (key: string) => void;
  nextPost: () => void;

  overrideResult: (index: number, nextReviewDate: string, originalValues: OriginalValues) => void;
  undoOverrideResult: (index: number, correct: boolean, nextReviewDate: string) => void;
  skipResult: (index: number) => void;
  resumeResult: (index: number) => void;

  reset: () => void;
}

const initialState = {
  posts: [] as GrammarPostCard[],
  currentPostIndex: 0,
  inputs: {} as Record<string, string>,
  results: [] as GrammarResultState[],
  submittedPostIndices: [] as number[],
  reviewedKeys: [] as string[],
  selectedKey: null as string | null,
};

function updateArrayItem<T>(arr: T[], index: number, patch: Partial<T>): T[] {
  return arr.map((item, i) => (i === index ? { ...item, ...patch } : item));
}

export const useGrammarStore = create<GrammarState>((set) => ({
  ...initialState,
  seedPosts: (posts) => set({ ...initialState, posts: [...posts] }),
  setInput: (key, value) => set((state) => ({ inputs: { ...state.inputs, [key]: value } })),
  // Enter review as soon as the user submits — pills render immediately and
  // fill in as each chunk of results arrives via recordPostResults.
  markPostSubmitted: (postIndex) =>
    set((state) =>
      state.submittedPostIndices.includes(postIndex)
        ? {}
        : { submittedPostIndices: [...state.submittedPostIndices, postIndex] },
    ),
  recordPostResults: (results) =>
    set((state) => {
      // Auto-select the first wrong blank (or first) once results start landing.
      const selectedKey =
        state.selectedKey ??
        (results.find((r) => !r.correct) ?? results[0])?.noteId.toString() ??
        null;
      return { results: [...state.results, ...results], selectedKey };
    }),
  selectBlank: (selectedKey) => set({ selectedKey }),
  markReviewed: (key) =>
    set((state) =>
      state.reviewedKeys.includes(key) ? {} : { reviewedKeys: [...state.reviewedKeys, key] },
    ),
  nextPost: () =>
    set((state) => ({
      currentPostIndex: state.currentPostIndex + 1,
      inputs: {},
      reviewedKeys: [],
      selectedKey: null,
    })),

  overrideResult: (index, nextReviewDate, originalValues) =>
    set((state) => ({
      results: updateArrayItem(state.results, index, {
        correct: !state.results[index].correct,
        isOverridden: true,
        nextReviewDate,
        originalValues,
      }),
    })),
  undoOverrideResult: (index, correct, nextReviewDate) =>
    set((state) => ({
      results: updateArrayItem(state.results, index, {
        correct,
        isOverridden: false,
        nextReviewDate,
        originalValues: undefined,
      }),
    })),
  skipResult: (index) =>
    set((state) => ({ results: updateArrayItem(state.results, index, { isSkipped: true }) })),
  resumeResult: (index) =>
    set((state) => ({ results: updateArrayItem(state.results, index, { isSkipped: false }) })),

  reset: () => set(initialState),
}));
