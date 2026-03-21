import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import EtymologyNotebookPage from "./page";
import * as client from "@/lib/client";

vi.mock("@/lib/client", () => ({
  notebookClient: {
    getEtymologyNotebook: vi.fn(),
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useParams: () => ({ id: "etym-1" }),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <EtymologyNotebookPage />
    </ChakraProvider>,
  );
}

const mockEtymologyResponse = {
  origins: [
    {
      origin: "graph",
      type: "root",
      language: "Greek",
      meaning: "to write",
      wordCount: 4,
    },
    {
      origin: "tele",
      type: "prefix",
      language: "Greek",
      meaning: "far",
      wordCount: 3,
    },
    {
      origin: "scrib",
      type: "root",
      language: "Latin",
      meaning: "to write",
      wordCount: 2,
    },
  ],
  definitions: [
    {
      expression: "telegraph",
      meaning: "a system for transmitting messages over long distances",
      partOfSpeech: "noun",
      note: "",
      originParts: [
        { origin: "tele", type: "prefix", language: "Greek", meaning: "far", wordCount: 3 },
        { origin: "graph", type: "root", language: "Greek", meaning: "to write", wordCount: 4 },
      ],
      notebookName: "vocabulary",
    },
    {
      expression: "autograph",
      meaning: "a person's own signature",
      partOfSpeech: "noun",
      note: "",
      originParts: [
        { origin: "auto", type: "prefix", language: "Greek", meaning: "self", wordCount: 2 },
        { origin: "graph", type: "root", language: "Greek", meaning: "to write", wordCount: 4 },
      ],
      notebookName: "vocabulary",
    },
    {
      expression: "describe",
      meaning: "to give an account of something",
      partOfSpeech: "verb",
      note: "",
      originParts: [
        { origin: "de", type: "prefix", language: "Latin", meaning: "down", wordCount: 1 },
        { origin: "scrib", type: "root", language: "Latin", meaning: "to write", wordCount: 2 },
      ],
      notebookName: "vocabulary",
    },
  ],
  meaningGroups: [
    {
      meaning: "to write",
      origins: [
        { origin: "graph", type: "root", language: "Greek", meaning: "to write", wordCount: 4 },
        { origin: "scrib", type: "root", language: "Latin", meaning: "to write", wordCount: 2 },
      ],
    },
    {
      meaning: "far",
      origins: [
        { origin: "tele", type: "prefix", language: "Greek", meaning: "far", wordCount: 3 },
      ],
    },
  ],
  originCount: 3,
  definitionCount: 3,
};

describe("EtymologyNotebookPage - Origin List", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders origin list with back link and title", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Etymology")).toBeInTheDocument();
      expect(screen.getByText("< Learn")).toBeInTheDocument();
    });
    const backLink = screen.getByText("< Learn").closest("a");
    expect(backLink).toHaveAttribute("href", "/learn");
  });

  it("renders origin cards with type and language badges", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("graph")).toBeInTheDocument();
      expect(screen.getByText("tele")).toBeInTheDocument();
      expect(screen.getByText("scrib")).toBeInTheDocument();
    });
    // Type badges
    expect(screen.getAllByText("root").length).toBeGreaterThan(0);
    expect(screen.getAllByText("prefix").length).toBeGreaterThan(0);
    // Language badges
    expect(screen.getAllByText("Greek").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Latin").length).toBeGreaterThan(0);
    // Word counts
    expect(screen.getByText("4 words")).toBeInTheDocument();
    expect(screen.getByText("3 words")).toBeInTheDocument();
    expect(screen.getByText("2 words")).toBeInTheDocument();
  });

  it("renders summary footer with origin and word counts", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/3 origins .* 3 words/)).toBeInTheDocument();
    });
  });

  it("filters origins by search text", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => expect(screen.getByText("graph")).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText("Search origins or meanings..."), {
      target: { value: "tele" },
    });

    await waitFor(() => {
      expect(screen.getByText("tele")).toBeInTheDocument();
      expect(screen.queryByText("graph")).not.toBeInTheDocument();
      expect(screen.queryByText("scrib")).not.toBeInTheDocument();
    });
  });

  it("shows empty state when search matches nothing", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => expect(screen.getByText("graph")).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText("Search origins or meanings..."), {
      target: { value: "nonexistent" },
    });

    await waitFor(() => {
      expect(screen.getByText("No origins match your search.")).toBeInTheDocument();
    });
  });

  it("shows error message when API fails", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockRejectedValue(new Error("network error"));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Failed to load etymology notebook")).toBeInTheDocument();
    });
  });
});

describe("EtymologyNotebookPage - By Meaning tab", () => {
  beforeEach(() => vi.clearAllMocks());

  it("switches to By Meaning tab and shows grouped origins", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => expect(screen.getByText("graph")).toBeInTheDocument());

    fireEvent.click(screen.getByText("By Meaning"));

    await waitFor(() => {
      // Meaning headings are quoted
      expect(screen.getByText(/to write/)).toBeInTheDocument();
      expect(screen.getByText(/far/)).toBeInTheDocument();
    });
  });
});

describe("EtymologyNotebookPage - Origin Detail", () => {
  beforeEach(() => vi.clearAllMocks());

  it("navigates to origin detail when clicking an origin", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => expect(screen.getByText("graph")).toBeInTheDocument());

    fireEvent.click(screen.getByText("graph"));

    await waitFor(() => {
      expect(screen.getByText("Origin Detail")).toBeInTheDocument();
      expect(screen.getByText("< Origin List")).toBeInTheDocument();
      expect(screen.getByText("to write")).toBeInTheDocument();
      // Shows related words
      expect(screen.getByText("telegraph")).toBeInTheDocument();
      expect(screen.getByText("autograph")).toBeInTheDocument();
      // Does not show unrelated word
      expect(screen.queryByText("describe")).not.toBeInTheDocument();
    });
  });

  it("shows current origin highlighted in word breakdown", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => expect(screen.getByText("graph")).toBeInTheDocument());

    fireEvent.click(screen.getByText("graph"));

    await waitFor(() => {
      expect(screen.getAllByText("(current)").length).toBeGreaterThan(0);
    });
  });

  it("navigates to another origin via word breakdown link", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => expect(screen.getByText("graph")).toBeInTheDocument());

    fireEvent.click(screen.getByText("graph"));

    await waitFor(() => expect(screen.getByText("telegraph")).toBeInTheDocument());

    // Click "tele" in the telegraph breakdown to navigate to tele's detail
    const teleLink = screen.getByText("tele");
    fireEvent.click(teleLink);

    await waitFor(() => {
      // Now showing tele's detail page
      expect(screen.getByText("far")).toBeInTheDocument();
      expect(screen.getByText("telegraph")).toBeInTheDocument();
    });
  });

  it("returns to origin list when clicking back", async () => {
    vi.mocked(client.notebookClient.getEtymologyNotebook).mockResolvedValue(mockEtymologyResponse);
    renderPage();
    await waitFor(() => expect(screen.getByText("graph")).toBeInTheDocument());

    fireEvent.click(screen.getByText("graph"));
    await waitFor(() => expect(screen.getByText("Origin Detail")).toBeInTheDocument());

    fireEvent.click(screen.getByText("< Origin List"));

    await waitFor(() => {
      expect(screen.getByText("Etymology")).toBeInTheDocument();
      expect(screen.queryByText("Origin Detail")).not.toBeInTheDocument();
    });
  });
});
