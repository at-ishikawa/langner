import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import QuizHubPage from "./page";
import * as client from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";

vi.mock("@/lib/client", () => ({
  quizClient: {
    getQuizOptions: vi.fn(),
    startQuiz: vi.fn(),
    startReverseQuiz: vi.fn(),
    startFreeformQuiz: vi.fn(),
  },
  EtymologyQuizMode: { BREAKDOWN: 1, ASSEMBLY: 2 },
}));

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

vi.mock("next/link", () => ({
  default: ({ children, ...props }: { children: React.ReactNode; href: string }) => (
    <a {...props}>{children}</a>
  ),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <QuizHubPage />
    </ChakraProvider>
  );
}

const mockNotebooks = [
  { notebookId: "english-phrases", name: "English Phrases", reviewCount: 2, reverseReviewCount: 1 },
];

const mockFlashcards = [
  {
    noteId: 1n,
    entry: "break the ice",
    examples: [{ text: "She told a joke to break the ice.", speaker: "Alice" }],
  },
];

describe("QuizHubPage inline start", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useQuizStore.getState().reset();
    pushMock.mockReset();
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: mockNotebooks,
    } as ReturnType<typeof client.quizClient.getQuizOptions> extends Promise<infer T> ? T : never);
  });

  it("sets quizType to standard in store when starting standard quiz with stale store state", async () => {
    useQuizStore.getState().setQuizType("reverse");

    vi.mocked(client.quizClient.startQuiz).mockResolvedValue({
      flashcards: mockFlashcards,
    } as ReturnType<typeof client.quizClient.startQuiz> extends Promise<infer T> ? T : never);

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Standard")).toBeInTheDocument();
    });

    const user = userEvent.setup();

    // Select Standard mode
    await user.click(screen.getByText("Standard"));

    // Select notebook
    await waitFor(() => {
      expect(screen.getByText("English Phrases")).toBeInTheDocument();
    });

    const checkbox = screen.getByRole("checkbox", { name: /English Phrases/ });
    await user.click(checkbox);

    const startButton = screen.getByRole("button", { name: "Start" });
    await user.click(startButton);

    await waitFor(() => {
      expect(useQuizStore.getState().quizType).toBe("standard");
      expect(pushMock).toHaveBeenCalledWith("/quiz/standard");
    });
  });
});
