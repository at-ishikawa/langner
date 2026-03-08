import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import NotebookListPage from "./page";
import * as client from "@/lib/client";

vi.mock("@/lib/client", () => ({
  quizClient: {
    getQuizOptions: vi.fn(),
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <NotebookListPage />
    </ChakraProvider>,
  );
}

describe("NotebookListPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders notebooks from API", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [
        { notebookId: "nb-1", name: "Vocabulary A", reviewCount: 10 },
        { notebookId: "nb-2", name: "Vocabulary B", reviewCount: 5 },
      ],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Notebooks")).toBeInTheDocument();
      expect(screen.getByText("Vocabulary A")).toBeInTheDocument();
      expect(screen.getByText("Vocabulary B")).toBeInTheDocument();
    });

    const linkA = screen.getByText("Vocabulary A").closest("a");
    expect(linkA).toHaveAttribute("href", "/notebooks/nb-1");
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
});
