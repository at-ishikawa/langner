import { beforeEach, describe, expect, it } from "vitest";
import { useRelearnStore, type RelearnItem } from "./relearnStore";
import { QuizType, type RelearnCard } from "@/lib/client";

function card(entry: string): RelearnCard {
  // Only the fields the queue logic touches matter here.
  return { entry } as RelearnCard;
}

// A grammar correction card: its post text is the grouping key.
function grammarCard(content: string, incorrect: string): RelearnCard {
  return {
    entry: incorrect,
    sourceQuizType: QuizType.GRAMMAR,
    content,
    incorrect,
  } as RelearnCard;
}

// An etymology-origin card: originText + originMeaning is the grouping key.
function originCard(entry: string, originText = "capere"): RelearnCard {
  return {
    entry,
    sourceQuizType: QuizType.ETYMOLOGY_ORIGIN,
    originText,
    originMeaning: "to take",
    type: "root",
    language: "Latin",
  } as RelearnCard;
}

// entries flattens the queue to the entry of each single card, so the
// card-only tests read the same as before the queue became a list of screens.
function entries(queue: RelearnItem[]): (string | undefined)[] {
  return queue.map((it) => (it.kind === "card" ? it.card.entry : undefined));
}

// originWords reads the words of the front origin screen (the one completeOrigin
// acts on), as the session page passes them into RelearnOriginPost.
function frontOriginWords(): RelearnCard[] {
  const front = useRelearnStore.getState().queue[0];
  if (front?.kind !== "origin") throw new Error("front is not an origin screen");
  return front.group.words;
}

// frontPostBlanks reads the blanks of the front grammar post screen (the one
// completeGrammarPost acts on), as the session page passes them into
// RelearnGrammarPost.
function frontPostBlanks(): RelearnCard[] {
  const front = useRelearnStore.getState().queue[0];
  if (front?.kind !== "post") throw new Error("front is not a post screen");
  return front.post.blanks;
}

