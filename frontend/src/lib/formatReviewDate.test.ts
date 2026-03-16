import { describe, it, expect, vi, afterEach } from "vitest";
import { formatReviewDate } from "./formatReviewDate";

describe("formatReviewDate", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns 'tomorrow' when date is tomorrow", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 2, 15)); // March 15, 2026
    expect(formatReviewDate("2026-03-16")).toBe("tomorrow");
  });

  it("returns 'in X days' when date is within 7 days", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 2, 15));
    expect(formatReviewDate("2026-03-18")).toBe("in 3 days");
    expect(formatReviewDate("2026-03-22")).toBe("in 7 days");
  });

  it("returns full date when date is more than 7 days away", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 2, 15));
    expect(formatReviewDate("2026-03-25")).toBe("March 25, 2026");
  });

  it("returns full date for past dates", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 2, 15));
    expect(formatReviewDate("2026-03-10")).toBe("March 10, 2026");
  });

  it("returns original string for invalid format", () => {
    expect(formatReviewDate("not-a-date")).toBe("not-a-date");
    expect(formatReviewDate("")).toBe("");
  });
});
