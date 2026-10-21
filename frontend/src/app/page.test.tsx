import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import TodayPage from "./page";
import * as client from "@/lib/client";

vi.mock("@/lib/client", () => ({
  quizClient: { getQuizOptions: vi.fn() },
}));

vi.mock("next/link", () => ({
  default: ({ children, ...props }: { children: React.ReactNode; href: string }) => (
    <a {...props}>{children}</a>
  ),
}));

function nb(partial: Partial<client.NotebookSummary> & { notebookId: string; name: string }) {
  return {
    reviewCount: 0,
    reverseReviewCount: 0,
    etymologyReviewCount: 0,
    etymologyReverseReviewCount: 0,
    grammarReviewCount: 0,
    vocabularyCount: 0,
    kind: "",
    hasContent: true,
    sections: [],
    ...partial,
  } as client.NotebookSummary;
}

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <TodayPage />
    </ChakraProvider>,
  );
}

describe("TodayPage", () => {
  afterEach(() => {
    document.documentElement.classList.remove("dark");
    document.documentElement.removeAttribute("data-theme");
    vi.clearAllMocks();
  });

  it("shows total due and a one-tap review button to the quiz hub", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [
        nb({ notebookId: "a", name: "Word Power", reviewCount: 14, vocabularyCount: 412 }),
        nb({ notebookId: "b", name: "Latin Roots", reviewCount: 9, vocabularyCount: 230 }),
        nb({ notebookId: "g", name: "Grammar — tenses", grammarReviewCount: 6 }),
      ],
    } as Awaited<ReturnType<typeof client.quizClient.getQuizOptions>>);

    renderPage();

    expect(await screen.findByText("Today")).toBeInTheDocument();
    // 14 + 9 words due across notebooks.
    expect(await screen.findByText("23")).toBeInTheDocument();
    const review = screen.getByText(/Review 23 due/).closest("a");
    expect(review).toHaveAttribute("href", "/quiz");
    // Per-notebook due breakdown.
    expect(screen.getByText("Word Power")).toBeInTheDocument();
    expect(screen.getByText("Latin Roots")).toBeInTheDocument();
  });

  it("lists imported books under Continue reading", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [
        nb({ notebookId: "gutenberg-84", name: "Frankenstein", reviewCount: 3, vocabularyCount: 5 }),
      ],
    } as Awaited<ReturnType<typeof client.quizClient.getQuizOptions>>);

    renderPage();

    expect(await screen.findByText("Continue reading")).toBeInTheDocument();
    // The book is both due and continuable; the Continue-reading entry is the link.
    const book = screen.getByRole("link", { name: "Frankenstein" });
    expect(book).toHaveAttribute("href", "/learn/gutenberg-84");
  });

  it("shows an all-clear state with links when nothing is due", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [nb({ notebookId: "a", name: "Word Power", vocabularyCount: 412 })],
    } as Awaited<ReturnType<typeof client.quizClient.getQuizOptions>>);

    renderPage();

    expect(await screen.findByText(/Nothing due right now/)).toBeInTheDocument();
    const browse = screen.getByText("Browse your words").closest("a");
    expect(browse).toHaveAttribute("href", "/learn");
    const find = screen.getByText("Find a book").closest("a");
    expect(find).toHaveAttribute("href", "/books");
  });

  it("folds duplicate notebook rows so a notebook is counted once", async () => {
    // getQuizOptions returns the same notebookId as more than one row by design
    // (a journal surfaces as a vocabulary row AND a separate grammar row). The
    // Today dashboard, which doesn't split by kind, must count each notebook
    // once — not sum the duplicated rows — and render one list entry per id.
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [
        nb({ notebookId: "a", name: "Reading Journal", reviewCount: 7, vocabularyCount: 100 }),
        nb({ notebookId: "a", name: "Reading Journal", grammarReviewCount: 4, kind: "Grammar" }),
      ],
    } as Awaited<ReturnType<typeof client.quizClient.getQuizOptions>>);

    renderPage();

    // 7 words due, counted once (not 14 from summing the duplicate rows)…
    expect(await screen.findByText(/Review 7 due/)).toBeInTheDocument();
    // …and the grammar facet survives the merge (4 grammar notes).
    expect(screen.getByText("4")).toBeInTheDocument();
    // Exactly one "Reading Journal" row in the due-by-notebook list.
    expect(screen.getAllByText("Reading Journal")).toHaveLength(1);
  });

  it("renders in dark mode without errors", async () => {
    document.documentElement.classList.add("dark");
    document.documentElement.setAttribute("data-theme", "dark");
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [nb({ notebookId: "a", name: "Word Power", reviewCount: 3, vocabularyCount: 10 })],
    } as Awaited<ReturnType<typeof client.quizClient.getQuizOptions>>);

    renderPage();
    expect(await screen.findByText("Today")).toBeInTheDocument();
  });
});
