import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import TrendsPage from "./page";

const getTrends = vi.fn();
const getQuizOptions = vi.fn();

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock, replace: vi.fn() }),
}));

vi.mock("@/lib/client", () => ({
  analyticsClient: { getTrends: (...args: unknown[]) => getTrends(...args) },
  quizClient: { getQuizOptions: (...args: unknown[]) => getQuizOptions(...args) },
  Granularity: { UNSPECIFIED: 0, DAY: 1, WEEK: 2, MONTH: 3 },
  TrendGroupBy: { UNSPECIFIED: 0, QUIZ_TYPE: 1, NOTEBOOK: 2, STATUS: 3, LEVEL: 4 },
}));

function trendsResponse() {
  return {
    buckets: [
      {
        period: "2026-06-01",
        series: [
          { groupKey: "notebook", groupLabel: "Notebook", attempts: 12, wordsTested: 8, wordsLearned: 3, levelUps: 2, lapses: 0 },
          { groupKey: "reverse", groupLabel: "Reverse", attempts: 4, wordsTested: 3, wordsLearned: 1, levelUps: 1, lapses: 0 },
        ],
      },
    ],
    summary: { attempts: 16, wordsTested: 11, wordsLearned: 4, levelUps: 3, lapses: 0 },
    backlog: { neverCorrect: 5, inProgress: 9, mastered: 20 },
  };
}

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <TrendsPage />
    </ChakraProvider>,
  );
}

describe("TrendsPage", () => {
  beforeEach(() => {
    getTrends.mockReset().mockResolvedValue(trendsResponse());
    getQuizOptions.mockReset().mockResolvedValue({ notebooks: [{ notebookId: "flashcards", name: "Flashcards" }] });
  });

  it("renders KPI summary, backlog, and the chart", async () => {
    renderPage();
    // KPI values from the summary, scoped to the KPI strip (chart axis
    // labels can repeat the same digits).
    await waitFor(() => expect(screen.getByTestId("trend-kpis")).toBeInTheDocument());
    const kpis = within(screen.getByTestId("trend-kpis"));
    expect(kpis.getByText("11")).toBeInTheDocument(); // words tested
    expect(kpis.getByText("16")).toBeInTheDocument(); // attempts
    // Backlog snapshot.
    expect(within(screen.getByTestId("trend-backlog")).getByText("20")).toBeInTheDocument(); // mastered
    expect(screen.getByTestId("trend-chart")).toBeInTheDocument();
    expect(screen.getByTestId("trend-legend")).toBeInTheDocument();
  });

  it("forces the LEVEL grouping when the Level-ups metric is selected", async () => {
    renderPage();
    await waitFor(() => expect(getTrends).toHaveBeenCalled());
    // First fetch uses the default split (quiz type = 1), not level.
    expect(getTrends.mock.calls[0][0].groupBy).toBe(1);

    fireEvent.click(screen.getByText("Level-ups"));

    await waitFor(() => {
      const last = getTrends.mock.calls[getTrends.mock.calls.length - 1][0];
      expect(last.groupBy).toBe(4); // TrendGroupBy.LEVEL
    });
  });
});
