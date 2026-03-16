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
  it("renders next review date in blue box with formatted date when nextReviewDate is provided", () => {
    renderComponent({ nextReviewDate: "2027-06-15" });
    expect(screen.getByText(/Next review:/)).toBeInTheDocument();
    expect(screen.getByText(/June 15, 2027/)).toBeInTheDocument();
  });

  it("does not render review date box when nextReviewDate is undefined", () => {
    renderComponent({ nextReviewDate: undefined });
    expect(screen.queryByText(/Next review:/)).not.toBeInTheDocument();
  });

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
    expect(screen.getByText("Skip")).toBeInTheDocument();
  });

  it("does not render Skip button when isSkipped=true", () => {
    renderComponent({ isSkipped: true, onSkip: vi.fn() });
    expect(screen.queryByText(/^Skip$/)).not.toBeInTheDocument();
  });

  it("shows Skipped dimmed label when isSkipped=true", () => {
    renderComponent({ isSkipped: true, onSkip: vi.fn() });
    expect(screen.getByText("Skipped")).toBeInTheDocument();
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
    fireEvent.click(screen.getByText("Skip"));
    expect(onSkip).toHaveBeenCalledTimes(1);
  });

  it("shows Change link next to review date when onChangeReviewDate is provided", () => {
    renderComponent({ nextReviewDate: "2027-06-15", onChangeReviewDate: vi.fn() });
    expect(screen.getByText("Change")).toBeInTheDocument();
  });

  it("does not show Change link when onChangeReviewDate is not provided", () => {
    renderComponent({ nextReviewDate: "2027-06-15" });
    expect(screen.queryByText("Change")).not.toBeInTheDocument();
  });

  it("opens date picker when Change link is clicked", () => {
    renderComponent({ nextReviewDate: "2027-06-15", onChangeReviewDate: vi.fn() });
    fireEvent.click(screen.getByText("Change"));
    expect(screen.getByText("Pick a new review date:")).toBeInTheDocument();
    expect(screen.getByText("Save")).toBeInTheDocument();
    expect(screen.getByText("Cancel")).toBeInTheDocument();
  });

  it("calls onChangeReviewDate with new date when Save is clicked", () => {
    const onChangeReviewDate = vi.fn();
    renderComponent({ nextReviewDate: "2027-06-15", onChangeReviewDate });
    fireEvent.click(screen.getByText("Change"));

    const dateInput = document.querySelector('input[type="date"]') as HTMLInputElement;
    fireEvent.change(dateInput, { target: { value: "2027-07-01" } });
    fireEvent.click(screen.getByText("Save"));

    expect(onChangeReviewDate).toHaveBeenCalledWith("2027-07-01");
  });

  it("closes date picker when Cancel is clicked without calling onChangeReviewDate", () => {
    const onChangeReviewDate = vi.fn();
    renderComponent({ nextReviewDate: "2027-06-15", onChangeReviewDate });
    fireEvent.click(screen.getByText("Change"));
    expect(screen.getByText("Pick a new review date:")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Cancel"));
    expect(screen.queryByText("Pick a new review date:")).not.toBeInTheDocument();
    expect(screen.getByText(/Next review:/)).toBeInTheDocument();
    expect(onChangeReviewDate).not.toHaveBeenCalled();
  });

  it("date picker has min set to tomorrow", () => {
    renderComponent({ nextReviewDate: "2027-06-15", onChangeReviewDate: vi.fn() });
    fireEvent.click(screen.getByText("Change"));

    const dateInput = document.querySelector('input[type="date"]') as HTMLInputElement;
    expect(dateInput).toBeTruthy();

    const tomorrow = new Date();
    tomorrow.setDate(tomorrow.getDate() + 1);
    const tomorrowStr = tomorrow.toISOString().split("T")[0];
    expect(dateInput.min).toBe(tomorrowStr);
  });
});
