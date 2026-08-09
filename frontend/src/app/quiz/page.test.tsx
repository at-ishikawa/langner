import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import QuizHubPage from "./page";
import { quizClient } from "@/lib/client";

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

// Minimal NotebookSummary shape the picker reads. Extra proto fields default
// to 0/undefined and are irrelevant to the vocab-tab visibility rule.
function summary(overrides: Record<string, unknown>) {
  return {
    notebookId: "",
    name: "",
    kind: "",
    reviewCount: 0,
    reverseReviewCount: 0,
    etymologyReviewCount: 0,
    etymologyReverseReviewCount: 0,
    grammarReviewCount: 0,
    vocabularyCount: 0,
    hasContent: false,
    sections: [],
    ...overrides,
  };
}

describe("QuizHubPage vocab-tab filtering by vocabularyCount", () => {
  // A definitions book with vocabulary but nothing due (reviewCount 0), an
  // empty journal with no vocabulary, and a Journal+Grammar duplicate id where
  // only the Journal side carries vocabulary.
  const notebooks = [
    summary({ notebookId: "roots-book", name: "Roots Book", kind: "Books", vocabularyCount: 3 }),
    summary({ notebookId: "empty-journal", name: "Empty Journal", kind: "Journal", vocabularyCount: 0 }),
    summary({ notebookId: "j2", name: "Journal With Vocab", kind: "Journal", vocabularyCount: 2 }),
    summary({ notebookId: "j2", name: "Journal With Vocab", kind: "Grammar", vocabularyCount: 0 }),
  ];

  beforeEach(() => {
    vi.mocked(quizClient.getQuizOptions).mockResolvedValue({ notebooks } as never);
  });

  afterEach(() => {
    vi.mocked(quizClient.getQuizOptions).mockResolvedValue({ notebooks: [] } as never);
  });

  async function openPickerWithUnstudied() {
    const { container } = renderPage();
    await waitFor(() => expect(screen.getByText("Standard")).toBeInTheDocument());
    // Select a mode so the notebook picker + "Include unstudied" toggle appear.
    fireEvent.click(screen.getByText("Standard"));
    // Turn the toggle ON: a with-vocabulary notebook that has no due reviews
    // (reviewCount 0) must still appear — the structural gate never removes it.
    // Before the toggle the SR filter hides every reviewCount-0 notebook, so
    // the only checkbox input present is the toggle itself.
    const toggle = await waitFor(() => {
      const input = container.querySelector('input[type="checkbox"]');
      if (!input) throw new Error("toggle not rendered yet");
      return input;
    });
    fireEvent.click(toggle);
  }

  it("shows only notebooks that structurally have vocabulary, with no duplicate id", async () => {
    await openPickerWithUnstudied();

    // With-vocabulary notebooks appear (even the definitions book whose
    // reviewCount is 0) because the "Include unstudied" toggle is ON.
    await waitFor(() => expect(screen.getByText("Roots Book")).toBeInTheDocument());
    // The Journal+Grammar duplicate id collapses to exactly one vocab row: the
    // Journal side has vocabulary, the Grammar side is filtered out by kind.
    expect(screen.getAllByText("Journal With Vocab")).toHaveLength(1);

    // The empty journal (vocabularyCount 0) is gone even with the toggle ON —
    // the structural gate is independent of studied/due state.
    expect(screen.queryByText("Empty Journal")).not.toBeInTheDocument();
  });
});
