import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import LearnHubPage from "../learn/page";
import * as client from "@/lib/client";

vi.mock("@/lib/client", () => ({
  quizClient: {
    getQuizOptions: vi.fn(),
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("next/link", () => ({
  default: ({ children, ...props }: { children: React.ReactNode; href: string }) => (
    <a {...props}>{children}</a>
  ),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <LearnHubPage />
    </ChakraProvider>,
  );
}

describe("LearnHubPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders Learn title and back link to Home", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Learn")).toBeInTheDocument();
    });

    const backLink = screen.getByText("< Home").closest("a");
    expect(backLink).toHaveAttribute("href", "/");
  });

  it("renders vocabulary notebooks in Vocabulary tab", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [
        { notebookId: "nb-1", name: "Vocabulary A", reviewCount: 10, kind: "Flashcard" },
        { notebookId: "nb-2", name: "Vocabulary B", reviewCount: 5, kind: "Story" },
      ],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A")).toBeInTheDocument();
      expect(screen.getByText("Vocabulary B")).toBeInTheDocument();
    });

    const linkA = screen.getByText("Vocabulary A").closest("a");
    expect(linkA).toHaveAttribute("href", "/notebooks/nb-1");
  });

  it("renders etymology notebooks in Etymology tab", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [
        { notebookId: "nb-1", name: "Vocabulary A", reviewCount: 10, kind: "Flashcard" },
        { notebookId: "nb-ety", name: "Word Roots", reviewCount: 15, kind: "Etymology" },
      ],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Vocabulary A")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Etymology"));

    await waitFor(() => {
      expect(screen.getByText("Word Roots")).toBeInTheDocument();
    });

    const linkEty = screen.getByText("Word Roots").closest("a");
    expect(linkEty).toHaveAttribute("href", "/notebooks/etymology/nb-ety");
  });

  it("renders empty state when no notebooks", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No notebooks found.")).toBeInTheDocument();
    });
  });

  it("shows error message when API fails", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockRejectedValue(
      new Error("network error"),
    );

    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText("Failed to load notebooks"),
      ).toBeInTheDocument();
    });
  });

  it("renders summary footer with notebook and word counts", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [
        { notebookId: "nb-1", name: "Vocab A", reviewCount: 10, kind: "Flashcard" },
        { notebookId: "nb-2", name: "Vocab B", reviewCount: 5, kind: "Story" },
      ],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText(/2 notebooks/)).toBeInTheDocument();
      expect(screen.getByText(/15 words/)).toBeInTheDocument();
    });
  });
});
