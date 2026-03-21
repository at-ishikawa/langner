import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import QuizHubPage from "./page";

const mockPush = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
}));

vi.mock("next/link", () => ({
  default: ({ children, ...props }: { children: React.ReactNode; href: string }) => (
    <a {...props}>{children}</a>
  ),
}));

vi.mock("@/lib/client", () => ({
  quizClient: {
    getQuizOptions: vi.fn().mockResolvedValue({ notebooks: [] }),
  },
  EtymologyQuizMode: { BREAKDOWN: 1, ASSEMBLY: 2 },
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <QuizHubPage />
    </ChakraProvider>
  );
}

describe("QuizHubPage", () => {
  beforeEach(() => {
    mockPush.mockClear();
  });

  it("renders Quiz title and back link to Home", async () => {
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Quiz")).toBeInTheDocument();
    });
    const backLink = screen.getByText("< Home").closest("a");
    expect(backLink).toHaveAttribute("href", "/");
  });

  it("shows Vocabulary tab with 3 quiz mode cards by default", async () => {
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Standard")).toBeInTheDocument();
    });
    expect(screen.getByText("See a word, type its meaning")).toBeInTheDocument();
    expect(screen.getByText("Reverse")).toBeInTheDocument();
    expect(screen.getByText("Freeform")).toBeInTheDocument();
  });

  it("selecting a mode highlights it and shows Start button", async () => {
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Standard")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("Standard"));
    expect(screen.getByText("Start")).toBeInTheDocument();
  });

  it("switches to Etymology tab and shows etymology quiz modes", async () => {
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Vocabulary")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("Etymology"));

    expect(screen.getByText("Breakdown")).toBeInTheDocument();
    expect(screen.getByText("See a word, identify its origins and meanings")).toBeInTheDocument();
    expect(screen.getByText("Assembly")).toBeInTheDocument();
    expect(screen.getByText("Freeform")).toBeInTheDocument();
  });

  it("deselects mode when clicking it again", async () => {
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Standard")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("Standard"));
    expect(screen.getByText("Start")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Standard"));
    expect(screen.queryByText("Start")).not.toBeInTheDocument();
  });
});
