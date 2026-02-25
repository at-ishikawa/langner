import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import QuizStartPage from "./page";
import * as client from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";

vi.mock("@/lib/client", () => ({
  quizClient: {
    getQuizOptions: vi.fn(),
    startQuiz: vi.fn(),
  },
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

function clickCheckbox(name: RegExp) {
  const checkbox = screen.getByRole("checkbox", { name });
  fireEvent.click(checkbox);
  fireEvent.change(checkbox, { target: { checked: !checkbox.hasAttribute("checked") } });
}

const defaultMockNotebooks = [
  { notebookId: "nb-1", name: "Vocabulary A", reviewCount: 10 },
  { notebookId: "nb-2", name: "Vocabulary B", reviewCount: 5 },
];

describe("QuizStartPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useQuizStore.getState().reset();
    pushMock.mockReset();
  });

  it.each([
    {
      name: "renders notebooks from API",
      mockNotebooks: [
        { notebookId: "nb-1", name: "Vocabulary A", reviewCount: 10 },
        { notebookId: "nb-2", name: "Vocabulary B", reviewCount: 5 },
      ],
      expectedTexts: ["Quiz", "Select notebooks", "Vocabulary A", "10", "Vocabulary B", "5", "0 words due for review"],
    },
    {
      name: "renders empty state when no notebooks",
      mockNotebooks: [],
      expectedTexts: ["Quiz", "Select notebooks", "0 words due for review"],
    },
    {
      name: "renders single notebook",
      mockNotebooks: [
        { notebookId: "nb-1", name: "Grammar Basics", reviewCount: 3 },
      ],
      expectedTexts: ["Grammar Basics", "3", "0 words due for review"],
    },
  ])("$name", async ({ mockNotebooks, expectedTexts }) => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: mockNotebooks,
    });

    renderPage();

    await waitFor(() => {
      for (const text of expectedTexts) {
        expect(screen.getByText(text)).toBeInTheDocument();
      }
    });
  });

  it("selects individual notebooks and updates total due", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: defaultMockNotebooks,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A")).toBeInTheDocument();
    });

    clickCheckbox(/Vocabulary A/);
    await waitFor(() => {
      expect(screen.getByText("10 words due for review")).toBeInTheDocument();
    });

    clickCheckbox(/Vocabulary B/);
    await waitFor(() => {
      expect(screen.getByText("15 words due for review")).toBeInTheDocument();
    });
  });

  it("toggles all notebooks with the all checkbox", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: defaultMockNotebooks,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A")).toBeInTheDocument();
    });

    clickCheckbox(/All notebooks/);
    await waitFor(() => {
      expect(screen.getByText("15 words due for review")).toBeInTheDocument();
    });

    clickCheckbox(/All notebooks/);
    await waitFor(() => {
      expect(screen.getByText("0 words due for review")).toBeInTheDocument();
    });
  });

  it("toggles include unstudied words", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: defaultMockNotebooks,
    });
    vi.mocked(client.quizClient.startQuiz).mockResolvedValue({
      flashcards: [],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Include unstudied words")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Include unstudied words"));

    clickCheckbox(/All notebooks/);
    await waitFor(() => {
      expect(screen.getByText("15 words due for review")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Start"));

    await waitFor(() => {
      expect(client.quizClient.startQuiz).toHaveBeenCalledWith({
        notebookIds: ["nb-1", "nb-2"],
        includeUnstudied: true,
      });
    });
  });

  it("shows error message when getQuizOptions fails", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockRejectedValue(
      new Error("network error")
    );

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Failed to load notebooks")).toBeInTheDocument();
    });
  });

  it("calls StartQuiz and navigates to /quiz on start", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: defaultMockNotebooks,
    });
    vi.mocked(client.quizClient.startQuiz).mockResolvedValue({
      flashcards: [
        {
          noteId: 1n,
          entry: "hello",
          examples: [{ text: "Hello there", speaker: "" }],
        },
      ],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A")).toBeInTheDocument();
    });

    clickCheckbox(/All notebooks/);
    await waitFor(() => {
      expect(screen.getByText("15 words due for review")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Start"));

    await waitFor(() => {
      expect(client.quizClient.startQuiz).toHaveBeenCalledWith({
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
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: defaultMockNotebooks,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A")).toBeInTheDocument();
    });

    const startButton = screen.getByText("Start");
    expect(startButton.closest("button")).toBeDisabled();
  });
});
