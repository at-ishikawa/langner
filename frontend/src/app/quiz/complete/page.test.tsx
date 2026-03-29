import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import SessionCompletePage from "./page";
import { useQuizStore, type QuizResult, type FreeformResult } from "@/store/quizStore";
import * as client from "@/lib/client";

vi.mock("@/lib/client", () => ({
  quizClient: {
    overrideAnswer: vi.fn(),
    undoOverrideAnswer: vi.fn(),
    skipWord: vi.fn(),
    resumeWord: vi.fn(),
  },
  QuizType: { STANDARD: 1, REVERSE: 2, FREEFORM: 3 },
}));

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <SessionCompletePage />
    </ChakraProvider>
  );
}

const mockResults: QuizResult[] = [
  {
    noteId: BigInt(1),
    entry: "break the ice",
    answer: "to initiate conversation",
    correct: true,
    meaning: "to initiate social interaction",
    reason: "close enough",
  },
  {
    noteId: BigInt(2),
    entry: "lose one's temper",
    answer: "to become happy",
    correct: false,
    meaning: "to become angry",
    reason: "opposite meaning",
  },
  {
    noteId: BigInt(3),
    entry: "hit the road",
    answer: "to leave",
    correct: true,
    meaning: "to depart or leave",
    reason: "correct",
  },
];

function renderPageDark() {
  document.documentElement.classList.add("dark");
  document.documentElement.setAttribute("data-theme", "dark");
  return renderPage();
}

