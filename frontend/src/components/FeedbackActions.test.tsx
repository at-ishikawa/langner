import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { FeedbackActions } from "./FeedbackActions";

function renderComponent(props: Partial<Parameters<typeof FeedbackActions>[0]> = {}) {
  const defaultProps: Parameters<typeof FeedbackActions>[0] = {
    isCorrect: true,
    noteId: BigInt(1),
    isOverridden: false,
    isSkipped: false,
    nextLabel: "Next",
    onNext: vi.fn(),
    ...props,
  };

  return render(
    <ChakraProvider value={defaultSystem}>
      <FeedbackActions {...defaultProps} />
    </ChakraProvider>
  );
}

describe("FeedbackActions", () => {
  it("renders Next button with correct label", () => {
    renderComponent({ nextLabel: "Next" });
    expect(screen.getByText("Next")).toBeInTheDocument();
  });

  it("renders See Results button when nextLabel is See Results", () => {
    renderComponent({ nextLabel: "See Results" });
    expect(screen.getByText("See Results")).toBeInTheDocument();
  });

  it("renders Mark as Incorrect button when isCorrect=true and onOverride provided and noteId defined", () => {
    renderComponent({ isCorrect: true, noteId: BigInt(1), onOverride: vi.fn() });
    expect(screen.getByText("Mark as Incorrect")).toBeInTheDocument();
  });

  it("renders Mark as Correct button when isCorrect=false and onOverride provided and noteId defined", () => {
    renderComponent({ isCorrect: false, noteId: BigInt(1), onOverride: vi.fn() });
    expect(screen.getByText("Mark as Correct")).toBeInTheDocument();
  });

  it("does not render override button when isOverridden=true", () => {
    renderComponent({ isOverridden: true, onOverride: vi.fn() });
    expect(screen.queryByText("Mark as Correct")).not.toBeInTheDocument();
    expect(screen.queryByText("Mark as Incorrect")).not.toBeInTheDocument();
  });

  it("does not render override button when noteId is undefined", () => {
    renderComponent({ noteId: undefined, onOverride: vi.fn() });
    expect(screen.queryByText("Mark as Correct")).not.toBeInTheDocument();
    expect(screen.queryByText("Mark as Incorrect")).not.toBeInTheDocument();
  });

  it("renders Skip button when onSkip provided and noteId defined", () => {
    renderComponent({ onSkip: vi.fn(), noteId: BigInt(1) });
    expect(screen.getByText("Exclude from Quizzes")).toBeInTheDocument();
  });

  it("does not render Skip button when isSkipped=true", () => {
    renderComponent({ isSkipped: true, onSkip: vi.fn() });
    expect(screen.queryByText(/^Skip$/)).not.toBeInTheDocument();
  });

  it("shows Skipped dimmed label when isSkipped=true", () => {
    renderComponent({ isSkipped: true, onSkip: vi.fn() });
    expect(screen.getByText("Excluded from quizzes")).toBeInTheDocument();
  });

  it("calls onNext when Next button clicked", () => {
    const onNext = vi.fn();
    renderComponent({ onNext });
    fireEvent.click(screen.getByText("Next"));
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  it("calls onOverride when override button clicked", () => {
    const onOverride = vi.fn();
    renderComponent({ isCorrect: false, onOverride });
    fireEvent.click(screen.getByText("Mark as Correct"));
    expect(onOverride).toHaveBeenCalledTimes(1);
  });

  it("calls onSkip immediately when Skip button clicked (no confirmation)", () => {
    const onSkip = vi.fn();
    renderComponent({ onSkip, noteId: BigInt(1) });
    fireEvent.click(screen.getByText("Exclude from Quizzes"));
    expect(onSkip).toHaveBeenCalledTimes(1);
  });
});
