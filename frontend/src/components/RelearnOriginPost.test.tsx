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
): RelearnCard =>
  ({
    entry,
    noteId: BigInt(noteId),
    sourceQuizType: 4,
    originDirection,
    meaning: `${entry}-meaning`,
    examples: [],
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
