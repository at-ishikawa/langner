import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import NotebookDetailPage from "./page";
import * as client from "@/lib/client";

vi.mock("@/lib/client", () => ({
  notebookClient: {
    getNotebookDetail: vi.fn(),
    exportNotebookPDF: vi.fn(),
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useParams: () => ({ id: "nb-1" }),
  useSearchParams: () => new URLSearchParams(),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <NotebookDetailPage />
    </ChakraProvider>,
  );
}

const mockStoryNotebook = {
  notebookId: "nb-1",
  name: "Vocabulary Notebook",
  totalWordCount: 2,
  stories: [
    {
      event: "Episode One",
      metadata: { series: "Test Series", season: 1, episode: 1 },
      date: "2025-01-15",
      scenes: [
        {
          title: "Opening Scene",
          statements: [],
          conversations: [
            { speaker: "Alice", quote: "I need to {{ break the ice }} at this party." },
          ],
          definitions: [
            {
              expression: "break the ice",
              definition: "",
              meaning: "to initiate social interaction",
              partOfSpeech: "idiom",
              pronunciation: "",
              examples: ["She told a joke to break the ice."],
              synonyms: ["warm up"],
              antonyms: [],
              learningStatus: "understood",
              learnedLogs: [
                { status: "understood", learnedAt: "2025-01-10", quality: 4, responseTimeMs: 3000n, quizType: "notebook", intervalDays: 7 },
              ],
              easinessFactor: 2.5,
              nextReviewDate: "2025-01-17",
            },
            {
              expression: "lose one's temper",
              definition: "",
              meaning: "to become angry",
              partOfSpeech: "idiom",
              pronunciation: "",
              examples: [],
              synonyms: [],
              antonyms: [],
              learningStatus: "misunderstood",
              learnedLogs: [],
              easinessFactor: 2.5,
              nextReviewDate: "",
            },
          ],
        },
      ],
    },
  ],
};

// Flashcard-style: single scene with no title
const mockFlashcardNotebook = {
  notebookId: "nb-2",
  name: "Vocab Cards",
  totalWordCount: 2,
  stories: [
    {
      event: "English Vocabulary",
      metadata: { series: "", season: 0, episode: 0 },
      date: "",
      scenes: [
        {
          title: "",
          statements: [],
          conversations: [],
          definitions: [
            {
              expression: "serendipity",
              definition: "",
              meaning: "a happy accident",
              partOfSpeech: "noun",
              pronunciation: "/ˌserənˈdɪpəti/",
              examples: [],
              synonyms: [],
              antonyms: [],
              learningStatus: "usable",
              learnedLogs: [],
              easinessFactor: 2.5,
              nextReviewDate: "",
            },
          ],
        },
      ],
    },
  ],
};

describe("NotebookDetailPage — story list", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders notebook header and story list", async () => {
    vi.mocked(client.notebookClient.getNotebookDetail).mockResolvedValue(mockStoryNotebook);
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Vocabulary Notebook")).toBeInTheDocument();
      expect(screen.getAllByText("2 words").length).toBeGreaterThan(0);
      expect(screen.getByText("Episode One")).toBeInTheDocument();
      // story detail not visible yet
      expect(screen.queryByText("Opening Scene")).not.toBeInTheDocument();
    });
  });

  it("shows story metadata in list row", async () => {
    vi.mocked(client.notebookClient.getNotebookDetail).mockResolvedValue(mockStoryNotebook);
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Series S01E01")).toBeInTheDocument();
    });
  });

  it("filters hides stories with no matching words", async () => {
    vi.mocked(client.notebookClient.getNotebookDetail).mockResolvedValue({
      ...mockStoryNotebook,
      stories: [
        mockStoryNotebook.stories[0],
        {
          event: "Episode Two",
          metadata: { series: "Test Series", season: 1, episode: 2 },
          date: "",
          scenes: [{
            title: "Scene",
            statements: [],
            conversations: [],
            definitions: [{
              expression: "look forward to",
              definition: "",
              meaning: "to anticipate with pleasure",
              partOfSpeech: "",
              pronunciation: "",
              examples: [],
              synonyms: [],
              antonyms: [],
              learningStatus: "intuitive",
              learnedLogs: [],
              easinessFactor: 2.5,
              nextReviewDate: "",
            }],
          }],
        },
      ],
    });
    renderPage();
    await waitFor(() => expect(screen.getByText("Episode One")).toBeInTheDocument());

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "misunderstood" } });

    await waitFor(() => {
      expect(screen.getByText("Episode One")).toBeInTheDocument();
      expect(screen.queryByText("Episode Two")).not.toBeInTheDocument();
    });
  });

  it("shows empty state when filter matches nothing", async () => {
    vi.mocked(client.notebookClient.getNotebookDetail).mockResolvedValue(mockStoryNotebook);
    renderPage();
    await waitFor(() => expect(screen.getByText("Episode One")).toBeInTheDocument());

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "intuitive" } });

    await waitFor(() => {
      expect(screen.getByText("No words match the selected filter.")).toBeInTheDocument();
    });
  });

  it("shows error message when API fails", async () => {
    vi.mocked(client.notebookClient.getNotebookDetail).mockRejectedValue(new Error("network error"));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Failed to load notebook")).toBeInTheDocument();
    });
  });

  it("has Export PDF button", async () => {
    vi.mocked(client.notebookClient.getNotebookDetail).mockResolvedValue(mockStoryNotebook);
    renderPage();
    await waitFor(() => expect(screen.getByText("Export PDF")).toBeInTheDocument());
  });

  it("has back to notebooks link", async () => {
    vi.mocked(client.notebookClient.getNotebookDetail).mockResolvedValue(mockStoryNotebook);
    renderPage();
    await waitFor(() => {
      const link = screen.getByText("← Back to notebooks").closest("a");
      expect(link).toHaveAttribute("href", "/notebooks");
    });
  });
});

