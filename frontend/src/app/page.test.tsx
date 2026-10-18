import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
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
        nb({ notebookId: "a", name: "Word Power Made Easy", reviewCount: 14, vocabularyCount: 412 }),
        nb({ notebookId: "b", name: "Latin & Greek Roots", reviewCount: 9, vocabularyCount: 230 }),
        nb({ notebookId: "g", name: "Grammar — tenses", grammarReviewCount: 6 }),
      ],
    } as Awaited<ReturnType<typeof client.quizClient.getQuizOptions>>);

    renderPage();

    expect(await screen.findByText("Today")).toBeInTheDocument();
    // 14 + 9 words due across notebooks.
    expect(await screen.findByText("23")).toBeInTheDocument();
    const review = screen.getByText(/Review 23 due/).closest("a");
    expect(review).toHaveAttribute("href", "/quiz");
    // Per-notebook due breakdown, highest first.
    expect(screen.getByText("Word Power Made Easy")).toBeInTheDocument();
    expect(screen.getByText("Latin & Greek Roots")).toBeInTheDocument();
  });

  it("shows an all-clear state with a Words link when nothing is due", async () => {
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [nb({ notebookId: "a", name: "Word Power Made Easy", vocabularyCount: 412 })],
    } as Awaited<ReturnType<typeof client.quizClient.getQuizOptions>>);

    renderPage();

    expect(await screen.findByText(/Nothing due right now/)).toBeInTheDocument();
    const browse = screen.getByText("Browse your words").closest("a");
    expect(browse).toHaveAttribute("href", "/learn");
  });

  it("renders in dark mode without errors", async () => {
    document.documentElement.classList.add("dark");
    document.documentElement.setAttribute("data-theme", "dark");
    vi.mocked(client.quizClient.getQuizOptions).mockResolvedValue({
      notebooks: [nb({ notebookId: "a", name: "Word Power Made Easy", reviewCount: 3, vocabularyCount: 10 })],
    } as Awaited<ReturnType<typeof client.quizClient.getQuizOptions>>);

    renderPage();
    expect(await screen.findByText("Today")).toBeInTheDocument();
  });
});
