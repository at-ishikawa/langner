import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import RelearnSessionPage from "./page";
import { useRelearnStore } from "@/store/relearnStore";
import type { RelearnCard } from "@/lib/client";

const submitRelearnAnswer = vi.fn();
const overrideRelearnCard = vi.fn();
vi.mock("@/lib/client", () => ({
  quizClient: {
    submitRelearnAnswer: (...args: unknown[]) => submitRelearnAnswer(...args),
    overrideRelearnCard: (...args: unknown[]) => overrideRelearnCard(...args),
  },
  QuizType: { QUIZ_TYPE_UNSPECIFIED: 0, STANDARD: 1, REVERSE: 2, FREEFORM: 3, ETYMOLOGY_STANDARD: 4, ETYMOLOGY_REVERSE: 5, ETYMOLOGY_FREEFORM: 6 },
}));

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: pushMock }) }));

// Isolate the loop from the context block.
vi.mock("@/components/RelearnContext", () => ({ default: () => <div data-testid="ctx" /> }));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <RelearnSessionPage />
    </ChakraProvider>,
  );
}

const card = (entry: string): RelearnCard =>
  ({ entry, noteId: BigInt(entry.length), sourceQuizType: 1, meaning: `${entry}-meaning`, examples: [], contexts: [] }) as RelearnCard;

// A reverse-format card: shown meaning, asks for the word.
const reverseCard = (entry: string, meaning: string): RelearnCard =>
  ({ entry, noteId: BigInt(entry.length), sourceQuizType: 2, meaning, examples: [], contexts: [] }) as RelearnCard;

describe("RelearnSessionPage", () => {
  beforeEach(() => {
    submitRelearnAnswer.mockReset();
    overrideRelearnCard.mockReset();
    overrideRelearnCard.mockResolvedValue({});
    pushMock.mockReset();
    useRelearnStore.getState().reset();
  });

  it("lets the learner override a wrong grade to correct, clearing the word", async () => {
    useRelearnStore.getState().seedQueue([card("alpha"), card("beta")]);
    submitRelearnAnswer.mockResolvedValue({ correct: false, meaning: "the first", reason: "grader was wrong" });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "knowing all stuff" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByText("✗ Incorrect")).toBeInTheDocument();

    // Override to correct: banner flips, and Next clears the word.
    fireEvent.click(screen.getByRole("button", { name: "Mark as Correct" }));
    expect(screen.getByText("✓ Correct")).toBeInTheDocument();
    expect(screen.getByText("(overridden)")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() =>
      expect(overrideRelearnCard).toHaveBeenCalledWith(expect.objectContaining({ markCorrect: true })),
    );
    // alpha cleared despite the wrong grade; beta is next.
    expect(useRelearnStore.getState().queue.map((c) => c.entry)).toEqual(["beta"]);
    expect(useRelearnStore.getState().clearedCount).toBe(1);
  });

  it("does not call the override RPC when the grader's verdict is accepted", async () => {
    useRelearnStore.getState().seedQueue([card("alpha")]);
    submitRelearnAnswer.mockResolvedValue({ correct: true, meaning: "the first", reason: "ok" });
    renderPage();
    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "the first" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    await screen.findByText("✓ Correct");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(useRelearnStore.getState().clearedCount).toBe(1));
    expect(overrideRelearnCard).not.toHaveBeenCalled();
  });

  it("redirects to start when the queue is empty and nothing was answered", async () => {
    renderPage();
    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/quiz/relearn"));
  });

  it("shows the current card and the words-left counter", () => {
    useRelearnStore.getState().seedQueue([card("alpha"), card("beta")]);
    renderPage();
    expect(screen.getByText("alpha")).toBeInTheDocument();
    expect(screen.getByText("2 words left")).toBeInTheDocument();
    expect(screen.getByText(/Recognition/)).toBeInTheDocument();
  });

  it("renders a reverse-format card as meaning-to-word", () => {
    useRelearnStore.getState().seedQueue([reverseCard("delta", "a change or difference")]);
    renderPage();
    // The meaning is the prompt; the word is NOT shown.
    expect(screen.getByTestId("relearn-prompt")).toHaveTextContent("a change or difference");
    expect(screen.queryByText("delta")).not.toBeInTheDocument();
    expect(screen.getByText(/Reverse/)).toBeInTheDocument();
    expect(screen.getByText("The word")).toBeInTheDocument();
  });

  it("submits a correct answer, shows feedback, and clears the word on Next", async () => {
    useRelearnStore.getState().seedQueue([card("alpha"), card("beta")]);
    submitRelearnAnswer.mockResolvedValue({ correct: true, meaning: "the first", reason: "ok" });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "the first" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));

    expect(await screen.findByText("✓ Correct")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    // alpha cleared, beta now current.
    expect(useRelearnStore.getState().queue.map((c) => c.entry)).toEqual(["beta"]);
    expect(useRelearnStore.getState().clearedCount).toBe(1);
    expect(screen.getByText("beta")).toBeInTheDocument();
  });

  it("requeues a wrong answer to the back", async () => {
    useRelearnStore.getState().seedQueue([card("alpha"), card("beta")]);
    submitRelearnAnswer.mockResolvedValue({ correct: false, meaning: "the first", reason: "nope" });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "bad guess" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByText("✗ Incorrect")).toBeInTheDocument();
    // The correct meaning and the learner's own answer are both shown.
    expect(screen.getByText("Meaning:")).toBeInTheDocument();
    expect(screen.getByText("the first")).toBeInTheDocument();
    expect(screen.getByText("Your answer:")).toBeInTheDocument();
    expect(screen.getByText("bad guess")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    expect(useRelearnStore.getState().queue.map((c) => c.entry)).toEqual(["beta", "alpha"]);
    expect(useRelearnStore.getState().clearedCount).toBe(0);
  });

  it("skips a card as not-correct but still shows the answer", async () => {
    useRelearnStore.getState().seedQueue([card("alpha")]);
    submitRelearnAnswer.mockResolvedValue({ correct: false, meaning: "the first", reason: "skipped by user" });
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Don't Know" }));
    expect(await screen.findByText("✗ Incorrect")).toBeInTheDocument();
    expect(submitRelearnAnswer).toHaveBeenCalledWith(expect.objectContaining({ isSkipped: true }));
  });

  it("navigates to complete when the last word is cleared", async () => {
    useRelearnStore.getState().seedQueue([card("alpha")]);
    submitRelearnAnswer.mockResolvedValue({ correct: true, meaning: "the first", reason: "ok" });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "the first" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    await screen.findByText("✓ Correct");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/quiz/relearn/complete"));
  });

  it("surfaces a grading error and returns to answering", async () => {
    useRelearnStore.getState().seedQueue([card("alpha")]);
    submitRelearnAnswer.mockRejectedValue(new Error("boom"));
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Grading failed");
  });
});
