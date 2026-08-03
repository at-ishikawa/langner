import { create } from "zustand";
import { QuizType, type RelearnCard } from "@/lib/client";

// The Relearn Quiz is a client-driven loop over a working queue. The existing
// quiz store advances a fixed list with a monotonically increasing index and
// has no requeue primitive, so the loop lives in this dedicated store.
//
// A queue entry is one SCREEN, not one card:
//
//   - a vocab / etymology / reverse card is one screen (RelearnItem "card"),
//   - a journal post's due grammar corrections are ONE screen (RelearnItem
//     "post") holding every due blank of that post, so relearn presents the
//     post once and drills all its blanks progressively — exactly like the
//     live grammar quiz — instead of re-showing the whole post per blank.
//
// Loop rule for a card: the front card is the current one; a correct answer
// removes it (and increments clearedCount), a wrong or skipped answer moves it
// to the back so it comes around again later in the same session. A grammar
// post mirrors the live grammar quiz instead — it is answered in one pass and
// removed (completePost), never requeued. The session ends when the queue is
// empty. totalAnswers counts every answer (it exceeds clearedCount because
// re-queued cards are answered more than once).

// RelearnPostGroup is one journal post shown once, with its due corrections as
// inline blanks answered progressively. Every blank of a post carries the same
// full post text (RelearnCard.content), so content is the group key.
export interface RelearnPostGroup {
  content: string;
  blanks: RelearnCard[];
}

export type RelearnItem =
  | { kind: "card"; card: RelearnCard }
  | { kind: "post"; post: RelearnPostGroup };

interface RelearnState {
  queue: RelearnItem[];
  clearedCount: number;
  totalAnswers: number;
  seedQueue: (cards: RelearnCard[]) => void;
  resolveFront: (correct: boolean) => void;
  completePost: (correctCount: number, blankCount: number) => void;
  reset: () => void;
}

// groupIntoItems turns the flat pool the server returns into per-screen items:
// non-grammar cards stay one-per-screen; grammar corrections that share a post
// fold into a single post screen, in first-seen order.
function groupIntoItems(cards: RelearnCard[]): RelearnItem[] {
  const items: RelearnItem[] = [];
  const postByContent = new Map<string, RelearnPostGroup>();
  for (const card of cards) {
    if (card.sourceQuizType !== QuizType.GRAMMAR) {
      items.push({ kind: "card", card });
      continue;
    }
    const existing = postByContent.get(card.content);
    if (existing) {
      existing.blanks.push(card);
      continue;
    }
    const post: RelearnPostGroup = { content: card.content, blanks: [card] };
    postByContent.set(card.content, post);
    items.push({ kind: "post", post });
  }
  return items;
}

const initialState = {
  queue: [] as RelearnItem[],
  clearedCount: 0,
  totalAnswers: 0,
};

export const useRelearnStore = create<RelearnState>((set) => ({
  ...initialState,
  seedQueue: (cards) =>
    set({ queue: groupIntoItems(cards), clearedCount: 0, totalAnswers: 0 }),
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
  completePost: (correctCount, blankCount) =>
    set((state) => {
      if (state.queue.length === 0) {
        return {};
      }
      const [, ...rest] = state.queue;
      return {
        queue: rest,
        clearedCount: state.clearedCount + correctCount,
        totalAnswers: state.totalAnswers + blankCount,
      };
    }),
  reset: () => set(initialState),
}));
