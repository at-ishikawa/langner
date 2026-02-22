import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import QuizStartPage from "./page";
import * as client from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";

vi.mock("@/lib/client", () => ({
  getQuizOptions: vi.fn(),
  startQuiz: vi.fn(),
}));

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <QuizStartPage />
    </ChakraProvider>
  );
}

const mockNotebooks: client.NotebookSummary[] = [
  { notebookId: "nb-1", name: "Vocabulary A", reviewCount: 10 },
  { notebookId: "nb-2", name: "Vocabulary B", reviewCount: 5 },
];

describe("QuizStartPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useQuizStore.getState().reset();
    pushMock.mockReset();
  });

  it("shows loading spinner then renders notebooks", async () => {
    vi.mocked(client.getQuizOptions).mockResolvedValue({
      notebooks: mockNotebooks,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A (10)")).toBeInTheDocument();
      expect(screen.getByText("Vocabulary B (5)")).toBeInTheDocument();
    });
  });

  it("selects individual notebooks and updates total due", async () => {
    vi.mocked(client.getQuizOptions).mockResolvedValue({
      notebooks: mockNotebooks,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A (10)")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("checkbox", { name: /Vocabulary A/ }));
    expect(screen.getByText("10 words due")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("checkbox", { name: /Vocabulary B/ }));
    expect(screen.getByText("15 words due")).toBeInTheDocument();
  });

  it("toggles all notebooks with the all checkbox", async () => {
    vi.mocked(client.getQuizOptions).mockResolvedValue({
      notebooks: mockNotebooks,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A (10)")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("checkbox", { name: /All notebooks/ }));
    expect(screen.getByText("15 words due")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("checkbox", { name: /All notebooks/ }));
    expect(screen.getByText("0 words due")).toBeInTheDocument();
  });

  it("toggles include unstudied words", async () => {
    vi.mocked(client.getQuizOptions).mockResolvedValue({
      notebooks: mockNotebooks,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Include unstudied words")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Include unstudied words"));
  });

  it("calls StartQuiz and navigates to /quiz on start", async () => {
    vi.mocked(client.getQuizOptions).mockResolvedValue({
      notebooks: mockNotebooks,
    });
    vi.mocked(client.startQuiz).mockResolvedValue({
      flashcards: [
        {
          noteId: "1",
          entry: "hello",
          examples: [{ text: "Hello there", speaker: "" }],
        },
      ],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A (10)")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("checkbox", { name: /All notebooks/ }));
    fireEvent.click(screen.getByText("Start"));

    await waitFor(() => {
      expect(client.startQuiz).toHaveBeenCalledWith({
        notebookIds: ["nb-1", "nb-2"],
        includeUnstudied: false,
      });
      expect(pushMock).toHaveBeenCalledWith("/quiz");
    });

    const state = useQuizStore.getState();
    expect(state.flashcards).toHaveLength(1);
    expect(state.flashcards[0].entry).toBe("hello");
  });

  it("disables start button when no notebooks selected", async () => {
    vi.mocked(client.getQuizOptions).mockResolvedValue({
      notebooks: mockNotebooks,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A (10)")).toBeInTheDocument();
    });

    const startButton = screen.getByText("Start");
    expect(startButton.closest("button")).toBeDisabled();
  });
});
