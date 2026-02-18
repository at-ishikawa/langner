import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import QuizCardPage from "./page";
import * as client from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import type { Flashcard } from "@/store/quizStore";

vi.mock("@/lib/client", () => ({
  getQuizOptions: vi.fn(),
  startQuiz: vi.fn(),
  submitAnswer: vi.fn(),
}));

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <QuizCardPage />
    </ChakraProvider>
  );
}

const mockFlashcards: Flashcard[] = [
  {
    noteId: BigInt(1),
    entry: "break the ice",
    examples: [
      { text: "She told a joke to break the ice.", speaker: "Rachel" },
      { text: "It was hard to break the ice at the meeting.", speaker: "" },
    ],
  },
  {
    noteId: BigInt(2),
    entry: "lose one's temper",
    examples: [
      {
        text: "Try not to lose your temper during the debate.",
        speaker: "",
      },
    ],
  },
];

describe("QuizCardPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useQuizStore.getState().reset();
    pushMock.mockReset();
  });

  it("redirects to / when no flashcards in store", async () => {
    renderPage();
    await waitFor(() => {
      expect(pushMock).toHaveBeenCalledWith("/");
    });
  });

  it("renders quiz card with word and examples", () => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    renderPage();

    expect(screen.getByText("break the ice")).toBeInTheDocument();
    expect(
      screen.getByText('Rachel: "She told a joke to break the ice."')
    ).toBeInTheDocument();
    expect(
      screen.getByText('"It was hard to break the ice at the meeting."')
    ).toBeInTheDocument();
    expect(screen.getByText("1 / 2")).toBeInTheDocument();
  });

  it("submits answer and shows correct feedback", async () => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    vi.mocked(client.submitAnswer).mockResolvedValue({
      correct: true,
      meaning: "to initiate social interaction",
      reason: "The answer captures the core meaning",
    });

    renderPage();

    const input = screen.getByPlaceholderText("Type your answer");
    fireEvent.change(input, { target: { value: "start a conversation" } });
    fireEvent.click(screen.getByText("Submit"));

    // Optimistic: answer shown immediately with loading spinner
    await waitFor(() => {
      expect(
        screen.getByText("Your answer: start a conversation")
      ).toBeInTheDocument();
    });

    // After RPC response
    await waitFor(() => {
      expect(screen.getByText("\u2713 Correct")).toBeInTheDocument();
      expect(
        screen.getByText("to initiate social interaction")
      ).toBeInTheDocument();
      expect(
        screen.getByText("The answer captures the core meaning")
      ).toBeInTheDocument();
    });

    expect(client.submitAnswer).toHaveBeenCalledWith({
      noteId: "1",
      answer: "start a conversation",
      responseTimeMs: expect.any(String),
    });

    // Result stored in Zustand
    const state = useQuizStore.getState();
    expect(state.results).toHaveLength(1);
    expect(state.results[0].correct).toBe(true);
  });

  it("submits answer and shows incorrect feedback", async () => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    vi.mocked(client.submitAnswer).mockResolvedValue({
      correct: false,
      meaning: "to initiate social interaction",
      reason: "The answer is not related",
    });

    renderPage();

    const input = screen.getByPlaceholderText("Type your answer");
    fireEvent.change(input, { target: { value: "to freeze something" } });
    fireEvent.click(screen.getByText("Submit"));

    await waitFor(() => {
      expect(screen.getByText("\u2717 Incorrect")).toBeInTheDocument();
      expect(
        screen.getByText("Your answer: to freeze something")
      ).toBeInTheDocument();
      expect(
        screen.getByText("to initiate social interaction")
      ).toBeInTheDocument();
      expect(
        screen.getByText("The answer is not related")
      ).toBeInTheDocument();
    });

    const state = useQuizStore.getState();
    expect(state.results).toHaveLength(1);
    expect(state.results[0].correct).toBe(false);
  });

  it("advances to next card after feedback", async () => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    vi.mocked(client.submitAnswer).mockResolvedValue({
      correct: true,
      meaning: "to initiate social interaction",
      reason: "correct",
    });

    renderPage();

    const input = screen.getByPlaceholderText("Type your answer");
    fireEvent.change(input, { target: { value: "start a conversation" } });
    fireEvent.click(screen.getByText("Submit"));

    await waitFor(() => {
      expect(screen.getByText("Next")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Next"));

    expect(screen.getByText("lose one's temper")).toBeInTheDocument();
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("Type your answer")
    ).toBeInTheDocument();
  });

  it("navigates to /quiz/complete after last card", async () => {
    useQuizStore.getState().setFlashcards([mockFlashcards[0]]);
    vi.mocked(client.submitAnswer).mockResolvedValue({
      correct: true,
      meaning: "to initiate social interaction",
      reason: "correct",
    });

    renderPage();

    const input = screen.getByPlaceholderText("Type your answer");
    fireEvent.change(input, { target: { value: "start a conversation" } });
    fireEvent.click(screen.getByText("Submit"));

    await waitFor(() => {
      expect(screen.getByText("See Results")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("See Results"));

    expect(pushMock).toHaveBeenCalledWith("/quiz/complete");
  });
});
