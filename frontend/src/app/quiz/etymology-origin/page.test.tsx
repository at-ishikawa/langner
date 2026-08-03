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

  // The answering screen has no skip affordance: there is no whole-card
  // "Don't Know" button and no per-word "Skip" toggle. Leaving a word blank
  // and tapping Submit is the only way to record "I don't know this one",
  // and the backend grades it incorrect.
  it("has no Skip toggle and no Don't Know button on the answering screen", () => {
    renderPage();

    expect(screen.getByRole("button", { name: "Submit" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /don'?t know/i })).not.toBeInTheDocument();
    for (const w of fourWordCard.words) {
      expect(screen.queryByRole("button", { name: `Skip ${w.expression}` })).not.toBeInTheDocument();
    }
  });

  // Type real answers for 2 of 4 words, leave 2 blank, tap Submit. Each typed
  // word is sent with its real answer; each blank word is sent with an empty
  // answer (no "skipped" field) so the backend grades it incorrect. The
  // feedback screen shows the blank words as incorrect, never as "(skipped)".
  it("submits typed answers as-is and blanks as empty answers graded incorrect", async () => {
    batchSubmitEtymologyOriginAnswers.mockResolvedValue({
      responses: [
        {
          noteId: BigInt(42),
          origin: "graph",
          meaning: "to write",
          correct: false,
          nextReviewDate: "",
          learnedAt: "2026-08-02",
          senseId: "",
          results: [
            { wordId: BigInt(1), expression: "photograph", correct: true, correctMeaning: "light writing", reason: "" },
            { wordId: BigInt(2), expression: "autograph", correct: false, correctMeaning: "self writing", reason: "not quite" },
            { wordId: BigInt(3), expression: "telegraph", correct: false, correctMeaning: "distant writing", reason: "not answered" },
            { wordId: BigInt(4), expression: "paragraph", correct: false, correctMeaning: "beside writing", reason: "not answered" },
          ],
        },
      ],
    });

    renderPage();

    fireEvent.change(screen.getByLabelText("Meaning of photograph"), { target: { value: "light writing" } });
    fireEvent.change(screen.getByLabelText("Meaning of autograph"), { target: { value: "a guess" } });
    // telegraph and paragraph are left blank.

    fireEvent.click(screen.getByRole("button", { name: "Submit" }));

    await waitFor(() => expect(batchSubmitEtymologyOriginAnswers).toHaveBeenCalledTimes(1));
    const request = batchSubmitEtymologyOriginAnswers.mock.calls[0][0];
    const wordAnswers = request.answers[0].answers as { wordId: bigint; answer: string }[];

    expect(wordAnswers.find((w) => w.wordId === BigInt(1))?.answer).toBe("light writing");
    expect(wordAnswers.find((w) => w.wordId === BigInt(2))?.answer).toBe("a guess");
    expect(wordAnswers.find((w) => w.wordId === BigInt(3))?.answer).toBe("");
    expect(wordAnswers.find((w) => w.wordId === BigInt(4))?.answer).toBe("");
    // The request must not carry any skip flag.
    expect(wordAnswers.every((w) => !("skipped" in w))).toBe(true);

    // The feedback screen shows the blank words graded incorrect, not skipped.
    expect(await screen.findByText("photograph")).toBeInTheDocument();
    expect(screen.getByText("telegraph")).toBeInTheDocument();
    expect(screen.queryByText(/\(skipped\)/)).not.toBeInTheDocument();
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

  // Submitting with nothing typed grades every word incorrect: the request
  // carries an empty answer for each word (no skip flag).
  it("submits all-empty answers when Submit is tapped with nothing typed", async () => {
    batchSubmitEtymologyOriginAnswers.mockResolvedValue({
      responses: [
        {
          noteId: BigInt(42),
          origin: "graph",
          meaning: "to write",
          correct: false,
          nextReviewDate: "",
          learnedAt: "2026-08-02",
          senseId: "",
          results: fourWordCard.words.map((w) => ({
            wordId: w.wordId,
            expression: w.expression,
            correct: false,
            correctMeaning: "",
            reason: "not answered",
          })),
        },
      ],
    });

    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));

    await waitFor(() => expect(batchSubmitEtymologyOriginAnswers).toHaveBeenCalledTimes(1));
    const request = batchSubmitEtymologyOriginAnswers.mock.calls[0][0];
    const wordAnswers = request.answers[0].answers as { answer: string }[];
    expect(wordAnswers.every((w) => w.answer === "")).toBe(true);
    expect(wordAnswers.every((w) => !("skipped" in w))).toBe(true);
  });
});
