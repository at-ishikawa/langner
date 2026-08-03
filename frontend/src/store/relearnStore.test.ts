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

// entries flattens the queue to the entry of each single card, so the
// card-only tests read the same as before the queue became a list of screens.
function entries(queue: RelearnItem[]): (string | undefined)[] {
  return queue.map((it) => (it.kind === "card" ? it.card.entry : undefined));
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
    }
  });

  it("completePost removes the post screen and tallies its blanks", () => {
    const post = "Yesterday the John called me and I go home.";
    useRelearnStore
      .getState()
      .seedQueue([grammarCard(post, "the John"), grammarCard(post, "go"), card("beta")]);
    // Two blanks, one correct.
    useRelearnStore.getState().completePost(1, 2);
    const s = useRelearnStore.getState();
    expect(s.queue).toHaveLength(1);
    expect(entries(s.queue)).toEqual(["beta"]);
    expect(s.clearedCount).toBe(1);
    expect(s.totalAnswers).toBe(2);
  });
});
