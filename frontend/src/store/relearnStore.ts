import { create } from "zustand";
import type { RelearnCard } from "@/lib/client";

// The Relearn Quiz is a client-driven loop over a working queue. The existing
// quiz store advances a fixed list with a monotonically increasing index and
// has no requeue primitive, so the loop lives in this dedicated store.
//
// Loop rule: the front card is the current one. A correct answer removes it
// (and increments clearedCount); a wrong or skipped answer moves it to the back
// so it comes around again later in the same session. The session ends when the
// queue is empty. totalAnswers counts every answer (it exceeds clearedCount
// because re-queued cards are answered more than once).
interface RelearnState {
  queue: RelearnCard[];
  clearedCount: number;
  totalAnswers: number;
  seedQueue: (cards: RelearnCard[]) => void;
  resolveFront: (correct: boolean) => void;
  reset: () => void;
}

const initialState = {
  queue: [] as RelearnCard[],
  clearedCount: 0,
  totalAnswers: 0,
};

export const useRelearnStore = create<RelearnState>((set) => ({
  ...initialState,
  seedQueue: (cards) => set({ queue: [...cards], clearedCount: 0, totalAnswers: 0 }),
  resolveFront: (correct) =>
    set((state) => {
      if (state.queue.length === 0) {
        return {};
      }
      const [front, ...rest] = state.queue;
      if (correct) {
        return {
          queue: rest,
          clearedCount: state.clearedCount + 1,
          totalAnswers: state.totalAnswers + 1,
        };
      }
      return {
        queue: [...rest, front],
        totalAnswers: state.totalAnswers + 1,
      };
    }),
  reset: () => set(initialState),
}));
