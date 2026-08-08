import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import RelearnSessionPage from "./page";
import { useRelearnStore } from "@/store/relearnStore";
import type { RelearnCard } from "@/lib/client";

const submitRelearnAnswer = vi.fn();
const skipWord = vi.fn();
vi.mock("@/lib/client", () => ({
  quizClient: {
    submitRelearnAnswer: (...args: unknown[]) => submitRelearnAnswer(...args),
    skipWord: (...args: unknown[]) => skipWord(...args),
  },
  QuizType: { QUIZ_TYPE_UNSPECIFIED: 0, STANDARD: 1, REVERSE: 2, FREEFORM: 3, ETYMOLOGY_ORIGIN: 4, RELEARN: 7, GRAMMAR: 8 },
}));

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: pushMock }) }));

// Isolate the loop from the context block.
vi.mock("@/components/RelearnContext", () => ({ default: () => <div data-testid="ctx" /> }));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <RelearnSessionPage />
    </ChakraProvider>,
  );
}

const card = (entry: string): RelearnCard =>
  ({ entry, noteId: BigInt(entry.length), sourceQuizType: 1, meaning: `${entry}-meaning`, examples: [], contexts: [] }) as RelearnCard;

// A reverse-format card: shown meaning, asks for the word.
const reverseCard = (entry: string, meaning: string): RelearnCard =>
  ({ entry, noteId: BigInt(entry.length), sourceQuizType: 2, meaning, examples: [], contexts: [] }) as RelearnCard;

// An etymology-origin-format card: shown the word, asks for the meaning.
const etymologyCard = (entry: string): RelearnCard =>
  ({ entry, noteId: BigInt(entry.length), sourceQuizType: 4, meaning: `${entry}-meaning`, examples: [], contexts: [] }) as RelearnCard;

// A grammar-format card: one due correction within a journal post. All
// corrections of a post carry the SAME full post text (content); each has its
// own note_id and mistaken span (incorrect).
const grammarCard = (content: string, incorrect: string, noteId: number): RelearnCard =>
  ({
    entry: incorrect,
    noteId: BigInt(noteId),
    sourceQuizType: 8,
    meaning: "",
    examples: [],
    contexts: [],
    content,
    incorrect,
  }) as RelearnCard;

// entries reads the single-card screens' entries; post screens map to undefined.
const entries = () =>
  useRelearnStore
    .getState()
    .queue.map((it) => (it.kind === "card" ? it.card.entry : undefined));

