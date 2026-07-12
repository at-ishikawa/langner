import { beforeEach, describe, expect, it } from "vitest";
import { useRelearnStore } from "./relearnStore";
import type { RelearnCard } from "@/lib/client";

function card(entry: string): RelearnCard {
  // Only the fields the queue logic touches matter here.
  return { entry } as RelearnCard;
}

describe("useRelearnStore", () => {
  beforeEach(() => {
    useRelearnStore.getState().reset();
  });

  it("seeds the queue and resets counters", () => {
    useRelearnStore.getState().seedQueue([card("a"), card("b")]);
    const s = useRelearnStore.getState();
    expect(s.queue.map((c) => c.entry)).toEqual(["a", "b"]);
    expect(s.clearedCount).toBe(0);
    expect(s.totalAnswers).toBe(0);
  });

  it("drops the front card and counts a clear on a correct answer", () => {
    useRelearnStore.getState().seedQueue([card("a"), card("b")]);
    useRelearnStore.getState().resolveFront(true);
    const s = useRelearnStore.getState();
    expect(s.queue.map((c) => c.entry)).toEqual(["b"]);
    expect(s.clearedCount).toBe(1);
    expect(s.totalAnswers).toBe(1);
  });

  it("moves the front card to the back on a wrong answer without clearing", () => {
    useRelearnStore.getState().seedQueue([card("a"), card("b")]);
    useRelearnStore.getState().resolveFront(false);
    const s = useRelearnStore.getState();
    expect(s.queue.map((c) => c.entry)).toEqual(["b", "a"]);
    expect(s.clearedCount).toBe(0);
    expect(s.totalAnswers).toBe(1);
  });

  it("clears a word exactly once even if it was wrong first", () => {
    useRelearnStore.getState().seedQueue([card("a")]);
    useRelearnStore.getState().resolveFront(false); // a -> back (only card, stays)
    expect(useRelearnStore.getState().queue.map((c) => c.entry)).toEqual(["a"]);
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
});
