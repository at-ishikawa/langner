import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import QuizCardPage from "./standard/page";
import * as client from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import type { Flashcard } from "@/store/quizStore";

vi.mock("@/lib/client", () => ({
  quizClient: {
    getQuizOptions: vi.fn(),
    startQuiz: vi.fn(),
    submitAnswer: vi.fn(),
  },
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
    useQuizStore.getState().setQuizType("standard");
    pushMock.mockReset();
  });

  it("redirects to / when no flashcards in store", async () => {
    renderPage();
    await waitFor(() => {
      expect(pushMock).toHaveBeenCalledWith("/");
    });
  });

  it("redirects to / when quizType is not standard even with flashcards", async () => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    useQuizStore.getState().setQuizType("reverse");
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

  it.each([
    {
      name: "submits answer and shows correct feedback",
      mockResponse: {
        correct: true,
        meaning: "to initiate social interaction",
        reason: "The answer captures the core meaning",
      },
      userAnswer: "start a conversation",
      expectedLabel: "\u2713 Correct",
    },
    {
      name: "submits answer and shows incorrect feedback",
      mockResponse: {
        correct: false,
        meaning: "to initiate social interaction",
        reason: "The answer is not related",
      },
      userAnswer: "to freeze something",
      expectedLabel: "\u2717 Incorrect",
    },
  ])("$name", async ({ mockResponse, userAnswer, expectedLabel }) => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    vi.mocked(client.quizClient.submitAnswer).mockResolvedValue(mockResponse);

    renderPage();

    const input = screen.getByPlaceholderText("Type your answer");
    fireEvent.change(input, { target: { value: userAnswer } });
    fireEvent.click(screen.getByText("Submit"));

    await waitFor(() => {
      expect(screen.getByText(`Your answer: ${userAnswer}`)).toBeInTheDocument();
    });

    await waitFor(() => {
      expect(screen.getByText(expectedLabel)).toBeInTheDocument();
      expect(screen.getByText(mockResponse.meaning)).toBeInTheDocument();
      expect(screen.getByText(mockResponse.reason)).toBeInTheDocument();
    });

    expect(client.quizClient.submitAnswer).toHaveBeenCalledWith({
      noteId: 1n,
      answer: userAnswer,
      responseTimeMs: expect.any(BigInt),
    });

    const state = useQuizStore.getState();
    expect(state.results).toHaveLength(1);
    expect(state.results[0].correct).toBe(mockResponse.correct);
  });

  it("shows error message when submitAnswer fails", async () => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    vi.mocked(client.quizClient.submitAnswer).mockRejectedValue(
      new Error("network error")
    );

    renderPage();

    const input = screen.getByPlaceholderText("Type your answer");
    fireEvent.change(input, { target: { value: "some answer" } });
    fireEvent.click(screen.getByText("Submit"));

    await waitFor(() => {
      expect(screen.getByText("Failed to submit answer")).toBeInTheDocument();
    });
  });

  it("advances to next card after feedback", async () => {
    useQuizStore.getState().setFlashcards(mockFlashcards);
    vi.mocked(client.quizClient.submitAnswer).mockResolvedValue({
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
    vi.mocked(client.quizClient.submitAnswer).mockResolvedValue({
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