describe("NotebookDetailPage — story detail (story notebook)", () => {
  beforeEach(() => vi.clearAllMocks());

  async function openStory() {
    vi.mocked(client.notebookClient.getNotebookDetail).mockResolvedValue(mockStoryNotebook);
    renderPage();
    await waitFor(() => expect(screen.getByText("Episode One")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Episode One"));
  }

  it("navigates into story and shows scene list", async () => {
    await openStory();
    await waitFor(() => {
      expect(screen.getByText("Opening Scene")).toBeInTheDocument();
      // words not yet visible (scene collapsed)
      expect(screen.queryByText("to initiate social interaction")).not.toBeInTheDocument();
    });
  });

  it("back button returns to story list", async () => {
    await openStory();
    await waitFor(() => expect(screen.getByText("Opening Scene")).toBeInTheDocument());

    fireEvent.click(screen.getByText("← Vocabulary Notebook"));

    await waitFor(() => {
      expect(screen.getByText("Episode One")).toBeInTheDocument();
      expect(screen.queryByText("Opening Scene")).not.toBeInTheDocument();
    });
  });

  it("expands scene to show conversations and word cards", async () => {
    await openStory();
    await waitFor(() => expect(screen.getByText("Opening Scene")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Opening Scene"));

    await waitFor(() => {
      expect(screen.getByText("Alice:")).toBeInTheDocument();
      expect(screen.getAllByText("break the ice").length).toBeGreaterThan(0);
      expect(screen.getByText("to initiate social interaction")).toBeInTheDocument();
    });
  });

  it("renders highlighted expression in conversation quote", async () => {
    await openStory();
    await waitFor(() => expect(screen.getByText("Opening Scene")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Opening Scene"));

    await waitFor(() => {
      const highlighted = screen.getByText("break the ice", { selector: "span" });
      expect(highlighted).toBeInTheDocument();
    });
  });

  it("expands word card to show learning history", async () => {
    await openStory();
    await waitFor(() => expect(screen.getByText("Opening Scene")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Opening Scene"));
    await waitFor(() => expect(screen.getAllByText("break the ice").length).toBeGreaterThan(0));

    const expressions = screen.getAllByText("break the ice");
    const cardExpression = expressions.find((el) => el.tagName === "P");
    fireEvent.click(cardExpression!);

    await waitFor(() => {
      expect(screen.getByText("Learning History:")).toBeInTheDocument();
      expect(screen.getByText("2025-01-10")).toBeInTheDocument();
      expect(screen.getByText("idiom")).toBeInTheDocument();
    });
  });
});

describe("NotebookDetailPage — story detail (flashcard notebook)", () => {
  beforeEach(() => vi.clearAllMocks());

  async function openFlashcardStory() {
    vi.mocked(client.notebookClient.getNotebookDetail).mockResolvedValue(mockFlashcardNotebook);
    renderPage();
    await waitFor(() => expect(screen.getByText("English Vocabulary")).toBeInTheDocument());
    fireEvent.click(screen.getByText("English Vocabulary"));
  }

  it("shows words directly without a scene level", async () => {
    await openFlashcardStory();
    await waitFor(() => {
      // no scene row shown
      expect(screen.queryByText("—")).not.toBeInTheDocument();
      // words shown directly
      expect(screen.getByText("serendipity")).toBeInTheDocument();
      expect(screen.getByText("a happy accident")).toBeInTheDocument();
    });
  });
});