describe("useRelearnStore", () => {
  beforeEach(() => {
    useRelearnStore.getState().reset();
  });

  it("seeds the queue and resets counters", () => {
    useRelearnStore.getState().seedQueue([card("a"), card("b")]);
    const s = useRelearnStore.getState();
    expect(entries(s.queue)).toEqual(["a", "b"]);
    expect(s.clearedCount).toBe(0);
    expect(s.totalAnswers).toBe(0);
  });

  it("drops the front card and counts a clear on a correct answer", () => {
    useRelearnStore.getState().seedQueue([card("a"), card("b")]);
    useRelearnStore.getState().resolveFront(true);
    const s = useRelearnStore.getState();
    expect(entries(s.queue)).toEqual(["b"]);
    expect(s.clearedCount).toBe(1);
    expect(s.totalAnswers).toBe(1);
  });

  it("moves the front card to the back on a wrong answer without clearing", () => {
    useRelearnStore.getState().seedQueue([card("a"), card("b")]);
    useRelearnStore.getState().resolveFront(false);
    const s = useRelearnStore.getState();
    expect(entries(s.queue)).toEqual(["b", "a"]);
    expect(s.clearedCount).toBe(0);
    expect(s.totalAnswers).toBe(1);
  });

  it("clears a word exactly once even if it was wrong first", () => {
    useRelearnStore.getState().seedQueue([card("a")]);
    useRelearnStore.getState().resolveFront(false); // a -> back (only card, stays)
    expect(entries(useRelearnStore.getState().queue)).toEqual(["a"]);
    useRelearnStore.getState().resolveFront(true); // a cleared
    const s = useRelearnStore.getState();
    expect(s.queue).toEqual([]);
    expect(s.clearedCount).toBe(1);
    expect(s.totalAnswers).toBe(2);
  });

  it("ends only when the queue is empty", () => {
    useRelearnStore.getState().seedQueue([card("a"), card("b")]);
    useRelearnStore.getState().resolveFront(true);
    expect(useRelearnStore.getState().queue.length).toBe(1);
    useRelearnStore.getState().resolveFront(true);
    expect(useRelearnStore.getState().queue.length).toBe(0);
  });

  it("resolveFront on an empty queue is a no-op", () => {
    useRelearnStore.getState().resolveFront(true);
    const s = useRelearnStore.getState();
    expect(s.queue).toEqual([]);
    expect(s.clearedCount).toBe(0);
    expect(s.totalAnswers).toBe(0);
  });

  it("reset clears queue and counters", () => {
    useRelearnStore.getState().seedQueue([card("a")]);
    useRelearnStore.getState().resolveFront(false);
    useRelearnStore.getState().reset();
    const s = useRelearnStore.getState();
    expect(s.queue).toEqual([]);
    expect(s.clearedCount).toBe(0);
    expect(s.totalAnswers).toBe(0);
  });

  // Grammar corrections that share a post fold into ONE post screen so the
  // whole entry is shown once and its due blanks are drilled together, like
  // the live grammar quiz — while vocab cards stay one-per-screen.
  it("groups same-post grammar corrections into one post screen", () => {
    const post = "Yesterday the John called me and I go home.";
    useRelearnStore
      .getState()
      .seedQueue([
        card("alpha"),
        grammarCard(post, "the John"),
        grammarCard(post, "go"),
      ]);
    const q = useRelearnStore.getState().queue;
    expect(q).toHaveLength(2); // one card screen + one post screen
    expect(q[0]).toMatchObject({ kind: "card" });
    expect(q[1].kind).toBe("post");
    if (q[1].kind === "post") {
      expect(q[1].post.blanks.map((b) => b.incorrect)).toEqual(["the John", "go"]);
      expect(q[1].post.attempt).toBe(0);
    }
  });

  // A grammar post re-queues the blanks answered WRONG so they are re-drilled
  // this session — the post-level analogue of completeOrigin / resolveFront.
  it("completeGrammarPost re-queues only the wrong blanks and clears only the correct ones", () => {
    const post = "Yesterday the John called me and I go home fast.";
    useRelearnStore
      .getState()
      .seedQueue([
        grammarCard(post, "the John"),
        grammarCard(post, "go"),
        grammarCard(post, "fast"),
        card("beta"),
      ]);
    const blanks = frontPostBlanks();
    // 1 correct (the John), 2 wrong (go, fast).
    const wrong = [blanks[1], blanks[2]];
    useRelearnStore.getState().completeGrammarPost(wrong, 1);

    const s = useRelearnStore.getState();
    // The card screen stays; the post re-queues behind it as a smaller post.
    expect(s.queue).toHaveLength(2);
    expect(s.queue[0]).toMatchObject({ kind: "card" });
    expect(s.queue[1].kind).toBe("post");
    if (s.queue[1].kind === "post") {
      expect(s.queue[1].post.blanks.map((b) => b.incorrect)).toEqual(["go", "fast"]);
      expect(s.queue[1].post.content).toBe(post); // same post text
      expect(s.queue[1].post.attempt).toBe(1); // re-queued → unique remount key
    }
    // Only the correct blank clears; every blank this pass is one answer.
    expect(s.clearedCount).toBe(1);
    expect(s.totalAnswers).toBe(3);
  });

  it("completeGrammarPost all-correct on the re-queued post empties the queue with correct counts", () => {
    const post = "Yesterday the John called me and I go home.";
    useRelearnStore
      .getState()
      .seedQueue([grammarCard(post, "the John"), grammarCard(post, "go")]);
    const first = frontPostBlanks();
    useRelearnStore.getState().completeGrammarPost([first[1]], 1); // 1 re-queued (go)
    // Second pass over the 1-blank post, correct.
    useRelearnStore.getState().completeGrammarPost([], 1);

    const s = useRelearnStore.getState();
    expect(s.queue).toEqual([]);
    // 1 cleared on the first pass + 1 on the retry = both, each counted once.
    expect(s.clearedCount).toBe(2);
    // 2 answers on the first pass + 1 on the retry.
    expect(s.totalAnswers).toBe(3);
  });

  it("completeGrammarPost all-correct on the first pass drops the post and re-queues nothing", () => {
    const post = "Yesterday the John called me and I go home.";
    useRelearnStore
      .getState()
      .seedQueue([grammarCard(post, "the John"), grammarCard(post, "go"), card("beta")]);
    useRelearnStore.getState().completeGrammarPost([], 2);
    const s = useRelearnStore.getState();
    expect(s.queue).toHaveLength(1);
    expect(entries(s.queue)).toEqual(["beta"]);
    expect(s.clearedCount).toBe(2);
    expect(s.totalAnswers).toBe(2);
  });

  it("completeGrammarPost on an empty queue is a no-op", () => {
    useRelearnStore.getState().completeGrammarPost([], 0);
    const s = useRelearnStore.getState();
    expect(s.queue).toEqual([]);
    expect(s.clearedCount).toBe(0);
    expect(s.totalAnswers).toBe(0);
  });

  // Etymology-origin cards that share an origin fold into ONE family screen, and
  // that screen re-queues the words answered WRONG so they are re-drilled this
  // session — the group-level analogue of resolveFront's card loop.
  it("groups same-origin cards into one origin family screen", () => {
    useRelearnStore
      .getState()
      .seedQueue([originCard("recipient"), originCard("intercept"), originCard("captive")]);
    const q = useRelearnStore.getState().queue;
    expect(q).toHaveLength(1);
    expect(q[0].kind).toBe("origin");
    if (q[0].kind === "origin") {
      expect(q[0].group.words.map((w) => w.entry)).toEqual([
        "recipient",
        "intercept",
        "captive",
      ]);
      expect(q[0].group.attempt).toBe(0);
    }
  });

  it("completeOrigin re-queues only the wrong words and clears only the correct ones", () => {
    useRelearnStore
      .getState()
      .seedQueue([originCard("recipient"), originCard("intercept"), originCard("captive")]);
    const words = frontOriginWords();
    // 1 correct (recipient), 2 wrong (intercept, captive).
    const wrong = [words[1], words[2]];
    useRelearnStore.getState().completeOrigin(wrong, 1);

    const s = useRelearnStore.getState();
    expect(s.queue).toHaveLength(1);
    expect(s.queue[0].kind).toBe("origin");
    if (s.queue[0].kind === "origin") {
      // A smaller family with exactly the two still-wrong words, same header.
      expect(s.queue[0].group.words.map((w) => w.entry)).toEqual(["intercept", "captive"]);
      expect(s.queue[0].group.originText).toBe("capere");
      expect(s.queue[0].group.attempt).toBe(1); // re-queued → unique remount key
    }
    // Only the correct word is cleared; every word this pass is one answer.
    expect(s.clearedCount).toBe(1);
    expect(s.totalAnswers).toBe(3);
  });

  it("completeOrigin all-correct on the re-queued family empties the queue with correct counts", () => {
    useRelearnStore
      .getState()
      .seedQueue([originCard("recipient"), originCard("intercept"), originCard("captive")]);
    const first = frontOriginWords();
    useRelearnStore.getState().completeOrigin([first[1], first[2]], 1); // 2 re-queued
    // Second pass over the 2-word family, both correct.
    useRelearnStore.getState().completeOrigin([], 2);

    const s = useRelearnStore.getState();
    expect(s.queue).toEqual([]);
    // 1 cleared on the first pass + 2 on the retry = all 3, each counted once.
    expect(s.clearedCount).toBe(3);
    // 3 answers on the first pass + 2 on the retry.
    expect(s.totalAnswers).toBe(5);
  });

  it("completeOrigin all-correct on the first pass drops the family and re-queues nothing", () => {
    useRelearnStore
      .getState()
      .seedQueue([originCard("recipient"), originCard("intercept")]);
    useRelearnStore.getState().completeOrigin([], 2);
    const s = useRelearnStore.getState();
    expect(s.queue).toEqual([]);
    expect(s.clearedCount).toBe(2);
    expect(s.totalAnswers).toBe(2);
  });

  it("completeOrigin on an empty queue is a no-op", () => {
    useRelearnStore.getState().completeOrigin([], 0);
    const s = useRelearnStore.getState();
    expect(s.queue).toEqual([]);
    expect(s.clearedCount).toBe(0);
    expect(s.totalAnswers).toBe(0);
  });
});