describe("RelearnSessionPage", () => {
  beforeEach(() => {
    submitRelearnAnswer.mockReset();
    skipWord.mockReset();
    skipWord.mockResolvedValue({});
    pushMock.mockReset();
    useRelearnStore.getState().reset();
  });

  it("lets the learner override a wrong grade to correct, clearing the word", async () => {
    useRelearnStore.getState().seedQueue([card("alpha"), card("beta")]);
    submitRelearnAnswer.mockResolvedValue({ correct: false, meaning: "the first", reason: "grader was wrong" });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "knowing all stuff" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByText("✗ Incorrect")).toBeInTheDocument();

    // Override to correct: banner flips, and Next clears the word.
    fireEvent.click(screen.getByRole("button", { name: "Mark as Correct" }));
    expect(screen.getByText("✓ Correct")).toBeInTheDocument();
    expect(screen.getByText("(overridden)")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    // The override reshapes only the working queue — relearn persists nothing,
    // so no RPC is involved. alpha clears despite the wrong grade; beta is next.
    await waitFor(() =>
      expect(entries()).toEqual(["beta"]),
    );
    expect(useRelearnStore.getState().clearedCount).toBe(1);
  });

  it("redirects to start when the queue is empty and nothing was answered", async () => {
    renderPage();
    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/quiz?tab=relearn"));
  });

  it("shows the current card and the words-left counter", () => {
    useRelearnStore.getState().seedQueue([card("alpha"), card("beta")]);
    renderPage();
    expect(screen.getByText("alpha")).toBeInTheDocument();
    expect(screen.getByText("2 words left")).toBeInTheDocument();
    expect(screen.getByText(/Recognition/)).toBeInTheDocument();
  });

  it("renders a reverse-format card as meaning-to-word", () => {
    useRelearnStore.getState().seedQueue([reverseCard("delta", "a change or difference")]);
    renderPage();
    // The meaning is the prompt; the word is NOT shown.
    expect(screen.getByTestId("relearn-prompt")).toHaveTextContent("a change or difference");
    expect(screen.queryByText("delta")).not.toBeInTheDocument();
    expect(screen.getByText(/Reverse/)).toBeInTheDocument();
    expect(screen.getByText("The word")).toBeInTheDocument();
  });

  it("submits a correct answer, shows feedback, and clears the word on Next", async () => {
    useRelearnStore.getState().seedQueue([card("alpha"), card("beta")]);
    submitRelearnAnswer.mockResolvedValue({ correct: true, meaning: "the first", reason: "ok" });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "the first" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));

    expect(await screen.findByText("✓ Correct")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    // alpha cleared, beta now current.
    expect(entries()).toEqual(["beta"]);
    expect(useRelearnStore.getState().clearedCount).toBe(1);
    expect(screen.getByText("beta")).toBeInTheDocument();
  });

  it("requeues a wrong answer to the back", async () => {
    useRelearnStore.getState().seedQueue([card("alpha"), card("beta")]);
    submitRelearnAnswer.mockResolvedValue({ correct: false, meaning: "the first", reason: "nope" });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "bad guess" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByText("✗ Incorrect")).toBeInTheDocument();
    // The correct meaning and the learner's own answer are both shown.
    expect(screen.getByText("Meaning:")).toBeInTheDocument();
    expect(screen.getByText("the first")).toBeInTheDocument();
    expect(screen.getByText("Your answer:")).toBeInTheDocument();
    expect(screen.getByText("bad guess")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    expect(entries()).toEqual(["beta", "alpha"]);
    expect(useRelearnStore.getState().clearedCount).toBe(0);
  });

  it("skips a card as not-correct but still shows the answer", async () => {
    useRelearnStore.getState().seedQueue([card("alpha")]);
    submitRelearnAnswer.mockResolvedValue({ correct: false, meaning: "the first", reason: "skipped by user" });
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Don't Know" }));
    expect(await screen.findByText("✗ Incorrect")).toBeInTheDocument();
    expect(submitRelearnAnswer).toHaveBeenCalledWith(expect.objectContaining({ isSkipped: true }));
  });

  it("navigates to complete when the last word is cleared", async () => {
    useRelearnStore.getState().seedQueue([card("alpha")]);
    submitRelearnAnswer.mockResolvedValue({ correct: true, meaning: "the first", reason: "ok" });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "the first" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    await screen.findByText("✓ Correct");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/quiz/relearn/complete"));
  });

  it("surfaces a grading error and returns to answering", async () => {
    useRelearnStore.getState().seedQueue([card("alpha")]);
    submitRelearnAnswer.mockRejectedValue(new Error("boom"));
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Grading failed");
  });

  // A journal post's due grammar corrections must be presented like the LIVE
  // grammar quiz: the whole entry shown ONCE, every due blank drilled in place,
  // each graded progressively (per-blank feedback the moment it is committed),
  // then a single Next to advance — not one-blank-per-card with the whole post
  // repeated, and not a single submit-then-reveal.
  it("presents a post once and drills its due blanks progressively", async () => {
    const post = "Yesterday the John called me and then I go home.";
    useRelearnStore
      .getState()
      .seedQueue([grammarCard(post, "the John", 1), grammarCard(post, "go", 2)]);
    submitRelearnAnswer.mockImplementation(async (req: { noteId: bigint }) =>
      req.noteId === BigInt(1)
        ? { correct: true, correctAnswer: "John", category: "article", grammarNote: "No article before a personal name.", reason: "" }
        : { correct: true, correctAnswer: "went", category: "tense", grammarNote: "Past tense for a past event.", reason: "" },
    );
    renderPage();

    // The whole post is shown once, as a single screen, with an inline box per
    // due correction — not a plain word/meaning prompt.
    const postBox = screen.getByTestId("relearn-grammar-post");
    expect(postBox).toHaveTextContent("Yesterday");
    expect(postBox).toHaveTextContent("called me and then");
    const firstInput = screen.getByLabelText('Correction for "the John"');
    const secondInput = screen.getByLabelText('Correction for "go"');
    expect(screen.getByText(/Grammar/)).toBeInTheDocument();

    // Commit the first blank → it is graded on its own and turns into a pill in
    // place, while the second blank is still an open textbox (progressive).
    fireEvent.change(firstInput, { target: { value: "John" } });
    fireEvent.blur(firstInput);
    const firstPill = await screen.findByRole("button", { name: /the John — correct/ });
    expect(submitRelearnAnswer).toHaveBeenCalledWith(expect.objectContaining({ noteId: BigInt(1), answer: "John" }));
    expect(screen.getByLabelText('Correction for "go"')).toBeInTheDocument();

    // Commit the second blank → now all blanks are graded and Next appears.
    fireEvent.change(secondInput, { target: { value: "went" } });
    fireEvent.blur(secondInput);
    await screen.findByRole("button", { name: /go — correct/ });
    expect(submitRelearnAnswer).toHaveBeenCalledWith(expect.objectContaining({ noteId: BigInt(2), answer: "went" }));

    // Tapping a graded correction shows the live grammar quiz's labelled
    // feedback body (Mistake / Suggested / grammar note).
    fireEvent.click(firstPill);
    expect(screen.getByText("Mistake")).toBeInTheDocument();
    expect(screen.getByText("Suggested")).toBeInTheDocument();
    expect(screen.getByText("No article before a personal name.")).toBeInTheDocument();

    // One Next advances past the whole post; with nothing left, the session ends.
    fireEvent.click(screen.getByTestId("relearn-grammar-next"));
    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/quiz/relearn/complete"));
    expect(useRelearnStore.getState().clearedCount).toBe(2);
    expect(useRelearnStore.getState().totalAnswers).toBe(2);
  });

  // The model has NO skip / "Don't know" control (see quiz-ui-invariants).
  it("has no Don't know / skip control on a grammar blank", () => {
    const post = "Yesterday the John called me.";
    useRelearnStore.getState().seedQueue([grammarCard(post, "the John", 1)]);
    renderPage();
    expect(screen.queryByRole("button", { name: /don't know/i })).not.toBeInTheDocument();
    // Every ungraded blank offers an Exclude control instead.
    expect(screen.getByRole("button", { name: /Exclude "the John" from quizzes/i })).toBeInTheDocument();
  });

  // "See answers" grades every UNANSWERED blank as INCORRECT (a normal miss —
  // empty answer, is_skipped=false), never a skip. Answered blanks keep their
  // grade; excluded blanks are left alone.
  it("See answers marks unanswered blanks incorrect, not skipped", async () => {
    const post = "Yesterday the John called me and then I go home.";
    useRelearnStore
      .getState()
      .seedQueue([grammarCard(post, "the John", 1), grammarCard(post, "go", 2)]);
    submitRelearnAnswer.mockImplementation(async (req: { noteId: bigint; answer: string }) =>
      req.noteId === BigInt(1)
        ? { correct: true, correctAnswer: "John", category: "article", grammarNote: "No article.", reason: "" }
        : { correct: false, correctAnswer: "went", category: "tense", grammarNote: "Past tense.", reason: "no answer" },
    );
    renderPage();

    // Answer the first blank correctly, leave the second unanswered.
    fireEvent.change(screen.getByLabelText('Correction for "the John"'), { target: { value: "John" } });
    fireEvent.blur(screen.getByLabelText('Correction for "the John"'));
    await screen.findByRole("button", { name: /the John — correct/ });

    // Reveal the rest: the unanswered blank is graded with an EMPTY answer and
    // is_skipped=false (an incorrect miss), never is_skipped=true.
    fireEvent.click(screen.getByRole("button", { name: /See answers/ }));
    await screen.findByRole("button", { name: /go — incorrect/ });
    expect(submitRelearnAnswer).toHaveBeenCalledWith(
      expect.objectContaining({ noteId: BigInt(2), answer: "", isSkipped: false }),
    );
    // No submission in this flow ever sets is_skipped=true.
    for (const call of submitRelearnAnswer.mock.calls) {
      expect(call[0].isSkipped).toBe(false);
    }

    // The answered blank keeps its correct grade; the revealed one is incorrect.
    expect(screen.getByRole("button", { name: /the John — correct/ })).toBeInTheDocument();
    const sheet = await screen.findByTestId("relearn-grammar-feedback");
    expect(sheet).toHaveTextContent("✗ Incorrect");
    expect(sheet).not.toHaveTextContent(/Excluded/i);
  });

  // A per-blank Exclude is the ONLY thing that excludes: it calls the SkipWord
  // RPC (the same path every other card uses) for the blank's note_id + GRAMMAR
  // quiz type, and drops the blank from the post's active blanks. It never
  // grades the blank incorrect (no submitRelearnAnswer for it).
  it("per-blank Exclude calls skipWord and removes the blank", async () => {
    const post = "Yesterday the John called me and then I go home.";
    useRelearnStore
      .getState()
      .seedQueue([grammarCard(post, "the John", 1), grammarCard(post, "go", 2)]);
    renderPage();

    // Two due blanks → denominator is 2.
    expect(screen.getByText(/0 \/ 2 correct/)).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByRole("button", { name: /Exclude "the John" from quizzes/i }));

    await waitFor(() =>
      expect(skipWord).toHaveBeenCalledWith({ noteId: BigInt(1), quizTypes: [8] }),
    );
    // Excluding never grades the blank.
    expect(submitRelearnAnswer).not.toHaveBeenCalledWith(
      expect.objectContaining({ noteId: BigInt(1) }),
    );
    // The excluded blank leaves the active set → denominator drops to 1, and the
    // blank reads "excluded" (not "incorrect").
    await waitFor(() => expect(screen.getByText(/0 \/ 1 correct/)).toBeInTheDocument());
    expect(screen.getByTestId("relearn-grammar-post")).toHaveTextContent(/excluded/i);
  });

  // A wrong answer auto-reveals its feedback in the pinned sheet (associated
  // with the blank just graded); a correct answer reveals nothing.
  it("auto-reveals feedback for a wrong blank but not a correct one", async () => {
    const post = "Yesterday the John called me and then I go home.";
    useRelearnStore
      .getState()
      .seedQueue([grammarCard(post, "the John", 1), grammarCard(post, "go", 2)]);
    submitRelearnAnswer.mockImplementation(async (req: { noteId: bigint }) =>
      req.noteId === BigInt(1)
        ? { correct: true, correctAnswer: "John", category: "article", grammarNote: "No article.", reason: "" }
        : { correct: false, correctAnswer: "went", category: "tense", grammarNote: "Past tense.", reason: "not past tense" },
    );
    renderPage();

    // Correct blank → graded, but no feedback sheet auto-opens.
    fireEvent.change(screen.getByLabelText('Correction for "the John"'), { target: { value: "John" } });
    fireEvent.blur(screen.getByLabelText('Correction for "the John"'));
    await screen.findByRole("button", { name: /the John — correct/ });
    expect(screen.queryByTestId("relearn-grammar-feedback")).not.toBeInTheDocument();

    // Wrong blank → feedback sheet auto-opens for THAT blank.
    fireEvent.change(screen.getByLabelText('Correction for "go"'), { target: { value: "gone" } });
    fireEvent.blur(screen.getByLabelText('Correction for "go"'));
    const sheet = await screen.findByTestId("relearn-grammar-feedback");
    expect(sheet).toHaveTextContent("✗ Incorrect");
    expect(sheet).toHaveTextContent("went"); // the suggested fix for the go blank
  });

  // An etymology-origin card's feedback must show the origin details the quiz
  // shows — the roots with their meanings, the pronunciation, and the literal
  // gloss — not just the generic word/meaning/answer body.
  it("shows origin roots, meanings, pronunciation, and literal on etymology feedback", async () => {
    useRelearnStore.getState().seedQueue([etymologyCard("deface")]);
    submitRelearnAnswer.mockResolvedValue({
      correct: false,
      meaning: "to mar the surface of",
      reason: "not quite",
      literal: 'de "down" + facere = "made down"',
      wordDetail: {
        pronunciation: "dɪˈfeɪs",
        originParts: [
          { origin: "de", meaning: "down", language: "Latin", type: "prefix" },
          { origin: "facere", meaning: "to make", language: "Latin", type: "root" },
        ],
      },
    });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "wrong guess" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByText("✗ Incorrect")).toBeInTheDocument();

    // Roots with their meanings.
    expect(screen.getByText("de")).toBeInTheDocument();
    expect(screen.getByText("(down)")).toBeInTheDocument();
    expect(screen.getByText("facere")).toBeInTheDocument();
    expect(screen.getByText("(to make)")).toBeInTheDocument();
    // Pronunciation and literal gloss.
    expect(screen.getByText("/dɪˈfeɪs/")).toBeInTheDocument();
    expect(screen.getByText('de "down" + facere = "made down"')).toBeInTheDocument();
  });

  // A plain vocabulary relearn card must NOT render the etymology origin block
  // (no literal, no Origin header).
  it("does not show the etymology origin block for a vocab card", async () => {
    useRelearnStore.getState().seedQueue([card("alpha")]);
    submitRelearnAnswer.mockResolvedValue({ correct: false, meaning: "the first", reason: "nope" });
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Type the meaning"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByText("✗ Incorrect")).toBeInTheDocument();
    expect(screen.queryByText("Origin")).not.toBeInTheDocument();
  });

  it("still renders non-grammar relearn cards one per screen", () => {
    useRelearnStore
      .getState()
      .seedQueue([card("alpha"), grammarCard("A the John post.", "the John", 3)]);
    renderPage();
    // The vocab card is its own screen and comes first; the post is a separate
    // screen behind it (not merged into the vocab card).
    expect(screen.getByTestId("relearn-prompt")).toHaveTextContent("alpha");
    expect(screen.queryByTestId("relearn-grammar-post")).not.toBeInTheDocument();
    expect(screen.getByText("2 words left")).toBeInTheDocument();
  });
});
