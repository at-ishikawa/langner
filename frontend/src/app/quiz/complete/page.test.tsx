import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import SessionCompletePage from "./page";
import { useQuizStore, type QuizResult, type FreeformResult } from "@/store/quizStore";

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

describe("SessionCompletePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useQuizStore.getState().reset();
    pushMock.mockReset();
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

  it("resets store and navigates to / on Back to Start click", () => {
    useQuizStore.setState({ results: mockResults });

    renderPage();

    fireEvent.click(screen.getByText("Back to Start"));

    const state = useQuizStore.getState();
    expect(state.results).toHaveLength(0);
    expect(state.flashcards).toHaveLength(0);
    expect(state.currentIndex).toBe(0);
    expect(pushMock).toHaveBeenCalledWith("/");
  });

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
