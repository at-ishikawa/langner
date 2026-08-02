import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import EtymologyOriginPage from "./page";
import { useQuizStore, type EtymologyOriginCard } from "@/store/quizStore";

const batchSubmitEtymologyOriginAnswers = vi.fn();
vi.mock("@/lib/client", () => ({
  quizClient: {
    batchSubmitEtymologyOriginAnswers: (...args: unknown[]) =>
      batchSubmitEtymologyOriginAnswers(...args),
    overrideAnswer: vi.fn(),
  },
  QuizType: { QUIZ_TYPE_UNSPECIFIED: 0, STANDARD: 1, REVERSE: 2, FREEFORM: 3, ETYMOLOGY_ORIGIN: 4, RELEARN: 7, GRAMMAR: 8 },
}));

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: pushMock }) }));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <EtymologyOriginPage />
    </ChakraProvider>,
  );
}

// A 4-word derived family for one origin card, matching the shape the
// server returns for LoadEtymologyOriginCards.
const fourWordCard: EtymologyOriginCard = {
  cardId: BigInt(1),
  origin: "graph",
  type: "root",
  language: "Greek",
  meaning: "to write",
  notebookName: "word-roots",
  sessionTitle: "session-1",
  sense: "",
  forms: [],
  words: [
    { wordId: BigInt(1), expression: "photograph" },
    { wordId: BigInt(2), expression: "autograph" },
    { wordId: BigInt(3), expression: "telegraph" },
    { wordId: BigInt(4), expression: "paragraph" },
  ],
};

describe("EtymologyOriginPage", () => {
  beforeEach(() => {
    batchSubmitEtymologyOriginAnswers.mockReset();
    pushMock.mockReset();
    useQuizStore.getState().reset();
    useQuizStore.getState().setQuizType("etymology-origin");
    useQuizStore.getState().setEtymologyOriginCards([fourWordCard]);
  });

  // Regression test for the exact user-reported repro: type real answers for
  // 2 of 4 family words, leave 2 blank, tap the whole-card "Don't Know"
  // button. Before the fix, handleSkip discarded every typed answer
  // (answers: {}) and force-skipped all 4 words, so the 2 typed answers
  // never reached grading and the feedback screen showed all 4 as skipped.
  it("grades typed words normally and only skips the blank ones when Don't Know is tapped", async () => {
    batchSubmitEtymologyOriginAnswers.mockResolvedValue({
      responses: [
        {
          noteId: BigInt(42),
          origin: "graph",
          meaning: "to write",
          correct: false,
          nextReviewDate: "",
          learnedAt: "",
          senseId: "",
          results: [
            { wordId: BigInt(1), expression: "photograph", correct: true, correctMeaning: "light writing", reason: "", skipped: false },
            { wordId: BigInt(2), expression: "autograph", correct: false, correctMeaning: "self writing", reason: "not quite", skipped: false },
            { wordId: BigInt(3), expression: "telegraph", correct: false, correctMeaning: "distant writing", reason: "skipped by user", skipped: true },
            { wordId: BigInt(4), expression: "paragraph", correct: false, correctMeaning: "beside writing", reason: "skipped by user", skipped: true },
          ],
        },
      ],
    });

    renderPage();

    fireEvent.change(screen.getByLabelText("Meaning of photograph"), { target: { value: "light writing" } });
    fireEvent.change(screen.getByLabelText("Meaning of autograph"), { target: { value: "a guess" } });
    // telegraph and paragraph are left blank.

    fireEvent.click(screen.getByRole("button", { name: "Don't Know" }));

    await waitFor(() => expect(batchSubmitEtymologyOriginAnswers).toHaveBeenCalledTimes(1));
    const request = batchSubmitEtymologyOriginAnswers.mock.calls[0][0];
    const wordAnswers = request.answers[0].answers as {
      wordId: bigint;
      answer: string;
      skipped: boolean;
    }[];

    // The 2 typed words must be sent with their real answers and
    // skipped:false so the backend grades them independently.
    expect(wordAnswers.find((w) => w.wordId === BigInt(1))).toMatchObject({
      answer: "light writing",
      skipped: false,
    });
    expect(wordAnswers.find((w) => w.wordId === BigInt(2))).toMatchObject({
      answer: "a guess",
      skipped: false,
    });
    // Only the 2 blank words are sent as skipped.
    expect(wordAnswers.find((w) => w.wordId === BigInt(3))).toMatchObject({
      skipped: true,
    });
    expect(wordAnswers.find((w) => w.wordId === BigInt(4))).toMatchObject({
      skipped: true,
    });

    // The feedback screen reflects independent per-word grading, not
    // "all skipped".
    expect(await screen.findByText("photograph")).toBeInTheDocument();
    expect(screen.getByText("autograph")).toBeInTheDocument();
    expect(screen.getAllByText(/\(skipped\)/)).toHaveLength(2);
  });

  // Study-context card (roots-book support): carries origin english_forms +
  // note and a word with pronunciation. The answering screen shows the safe
  // hints (pronunciation, english_forms, origin note) but must NOT leak the
  // graded meaning or the answer-adjacent example/literal.
  const contextCard: EtymologyOriginCard = {
    cardId: BigInt(1),
    origin: "facere",
    type: "root",
    language: "Latin",
    meaning: "to make, to do",
    notebookName: "roots",
    sessionTitle: "session-1",
    sense: "",
    forms: [],
    englishForms: ["fac", "fic", "fect"],
    note: "Watch for fic and fect.",
    words: [{ wordId: BigInt(1), expression: "facsimile", pronunciation: "fak-SIM-uh-lee" }],
  };

  it("shows english_forms, origin note, and pronunciation while answering but never the meaning", () => {
    useQuizStore.getState().setEtymologyOriginCards([contextCard]);
    renderPage();

    // Safe study context is visible while typing.
    expect(screen.getByText("fac")).toBeInTheDocument();
    expect(screen.getByText("fic")).toBeInTheDocument();
    expect(screen.getByText("fect")).toBeInTheDocument();
    expect(screen.getByText("Watch for fic and fect.")).toBeInTheDocument();
    expect(screen.getByText("/fak-SIM-uh-lee/")).toBeInTheDocument();

    // The word's meaning is the graded answer — it must not appear anywhere on
    // the answering screen (only the origin's own gloss "to make, to do" does).
    expect(screen.queryByText("an exact copy")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Meaning of facsimile")).toHaveValue("");
  });

  it("still marks every word skipped when Don't Know is tapped with nothing typed", async () => {
    batchSubmitEtymologyOriginAnswers.mockResolvedValue({
      responses: [
        {
          noteId: BigInt(42),
          origin: "graph",
          meaning: "to write",
          correct: false,
          nextReviewDate: "",
          learnedAt: "",
          senseId: "",
          results: fourWordCard.words.map((w) => ({
            wordId: w.wordId,
            expression: w.expression,
            correct: false,
            correctMeaning: "",
            reason: "skipped by user",
            skipped: true,
          })),
        },
      ],
    });

    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Don't Know" }));

    await waitFor(() => expect(batchSubmitEtymologyOriginAnswers).toHaveBeenCalledTimes(1));
    const request = batchSubmitEtymologyOriginAnswers.mock.calls[0][0];
    const wordAnswers = request.answers[0].answers as { skipped: boolean }[];
    expect(wordAnswers.every((w) => w.skipped)).toBe(true);
  });
});
