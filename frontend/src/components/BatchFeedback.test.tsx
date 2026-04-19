import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { BatchFeedback } from "./BatchFeedback";
import type { ResultItem } from "./QuizResultCard";

const mockItems: ResultItem[] = [
  {
    index: 0,
    key: "1",
    entry: "break the ice",
    meaning: "to initiate conversation",
    correct: true,
    originalCorrect: true,
    userAnswer: "start a conversation",
    noteId: BigInt(1),
    learnedAt: "2026-03-16T00:00:00Z",
  },
  {
    index: 1,
    key: "2",
    entry: "lose one's temper",
    meaning: "to become angry",
    correct: false,
    originalCorrect: false,
    userAnswer: "to become happy",
    noteId: BigInt(2),
    learnedAt: "2026-03-16T00:00:00Z",
  },
];

function renderComponent(props: Partial<Parameters<typeof BatchFeedback>[0]> = {}) {
  const defaults: Parameters<typeof BatchFeedback>[0] = {
    items: mockItems,
    isEtymology: false,
    isFinal: false,
    onContinue: vi.fn(),
    onSeeResults: vi.fn(),
    onOverride: vi.fn(),
    onUndo: vi.fn(),
    onSkip: vi.fn(),
    onResume: vi.fn(),
    ...props,
  };
  return render(
    <ChakraProvider value={defaultSystem}>
      <BatchFeedback {...defaults} />
    </ChakraProvider>
  );
}

describe("BatchFeedback", () => {
  it("shows batch heading when not final", () => {
    renderComponent({ isFinal: false });
    expect(screen.getByText("Batch Feedback")).toBeInTheDocument();
  });

  it("shows Session Complete heading when final", () => {
    renderComponent({ isFinal: true });
    expect(screen.getByText("Session Complete")).toBeInTheDocument();
  });

  it("shows correct and incorrect counts", () => {
    renderComponent();
    expect(screen.getByText("Correct: 1")).toBeInTheDocument();
    expect(screen.getByText("Incorrect: 1")).toBeInTheDocument();
  });

  it("renders each result entry", () => {
    renderComponent();
    expect(screen.getByText("break the ice")).toBeInTheDocument();
    expect(screen.getByText("lose one's temper")).toBeInTheDocument();
  });

  it("shows Continue button when not final", () => {
    renderComponent({ isFinal: false });
    expect(screen.getByRole("button", { name: "Continue" })).toBeInTheDocument();
  });

  it("shows See Results button when final (primary)", () => {
    renderComponent({ isFinal: true });
    const buttons = screen.getAllByRole("button", { name: "See Results" });
    expect(buttons).toHaveLength(1);
  });

  it("shows both Continue and See Results when not final", () => {
    renderComponent({ isFinal: false });
    expect(screen.getByRole("button", { name: "Continue" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "See Results" })).toBeInTheDocument();
  });

  it("calls onContinue when Continue clicked", () => {
    const onContinue = vi.fn();
    renderComponent({ onContinue });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(onContinue).toHaveBeenCalledTimes(1);
  });

  it("calls onSeeResults when See Results clicked (final)", () => {
    const onSeeResults = vi.fn();
    renderComponent({ isFinal: true, onSeeResults });
    fireEvent.click(screen.getByRole("button", { name: "See Results" }));
    expect(onSeeResults).toHaveBeenCalledTimes(1);
  });

  it("renders empty state when no items", () => {
    renderComponent({ items: [] });
    expect(screen.getByText("Correct: 0")).toBeInTheDocument();
    expect(screen.getByText("Incorrect: 0")).toBeInTheDocument();
  });
});