describe("SessionCompletePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useQuizStore.getState().reset();
    pushMock.mockReset();
  });

  afterEach(() => {
    document.documentElement.classList.remove("dark");
    document.documentElement.removeAttribute("data-theme");
  });

  it("redirects to / when no results in store", () => {
    renderPage();

    expect(pushMock).toHaveBeenCalledWith("/");
  });

  it.each([
    {
      name: "displays summary counts",
      expectedTexts: ["Total: 3 words", "Correct: 2", "Incorrect: 1"],
    },
    {
      name: "displays correct words with meanings",
      expectedTexts: ["break the ice", "to initiate social interaction", "hit the road", "to depart or leave"],
    },
    {
      name: "displays incorrect words with meanings",
      expectedTexts: ["lose one's temper", "to become angry"],
    },
  ])("$name", ({ expectedTexts }) => {
    useQuizStore.setState({ results: mockResults });
    renderPage();
    for (const text of expectedTexts) {
      expect(screen.getByText(text)).toBeInTheDocument();
    }
  });

  it("resets store and navigates to /quiz on Back to Start click", () => {
    useQuizStore.setState({ results: mockResults });

    renderPage();

    fireEvent.click(screen.getByText("Back to Start"));

    const state = useQuizStore.getState();
    expect(state.results).toHaveLength(0);
    expect(state.flashcards).toHaveLength(0);
    expect(state.currentIndex).toBe(0);
    expect(pushMock).toHaveBeenCalledWith("/quiz");
  });

  it("shows Incorrect section before Correct section", () => {
    useQuizStore.setState({ results: mockResults });
    renderPage();

    const incorrectHeading = screen.getByRole("heading", { name: "Incorrect" });
    const correctHeading = screen.getByRole("heading", { name: "Correct" });

    // Compare DOM order: Incorrect should appear before Correct
    const result = incorrectHeading.compareDocumentPosition(correctHeading);
    // DOCUMENT_POSITION_FOLLOWING means correctHeading follows incorrectHeading
    expect(result & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  // Next review dates are shown on per-question feedback screens, not the complete page

  it.each([
    {
      name: "hides correct section when all answers are incorrect",
      results: mockResults.filter((r) => !r.correct),
      visibleHeading: "Incorrect",
      hiddenHeading: "Correct",
    },
    {
      name: "hides incorrect section when all answers are correct",
      results: mockResults.filter((r) => r.correct),
      visibleHeading: "Correct",
      hiddenHeading: "Incorrect",
    },
  ])("$name", ({ results, visibleHeading, hiddenHeading }) => {
    useQuizStore.setState({ results });
    renderPage();
    expect(screen.getByRole("heading", { name: visibleHeading })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: hiddenHeading })).not.toBeInTheDocument();
  });

  describe("override and skip with RPCs", () => {
    const resultsWithLearnedAt: QuizResult[] = [
      {
        ...mockResults[0],
        learnedAt: "2026-03-16T00:00:00Z",
        nextReviewDate: "2027-06-15",
      },
      {
        ...mockResults[1],
        learnedAt: "2026-03-16T00:00:00Z",
        nextReviewDate: "2027-06-20",
      },
    ];

    it("override button calls quizClient.overrideAnswer and moves card between sections", async () => {
      vi.mocked(client.quizClient.overrideAnswer).mockResolvedValue({
        nextReviewDate: "2027-06-25",
        originalQuality: 5,
        originalStatus: "understood",
        originalIntervalDays: 10,
      });
      useQuizStore.setState({ results: resultsWithLearnedAt, quizType: "standard" });
      renderPage();

      // Initially: 1 correct, 1 incorrect
      expect(screen.getByText("Correct: 1")).toBeInTheDocument();
      expect(screen.getByText("Incorrect: 1")).toBeInTheDocument();

      // Click "Mark as Correct" on incorrect card
      fireEvent.click(screen.getByText("Mark as Correct"));

      await waitFor(() => {
        expect(client.quizClient.overrideAnswer).toHaveBeenCalledWith(
          expect.objectContaining({ noteId: 2n, markCorrect: true })
        );
      });

      // After override: 2 correct, 0 incorrect
      await waitFor(() => {
        expect(screen.getByText("Correct: 2")).toBeInTheDocument();
        expect(screen.getByText("Incorrect: 0")).toBeInTheDocument();
      });
    });

    it("skip button calls quizClient.skipWord", async () => {
      vi.mocked(client.quizClient.skipWord).mockResolvedValue({});
      useQuizStore.setState({ results: resultsWithLearnedAt, quizType: "standard" });
      renderPage();

      // Get the Skip buttons - should have 2 (one per card)
      const skipButtons = screen.getAllByText("Exclude from Quizzes");
      fireEvent.click(skipButtons[0]);

      await waitFor(() => {
        expect(client.quizClient.skipWord).toHaveBeenCalledWith({ noteId: 2n });
      });

      // Skipped card shows "Skipped" badge
      await waitFor(() => {
        expect(screen.getByText("Excluded")).toBeInTheDocument();
      });
    });

    it("undo override calls quizClient.undoOverrideAnswer", async () => {
      vi.mocked(client.quizClient.overrideAnswer).mockResolvedValue({
        nextReviewDate: "2027-06-25",
        originalQuality: 5,
        originalStatus: "understood",
        originalIntervalDays: 10,
      });
      vi.mocked(client.quizClient.undoOverrideAnswer).mockResolvedValue({
        correct: false,
        nextReviewDate: "2027-06-20",
      });
      useQuizStore.setState({ results: resultsWithLearnedAt, quizType: "standard" });
      renderPage();

      // Override the incorrect card first
      fireEvent.click(screen.getByText("Mark as Correct"));

      await waitFor(() => {
        expect(screen.getByText("Undo")).toBeInTheDocument();
      });

      // Undo the override
      fireEvent.click(screen.getByText("Undo"));

      await waitFor(() => {
        expect(client.quizClient.undoOverrideAnswer).toHaveBeenCalledWith(
          expect.objectContaining({ noteId: 2n })
        );
      });

      // Card should be back in incorrect section
      await waitFor(() => {
        expect(screen.getByText("Correct: 1")).toBeInTheDocument();
        expect(screen.getByText("Incorrect: 1")).toBeInTheDocument();
      });
    });

    it("resume button calls quizClient.resumeWord and un-skips the card", async () => {
      vi.mocked(client.quizClient.skipWord).mockResolvedValue({});
      vi.mocked(client.quizClient.resumeWord).mockResolvedValue({});
      useQuizStore.setState({ results: resultsWithLearnedAt, quizType: "standard" });
      renderPage();

      // Skip the incorrect card first
      const skipButtons = screen.getAllByText("Exclude from Quizzes");
      fireEvent.click(skipButtons[0]);

      await waitFor(() => {
        expect(screen.getByText("Resume")).toBeInTheDocument();
      });

      // Click Resume
      fireEvent.click(screen.getByText("Resume"));

      await waitFor(() => {
        expect(client.quizClient.resumeWord).toHaveBeenCalledWith({ noteId: 2n });
      });

      // Verify the store was updated (isSkipped cleared)
      await waitFor(() => {
        const storeResults = useQuizStore.getState().results;
        expect(storeResults[0].isSkipped).toBeFalsy();
      });
    });

    // Change date picker is available on per-question feedback screens, not the complete page
  });

  it("renders in dark mode without errors", () => {
    useQuizStore.setState({ results: mockResults });
    renderPageDark();
    expect(screen.getByText("Session Complete")).toBeInTheDocument();
    expect(screen.getByText("Total: 3 words")).toBeInTheDocument();
    expect(screen.getByText("break the ice")).toBeInTheDocument();
  });

  describe("freeform results", () => {
    const mockFreeformResults: FreeformResult[] = [
      {
        word: "hit the hay",
        answer: "to go to sleep",
        correct: true,
        meaning: "to go to bed",
        reason: "close enough",
        notebookName: "English Phrases",
      },
      {
        word: "under the weather",
        answer: "to be happy",
        correct: false,
        meaning: "to feel sick",
        reason: "incorrect meaning",
        notebookName: "English Phrases",
      },
    ];

    it("displays freeform results with summary counts", () => {
      useQuizStore.setState({ freeformResults: mockFreeformResults });
      renderPage();

      expect(screen.getByText("Total: 2 words")).toBeInTheDocument();
      expect(screen.getByText("Correct: 1")).toBeInTheDocument();
      expect(screen.getByText("Incorrect: 1")).toBeInTheDocument();
    });

    it("displays freeform correct and incorrect words", () => {
      useQuizStore.setState({ freeformResults: mockFreeformResults });
      renderPage();

      expect(screen.getByText("hit the hay")).toBeInTheDocument();
      expect(screen.getByText("under the weather")).toBeInTheDocument();
    });
  });
});
