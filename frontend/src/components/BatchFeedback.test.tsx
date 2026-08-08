import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { BatchFeedback } from "./BatchFeedback";
import type { ResultItem } from "./QuizResultCard";

const mockItems: ResultItem[] = [
  {
    index: 0,
    key: "1",
    entry: "break the ice",
    meaning: "to initiate conversation",
    correct: true,
    originalCorrect: true,
    userAnswer: "start a conversation",
    noteId: BigInt(1),
    learnedAt: "2026-03-16T00:00:00Z",
  },
  {
    index: 1,
    key: "2",
    entry: "lose one's temper",
    meaning: "to become angry",
    correct: false,
    originalCorrect: false,
    userAnswer: "to become happy",
    noteId: BigInt(2),
    learnedAt: "2026-03-16T00:00:00Z",
  },
];

function renderComponent(props: Partial<Parameters<typeof BatchFeedback>[0]> = {}) {
  const defaults: Parameters<typeof BatchFeedback>[0] = {
    items: mockItems,
    isEtymology: false,
    isFinal: false,
    onContinue: vi.fn(),
    onSeeResults: vi.fn(),
    onOverride: vi.fn(),
    onUndo: vi.fn(),
    onSkip: vi.fn(),
    onResume: vi.fn(),
    ...props,
  };
  return render(
    <ChakraProvider value={defaultSystem}>
      <BatchFeedback {...defaults} />
    </ChakraProvider>
  );
}

describe("BatchFeedback", () => {
  it("shows Feedback heading", () => {
    renderComponent({ isFinal: false });
    expect(screen.getByText("Feedback")).toBeInTheDocument();
  });

  it("shows correct and incorrect counts", () => {
    renderComponent();
    expect(screen.getByText("Correct: 1")).toBeInTheDocument();
    expect(screen.getByText("Incorrect: 1")).toBeInTheDocument();
  });

  it("renders each result entry", () => {
    renderComponent();
    expect(screen.getByText("break the ice")).toBeInTheDocument();
    expect(screen.getByText("lose one's temper")).toBeInTheDocument();
  });

  it("shows Continue button when not final", () => {
    renderComponent({ isFinal: false });
    expect(screen.getByRole("button", { name: "Continue" })).toBeInTheDocument();
  });

  it("shows See Results button when final (primary)", () => {
    renderComponent({ isFinal: true });
    const buttons = screen.getAllByRole("button", { name: "See Results" });
    expect(buttons).toHaveLength(1);
  });

  it("shows both Continue and See Results when not final", () => {
    renderComponent({ isFinal: false });
    expect(screen.getByRole("button", { name: "Continue" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "See Results" })).toBeInTheDocument();
  });

  it("calls onContinue when Continue clicked", () => {
    const onContinue = vi.fn();
    renderComponent({ onContinue });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(onContinue).toHaveBeenCalledTimes(1);
  });

  it("calls onSeeResults when See Results clicked (final)", () => {
    const onSeeResults = vi.fn();
    renderComponent({ isFinal: true, onSeeResults });
    fireEvent.click(screen.getByRole("button", { name: "See Results" }));
    expect(onSeeResults).toHaveBeenCalledTimes(1);
  });

  it("renders empty state when no items", () => {
    renderComponent({ items: [] });
    expect(screen.getByText("Correct: 0")).toBeInTheDocument();
    expect(screen.getByText("Incorrect: 0")).toBeInTheDocument();
  });
});

// Etymology-origin feedback screen: bug-1 regression (no leftover "Fill in
// the blank to complete this concept" concept-cluster block) and bug-2
// coverage (per-word Mark as Correct/Incorrect and Exclude buttons flip only
// the clicked word, not the whole origin or its siblings).
const etymologyItems: ResultItem[] = [
  {
    index: 0,
    key: "10",
    entry: "scribo",
    meaning: "to write",
    correct: false,
    originalCorrect: false,
    noteId: BigInt(10),
    learnedAt: "2026-03-16T00:00:00Z",
    // notebookName is the word's home definitions-book id — the key the
    // per-word Exclude uses (ExcludeEtymologyWord), independent of noteId.
    notebookName: "roots",
    etymologyWords: [
      { expression: "describe", correct: false, correctMeaning: "to represent in words", reason: "", userAnswer: "tell" },
      { expression: "inscribe", correct: true, correctMeaning: "to write or carve on a surface", reason: "", userAnswer: "carve" },
    ],
  },
];

