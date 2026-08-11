import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { RelearnOriginPost } from "./RelearnOriginPost";
import type { RelearnCard, SubmitRelearnAnswerResponse } from "@/lib/client";
import { quizClient } from "@/lib/client";

vi.mock("@/lib/client", () => ({
  quizClient: { submitRelearnAnswer: vi.fn() },
  QuizType: { QUIZ_TYPE_UNSPECIFIED: 0, STANDARD: 1, REVERSE: 2, FREEFORM: 3, ETYMOLOGY_ORIGIN: 4, RELEARN: 7, GRAMMAR: 8 },
}));

const submitMock = vi.mocked(quizClient.submitRelearnAnswer);

// originDirection: 1 (STANDARD/recognition — the default) or 2 (REVERSE).
const word = (
  entry: string,
  noteId: number,
  originDirection = 1,
  contexts: RelearnCard["contexts"] = [],
  examples: RelearnCard["examples"] = [],
): RelearnCard =>
  ({
    entry,
    noteId: BigInt(noteId),
    sourceQuizType: 4,
    originDirection,
    meaning: `${entry}-meaning`,
    examples,
    contexts,
    type: "root",
    language: "Latin",
    originText: "liber",
    originMeaning: "free",
    englishForms: ["lib", "liv"],
  }) as RelearnCard;

function renderPost(englishForms: string[]) {
  return render(
    <ChakraProvider value={defaultSystem}>
      <RelearnOriginPost
        originText="liber"
        originMeaning="free"
        type="root"
        language="Latin"
        englishForms={englishForms}
        words={[word("liberty", 1), word("liberal", 2)]}
        onComplete={vi.fn()}
      />
    </ChakraProvider>,
  );
}

// A graded response for one word. contextScenes drives the "Where it appears"
// section; correct=false makes the feedback sheet open automatically.
const gradeResponse = (
  overrides: Partial<SubmitRelearnAnswerResponse> = {},
): SubmitRelearnAnswerResponse =>
  ({
    correct: false,
    meaning: "free",
    reason: "",
    literal: "",
    contextScenes: [],
    ...overrides,
  }) as SubmitRelearnAnswerResponse;

