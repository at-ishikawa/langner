import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import SessionCompletePage from "./page";
import { useQuizStore, type QuizResult } from "@/store/quizStore";

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

  it("displays summary counts", () => {
    useQuizStore.setState({ results: mockResults });

    renderPage();

    expect(screen.getByText("Total: 3 words")).toBeInTheDocument();
    expect(screen.getByText("Correct: 2")).toBeInTheDocument();
    expect(screen.getByText("Incorrect: 1")).toBeInTheDocument();
  });

  it("displays correct words with meanings", () => {
    useQuizStore.setState({ results: mockResults });

    renderPage();

    expect(screen.getByText("break the ice")).toBeInTheDocument();
    expect(
      screen.getByText("to initiate social interaction")
    ).toBeInTheDocument();
    expect(screen.getByText("hit the road")).toBeInTheDocument();
    expect(screen.getByText("to depart or leave")).toBeInTheDocument();
  });

  it("displays incorrect words with meanings", () => {
    useQuizStore.setState({ results: mockResults });

    renderPage();

    expect(screen.getByText("lose one's temper")).toBeInTheDocument();
    expect(screen.getByText("to become angry")).toBeInTheDocument();
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

  it("hides correct section when all answers are incorrect", () => {
    const allIncorrect = mockResults.filter((r) => !r.correct);
    useQuizStore.setState({ results: allIncorrect });

    renderPage();

    expect(screen.queryByRole("heading", { name: "Correct" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Incorrect" })).toBeInTheDocument();
  });

  it("hides incorrect section when all answers are correct", () => {
    const allCorrect = mockResults.filter((r) => r.correct);
    useQuizStore.setState({ results: allCorrect });

    renderPage();

    expect(screen.getByRole("heading", { name: "Correct" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Incorrect" })).not.toBeInTheDocument();
  });
});