describe("BatchFeedback etymology-origin feedback screen", () => {
  it("does not render the leftover concept-cluster block", () => {
    renderComponent({ items: etymologyItems, isEtymology: true });
    expect(screen.queryByText(/Fill in the blank to complete this concept/i)).not.toBeInTheDocument();
  });

  it("renders a Mark as Correct/Incorrect button per family word", () => {
    renderComponent({ items: etymologyItems, isEtymology: true, onOverrideWord: vi.fn(), onExcludeWord: vi.fn() });
    // Each word button carries an aria-label naming the word, distinct from
    // the root-level "Mark as Correct"/"Mark as Incorrect" button so a
    // screen reader (and this test) can tell them apart.
    expect(screen.getByRole("button", { name: "Mark describe as correct" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mark inscribe as incorrect" })).toBeInTheDocument();
  });

  it("overriding one word calls onOverrideWord with only that word, not its sibling", () => {
    const onOverrideWord = vi.fn();
    renderComponent({ items: etymologyItems, isEtymology: true, onOverrideWord, onExcludeWord: vi.fn() });
    fireEvent.click(screen.getByRole("button", { name: "Mark describe as correct" }));
    expect(onOverrideWord).toHaveBeenCalledTimes(1);
    const [item, word] = onOverrideWord.mock.calls[0];
    expect(item.entry).toBe("scribo");
    expect(word.expression).toBe("describe");
  });

  it("excluding one word calls onExcludeWord for that word only", () => {
    const onExcludeWord = vi.fn();
    renderComponent({ items: etymologyItems, isEtymology: true, onOverrideWord: vi.fn(), onExcludeWord });
    fireEvent.click(screen.getByRole("button", { name: "Exclude describe" }));
    expect(onExcludeWord).toHaveBeenCalledTimes(1);
    expect(onExcludeWord.mock.calls[0][1].expression).toBe("describe");
    // The sibling word's exclude button is untouched.
    expect(screen.getByRole("button", { name: "Exclude inscribe" })).toBeInTheDocument();
  });

  it("omits per-word buttons when the handlers aren't provided", () => {
    renderComponent({ items: etymologyItems, isEtymology: true });
    expect(screen.queryByRole("button", { name: "Mark describe as correct" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Exclude describe" })).not.toBeInTheDocument();
  });

  // UX simplification: origin-level Mark as Correct/Incorrect and Exclude
  // are meaningless for a multi-word etymology-origin card (the per-word
  // actions above are the correct granularity), so they must never render
  // on this quiz mode's feedback screen — regardless of whether the
  // root-level handlers are provided.
  it("never renders the root-level Mark as Correct/Exclude buttons", () => {
    renderComponent({ items: etymologyItems, isEtymology: true, onOverrideWord: vi.fn(), onExcludeWord: vi.fn() });
    expect(screen.queryByRole("button", { name: "Mark as Correct" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Mark as Incorrect" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Exclude" })).not.toBeInTheDocument();
  });

  // Study context (roots-book support): the feedback card renders the origin's
  // english_forms + note and each word's pronunciation, example sentence, and
  // literal gloss. None of these are graded — only the meaning is.
  const contextItems: ResultItem[] = [
    {
      index: 0,
      key: "20",
      entry: "facere",
      meaning: "to make, to do",
      correct: false,
      originalCorrect: false,
      noteId: BigInt(20),
      learnedAt: "2026-03-16T00:00:00Z",
      etymologyEnglishForms: ["fac", "fic", "fect"],
      etymologyNote: "Watch for fic and fect.",
      etymologyWords: [
        {
          expression: "facsimile",
          correct: false,
          correctMeaning: "an exact copy",
          reason: "",
          userAnswer: "fax",
          pronunciation: "fak-SIM-uh-lee",
          literal: 'fac "make" + simile "like" = "make similar"',
          examples: ["The museum displayed a facsimile of the manuscript."],
        },
      ],
    },
  ];

  it("renders origin english_forms, origin note, and per-word pronunciation/example/literal", () => {
    renderComponent({ items: contextItems, isEtymology: true });
    expect(screen.getByText("fac")).toBeInTheDocument();
    expect(screen.getByText("fic")).toBeInTheDocument();
    expect(screen.getByText("fect")).toBeInTheDocument();
    expect(screen.getByText("Watch for fic and fect.")).toBeInTheDocument();
    expect(screen.getByText("/fak-SIM-uh-lee/")).toBeInTheDocument();
    expect(screen.getByText('fac "make" + simile "like" = "make similar"')).toBeInTheDocument();
    expect(screen.getByText(/The museum displayed a facsimile of the manuscript\./)).toBeInTheDocument();
  });
});

// Non-etymology quiz modes keep the root-level Mark as Correct/Exclude
// actions untouched — this UX change is scoped to etymology-origin only.
describe("BatchFeedback non-etymology feedback screen", () => {
  it("still renders the root-level Mark as Correct and Exclude buttons", () => {
    renderComponent({ items: mockItems, isEtymology: false });
    expect(screen.getAllByRole("button", { name: /^Mark as (Correct|Incorrect)$/ }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: "Exclude" }).length).toBeGreaterThan(0);
  });
});