describe("RelearnOriginPost", () => {
  it("renders the origin's english_forms as chips on the header", () => {
    renderPost(["lib", "liv"]);
    const chips = screen.getByTestId("relearn-origin-english-forms");
    expect(chips).toBeInTheDocument();
    expect(chips).toHaveTextContent("lib");
    expect(chips).toHaveTextContent("liv");
  });

  it("omits the chip row when there are no english_forms", () => {
    renderPost([]);
    expect(screen.queryByTestId("relearn-origin-english-forms")).not.toBeInTheDocument();
  });

  it("shows the word's example scenes in the feedback sheet when it is graded with contextScenes", async () => {
    submitMock.mockResolvedValueOnce(
      gradeResponse({
        contextScenes: [
          {
            notebookName: "nb",
            sceneTitle: "At the library",
            statements: ["They fought for liberty and freedom."],
            conversations: [],
          },
        ] as SubmitRelearnAnswerResponse["contextScenes"],
      }),
    );
    renderPost(["lib", "liv"]);

    const input = screen.getByLabelText('Meaning for "liberty"');
    fireEvent.change(input, { target: { value: "wrong guess" } });
    fireEvent.keyDown(input, { key: "Enter" });

    const sheet = await screen.findByTestId("relearn-origin-feedback");
    await waitFor(() => {
      expect(sheet).toHaveTextContent("Where it appears");
      expect(sheet).toHaveTextContent("They fought for liberty and freedom.");
    });
  });

  it("renders a reverse-direction word as meaning + a word input, and a recognition word as word + a meaning input, under one origin header", () => {
    render(
      <ChakraProvider value={defaultSystem}>
        <RelearnOriginPost
          originText="liber"
          originMeaning="free"
          type="root"
          language="Latin"
          englishForms={["lib", "liv"]}
          words={[
            // liberty missed in REVERSE (2): show the meaning, ask the word.
            word("liberty", 1, 2, [
              { context: "They fought for liberty.", maskedContext: "They fought for ____." },
            ] as RelearnCard["contexts"]),
            // liberal missed in recognition (1): show the word, ask the meaning.
            word("liberal", 2, 1),
          ]}
          onComplete={vi.fn()}
        />
      </ChakraProvider>,
    );

    // One shared origin header.
    expect(screen.getByTestId("relearn-origin-post")).toHaveTextContent("liber");

    // Reverse word: prompt is the meaning + masked context, input asks for the word.
    expect(screen.getByText("liberty-meaning")).toBeInTheDocument();
    expect(screen.getByText("They fought for ____.")).toBeInTheDocument();
    expect(screen.getByLabelText('Word for "liberty-meaning"')).toBeInTheDocument();

    // Recognition word: input asks for the meaning.
    expect(screen.getByLabelText('Meaning for "liberal"')).toBeInTheDocument();
  });

  it("grades a reverse-direction word by the WORD the learner types", async () => {
    submitMock.mockResolvedValueOnce(gradeResponse({ correct: true }));
    render(
      <ChakraProvider value={defaultSystem}>
        <RelearnOriginPost
          originText="liber"
          originMeaning="free"
          type="root"
          language="Latin"
          englishForms={[]}
          words={[word("liberty", 1, 2)]}
          onComplete={vi.fn()}
        />
      </ChakraProvider>,
    );

    const input = screen.getByLabelText('Word for "liberty-meaning"');
    fireEvent.change(input, { target: { value: "liberty" } });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => {
      expect(submitMock).toHaveBeenCalledWith(
        expect.objectContaining({ noteId: BigInt(1), answer: "liberty", isSkipped: false }),
      );
    });
  });

  it("shows a recognition word's example as usage context WHILE ASKING (before any grading)", () => {
    render(
      <ChakraProvider value={defaultSystem}>
        <RelearnOriginPost
          originText="liber"
          originMeaning="free"
          type="root"
          language="Latin"
          englishForms={[]}
          words={[
            // recognition (1): word shown, recall meaning → full example is a hint.
            word("liberty", 1, 1, [], [
              { text: "They fought for liberty and freedom.", speaker: "", highlight: "liberty" },
            ] as RelearnCard["examples"]),
          ]}
          onComplete={vi.fn()}
        />
      </ChakraProvider>,
    );

    // The word is still being asked (its meaning input is present, no grade yet)…
    expect(screen.getByLabelText('Meaning for "liberty"')).toBeInTheDocument();
    // …and the FULL example is already on screen as usage context.
    expect(screen.getByText("They fought for liberty and freedom.")).toBeInTheDocument();
  });

  it("shows a reverse word's example MASKED while asking, without revealing the answer", () => {
    render(
      <ChakraProvider value={defaultSystem}>
        <RelearnOriginPost
          originText="liber"
          originMeaning="free"
          type="root"
          language="Latin"
          englishForms={[]}
          words={[
            // reverse (2): meaning shown, recall the word → example is masked.
            word("liberty", 1, 2, [
              { context: "They fought for liberty.", maskedContext: "They fought for ____." },
            ] as RelearnCard["contexts"]),
          ]}
          onComplete={vi.fn()}
        />
      </ChakraProvider>,
    );

    expect(screen.getByLabelText('Word for "liberty-meaning"')).toBeInTheDocument();
    // The masked hint is shown; the answer word is NOT revealed.
    expect(screen.getByText("They fought for ____.")).toBeInTheDocument();
    expect(screen.queryByText("They fought for liberty.")).not.toBeInTheDocument();
  });

  it("disables Next while a committed word is still grading, then enables it once the grade lands", async () => {
    // A single-word family: committing the one word makes remainingCount 0, so
    // the Next control renders — but the grade is still in flight. Next must be
    // disabled until it lands, or tapping it discards the ungraded answer.
    let resolveGrade!: (r: SubmitRelearnAnswerResponse) => void;
    submitMock.mockReturnValueOnce(
      new Promise<SubmitRelearnAnswerResponse>((resolve) => {
        resolveGrade = resolve;
      }),
    );
    render(
      <ChakraProvider value={defaultSystem}>
        <RelearnOriginPost
          originText="liber"
          originMeaning="free"
          type="root"
          language="Latin"
          englishForms={[]}
          words={[word("liberty", 1)]}
          onComplete={vi.fn()}
        />
      </ChakraProvider>,
    );

    const input = screen.getByLabelText('Meaning for "liberty"');
    fireEvent.change(input, { target: { value: "wrong guess" } });
    fireEvent.keyDown(input, { key: "Enter" });

    // Grading in flight → Next is present but disabled.
    const next = await screen.findByTestId("relearn-origin-next");
    expect(next).toBeDisabled();

    resolveGrade(gradeResponse({ correct: true }));

    await waitFor(() => expect(screen.getByTestId("relearn-origin-next")).toBeEnabled());
  });

  it("flips a ✓ word to ✗ (in-session, no network) so it re-drills this session", async () => {
    submitMock.mockResolvedValueOnce(gradeResponse({ correct: true }));
    const onComplete = vi.fn();
    render(
      <ChakraProvider value={defaultSystem}>
        <RelearnOriginPost
          originText="liber"
          originMeaning="free"
          type="root"
          language="Latin"
          englishForms={[]}
          words={[word("liberty", 1)]}
          onComplete={onComplete}
        />
      </ChakraProvider>,
    );

    const input = screen.getByLabelText('Meaning for "liberty"');
    fireEvent.change(input, { target: { value: "freedom" } });
    fireEvent.keyDown(input, { key: "Enter" });

    // Graded correct → chip is correct. A correct word does NOT auto-open its
    // sheet, so the learner taps the chip to review/flip it.
    const chip = await screen.findByRole("button", { name: "liberty — correct" });
    const callsAfterGrade = submitMock.mock.calls.length;
    fireEvent.click(chip);

    const flip = await screen.findByTestId("relearn-origin-override");
    expect(flip).toHaveTextContent("Mark as Incorrect");
    fireEvent.click(flip);

    // Chip now reflects the override, and NO extra grading call fired.
    await screen.findByRole("button", { name: "liberty — incorrect" });
    expect(submitMock.mock.calls.length).toBe(callsAfterGrade);

    // On Next the flipped word is reported wrong → completeOrigin re-queues it.
    fireEvent.click(screen.getByTestId("relearn-origin-next"));
    expect(onComplete).toHaveBeenCalledTimes(1);
    const [wrongWords, correctCount] = onComplete.mock.calls[0] as [RelearnCard[], number];
    expect(wrongWords.map((w) => w.entry)).toContain("liberty");
    expect(correctCount).toBe(0);
  });

  it("flips a ✗ word to ✓ so it drops from this session", async () => {
    submitMock.mockResolvedValueOnce(gradeResponse({ correct: false }));
    const onComplete = vi.fn();
    render(
      <ChakraProvider value={defaultSystem}>
        <RelearnOriginPost
          originText="liber"
          originMeaning="free"
          type="root"
          language="Latin"
          englishForms={[]}
          words={[word("liberty", 1)]}
          onComplete={onComplete}
        />
      </ChakraProvider>,
    );

    const input = screen.getByLabelText('Meaning for "liberty"');
    fireEvent.change(input, { target: { value: "wrong guess" } });
    fireEvent.keyDown(input, { key: "Enter" });

    // Wrong answers auto-open the sheet.
    await screen.findByTestId("relearn-origin-feedback");
    const flip = await screen.findByTestId("relearn-origin-override");
    expect(flip).toHaveTextContent("Mark as Correct");
    fireEvent.click(flip);

    await screen.findByRole("button", { name: "liberty — correct" });

    fireEvent.click(screen.getByTestId("relearn-origin-next"));
    const [wrongWords, correctCount] = onComplete.mock.calls[0] as [RelearnCard[], number];
    expect(wrongWords).toHaveLength(0);
    expect(correctCount).toBe(1);
  });

  it("Undo restores the grader's verdict", async () => {
    submitMock.mockResolvedValueOnce(gradeResponse({ correct: true }));
    render(
      <ChakraProvider value={defaultSystem}>
        <RelearnOriginPost
          originText="liber"
          originMeaning="free"
          type="root"
          language="Latin"
          englishForms={[]}
          words={[word("liberty", 1)]}
          onComplete={vi.fn()}
        />
      </ChakraProvider>,
    );

    const input = screen.getByLabelText('Meaning for "liberty"');
    fireEvent.change(input, { target: { value: "freedom" } });
    fireEvent.keyDown(input, { key: "Enter" });

    fireEvent.click(await screen.findByRole("button", { name: "liberty — correct" }));
    fireEvent.click(await screen.findByTestId("relearn-origin-override")); // → incorrect
    await screen.findByRole("button", { name: "liberty — incorrect" });

    fireEvent.click(await screen.findByTestId("relearn-origin-override-undo"));
    // Back to the grader's verdict, and the flip control returns.
    await screen.findByRole("button", { name: "liberty — correct" });
    expect(screen.getByTestId("relearn-origin-override")).toHaveTextContent("Mark as Incorrect");
  });

  it("renders no example scenes for a word graded without contextScenes", async () => {
    // contextScenes absent → the `?? []` fallback → RelearnContext renders null.
    submitMock.mockResolvedValueOnce({
      correct: false,
      meaning: "free",
      reason: "",
      literal: "",
    } as SubmitRelearnAnswerResponse);
    renderPost(["lib", "liv"]);

    const input = screen.getByLabelText('Meaning for "liberty"');
    fireEvent.change(input, { target: { value: "wrong guess" } });
    fireEvent.keyDown(input, { key: "Enter" });

    const sheet = await screen.findByTestId("relearn-origin-feedback");
    await waitFor(() => expect(sheet).toHaveTextContent("wrong guess"));
    expect(sheet).not.toHaveTextContent("Where it appears");
  });
});
