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

  // Banner tests
  describe("correct/incorrect banner", () => {
    it("shows Correct banner when isCorrect=true", () => {
      renderComponent({ isCorrect: true });
      expect(screen.getByText(/Correct/)).toBeInTheDocument();
    });

    it("shows Incorrect banner when isCorrect=false", () => {
      renderComponent({ isCorrect: false });
      expect(screen.getByText(/Incorrect/)).toBeInTheDocument();
    });

    it("does not show (overridden) label when isOverridden=false", () => {
      renderComponent({ isOverridden: false });
      expect(screen.queryByText("(overridden)")).not.toBeInTheDocument();
    });

    it("shows (overridden) label when isOverridden=true", () => {
      renderComponent({ isOverridden: true });
      expect(screen.getByText("(overridden)")).toBeInTheDocument();
    });
  });

  // Undo tests
  describe("onUndo", () => {
    it("does not show Undo link when isOverridden=false", () => {
      renderComponent({ isOverridden: false, onUndo: vi.fn() });
      expect(screen.queryByText("Undo")).not.toBeInTheDocument();
    });

    it("does not show Undo link when onUndo is not provided", () => {
      renderComponent({ isOverridden: true });
      expect(screen.queryByText("Undo")).not.toBeInTheDocument();
    });

    it("shows Undo link when isOverridden=true and onUndo provided", () => {
      renderComponent({ isOverridden: true, onUndo: vi.fn() });
      expect(screen.getByText("Undo")).toBeInTheDocument();
    });

    it("calls onUndo when Undo link clicked", () => {
      const onUndo = vi.fn();
      renderComponent({ isOverridden: true, onUndo });
      fireEvent.click(screen.getByText("Undo"));
      expect(onUndo).toHaveBeenCalledTimes(1);
    });
  });

  // Children prop tests
  describe("children", () => {
    it("renders children between banner and buttons", () => {
      render(
        <ChakraProvider value={defaultSystem}>
          <FeedbackActions
            isCorrect={true}
            noteId={BigInt(1)}
            isOverridden={false}
            isSkipped={false}
            nextLabel="Next"
            onNext={vi.fn()}
          >
            <div data-testid="child-content">Page-specific content</div>
          </FeedbackActions>
        </ChakraProvider>
      );
      expect(screen.getByTestId("child-content")).toBeInTheDocument();
      expect(screen.getByText("Page-specific content")).toBeInTheDocument();
    });

    it("does not render children slot when no children provided", () => {
      const { container } = renderComponent();
      expect(container.querySelector("[data-testid='child-content']")).not.toBeInTheDocument();
    });
  });

  // onSeeResults tests
  describe("onSeeResults", () => {
    it("renders See Results button when onSeeResults is provided", () => {
      renderComponent({ onSeeResults: vi.fn(), nextLabel: "Next" });
      expect(screen.getByText("See Results")).toBeInTheDocument();
    });

    it("does not render See Results button when onSeeResults is not provided", () => {
      renderComponent({ nextLabel: "Next" });
      expect(screen.queryByText("See Results")).not.toBeInTheDocument();
    });

    it("calls onSeeResults when See Results button clicked", () => {
      const onSeeResults = vi.fn();
      renderComponent({ onSeeResults, nextLabel: "Next" });
      fireEvent.click(screen.getByText("See Results"));
      expect(onSeeResults).toHaveBeenCalledTimes(1);
    });
  });
});
