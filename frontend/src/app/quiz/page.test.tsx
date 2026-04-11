import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
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

function renderPageDark() {
  document.documentElement.classList.add("dark");
  document.documentElement.setAttribute("data-theme", "dark");
  return renderPage();
}

describe("QuizHubPage", () => {
  beforeEach(() => {
    mockPush.mockClear();
  });

  afterEach(() => {
    document.documentElement.classList.remove("dark");
    document.documentElement.removeAttribute("data-theme");
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

    // Etymology tab shows Standard, Reverse, Freeform modes
    const standardTexts = screen.getAllByText("Standard");
    expect(standardTexts.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("See an origin, type its meaning")).toBeInTheDocument();
    const reverseTexts = screen.getAllByText("Reverse");
    expect(reverseTexts.length).toBeGreaterThanOrEqual(1);
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

  it("renders in dark mode without errors", async () => {
    renderPageDark();
    await waitFor(() => {
      expect(screen.getByText("Quiz")).toBeInTheDocument();
      expect(screen.getByText("Standard")).toBeInTheDocument();
      expect(screen.getByText("Reverse")).toBeInTheDocument();
      expect(screen.getByText("Freeform")).toBeInTheDocument();
    });
  });
});
