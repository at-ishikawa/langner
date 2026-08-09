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
// to the back so it comes around again later in the same session. An origin
// family screen mirrors this at the GROUP level (completeOrigin): the words
// answered correctly clear, and the words answered wrong re-queue as a smaller
// family screen at the back so they come around again this session — the origin
// loop only ends once every word in it is answered correctly. A grammar post is
// the exception: it mirrors the live grammar quiz and is answered in one pass
// and removed (completePost), never requeued (a missed blank is simply due next
// session). The session ends when the queue is empty. totalAnswers counts every
// answer (it exceeds clearedCount because re-queued cards/words are answered
// more than once).

// RelearnPostGroup is one journal post shown once, with its due corrections as
// inline blanks answered progressively. Every blank of a post carries the same
// full post text (RelearnCard.content), so content is the group key.
export interface RelearnPostGroup {
  content: string;
  blanks: RelearnCard[];
}

// RelearnOriginGroup is one etymology origin shown once, with the missed words
// that share it listed as a family. Every card of the group carries the same
// originText + originMeaning (the grouping key); type/language head the origin.
// Presentation only — each word still grades through its own SubmitRelearnAnswer
// call keyed by its note_id, exactly like a recognition card.
export interface RelearnOriginGroup {
  originText: string;
  originMeaning: string;
  type: string;
  language: string;
  // englishForms are the origin's English combining-form spellings (e.g. the
  // Latin root "liber" → ["lib", "liv"]). A per-origin value, so it is taken
  // from the first card of the group and shown on the origin header.
  englishForms: string[];
  words: RelearnCard[];
  // attempt counts how many times this origin family has been (re-)queued: 0 on
  // first appearance, +1 each time its still-wrong words re-queue. It is not
  // shown — it only makes the React key of a re-queued family unique so the
  // origin screen remounts fresh (a family whose wrong words are the SAME set
  // would otherwise keep the same key, and its per-word grading state, on retry).
  attempt: number;
}

export type RelearnItem =
  | { kind: "card"; card: RelearnCard }
  | { kind: "post"; post: RelearnPostGroup }
  | { kind: "origin"; group: RelearnOriginGroup };

interface RelearnState {
  queue: RelearnItem[];
  clearedCount: number;
  totalAnswers: number;
  seedQueue: (cards: RelearnCard[]) => void;
  resolveFront: (correct: boolean) => void;
  completePost: (correctCount: number, blankCount: number) => void;
  completeOrigin: (wrongWords: RelearnCard[], correctCount: number) => void;
  reset: () => void;
}

// groupIntoItems turns the flat pool the server returns into per-screen items:
// recognition/reverse cards stay one-per-screen; grammar corrections that share
// a post fold into a single post screen; etymology-origin cards that share an
// origin fold into a single origin family screen — all in first-seen order.
function groupIntoItems(cards: RelearnCard[]): RelearnItem[] {
  const items: RelearnItem[] = [];
  const postByContent = new Map<string, RelearnPostGroup>();
  const originByKey = new Map<string, RelearnOriginGroup>();
  for (const card of cards) {
    if (card.sourceQuizType === QuizType.GRAMMAR) {
      const existing = postByContent.get(card.content);
      if (existing) {
        existing.blanks.push(card);
        continue;
      }
      const post: RelearnPostGroup = { content: card.content, blanks: [card] };
      postByContent.set(card.content, post);
      items.push({ kind: "post", post });
      continue;
    }
    if (card.sourceQuizType === QuizType.ETYMOLOGY_ORIGIN) {
      const key = `${card.originText} ${card.originMeaning}`;
      const existing = originByKey.get(key);
      if (existing) {
        existing.words.push(card);
        continue;
      }
      const group: RelearnOriginGroup = {
        originText: card.originText,
        originMeaning: card.originMeaning,
        type: card.type,
        language: card.language,
        englishForms: card.englishForms ?? [],
        words: [card],
        attempt: 0,
      };
      originByKey.set(key, group);
      items.push({ kind: "origin", group });
      continue;
    }
    items.push({ kind: "card", card });
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
  // completeOrigin ends one pass over the front origin family. Every word in the
  // family was answered this pass, so it counts as many attempts as the family
  // has words (totalAnswers) and clears the ones answered correctly this pass
  // (clearedCount). The still-wrong words re-queue as a smaller family screen at
  // the BACK — same origin header, only the missed words — so they come around
  // again this session and each word clears exactly once, when it is finally
  // right (mirrors resolveFront's card loop, at the group level). When every
  // word was correct there is nothing to re-queue and the family is dropped.
  completeOrigin: (wrongWords, correctCount) =>
    set((state) => {
      const [front, ...rest] = state.queue;
      if (!front || front.kind !== "origin") {
        return {};
      }
      const counts = {
        clearedCount: state.clearedCount + correctCount,
        totalAnswers: state.totalAnswers + front.group.words.length,
      };
      if (wrongWords.length === 0) {
        return { ...counts, queue: rest };
      }
      const retry: RelearnItem = {
        kind: "origin",
        group: { ...front.group, words: wrongWords, attempt: front.group.attempt + 1 },
      };
      return { ...counts, queue: [...rest, retry] };
    }),
  reset: () => set(initialState),
}));
