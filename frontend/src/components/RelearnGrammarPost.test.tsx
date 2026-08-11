import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { RelearnGrammarPost } from "./RelearnGrammarPost";
import type { RelearnCard, SubmitRelearnAnswerResponse } from "@/lib/client";
import { quizClient } from "@/lib/client";

vi.mock("@/lib/client", () => ({
  quizClient: { submitRelearnAnswer: vi.fn() },
  QuizType: { QUIZ_TYPE_UNSPECIFIED: 0, STANDARD: 1, REVERSE: 2, FREEFORM: 3, ETYMOLOGY_ORIGIN: 4, RELEARN: 7, GRAMMAR: 8 },
}));

const submitMock = vi.mocked(quizClient.submitRelearnAnswer);

const CONTENT = "Yesterday the John invited me while I was in school.";

const blank = (incorrect: string, noteId: number): RelearnCard =>
  ({
    noteId: BigInt(noteId),
    incorrect,
    content: CONTENT,
    sourceQuizType: 8,
    entry: incorrect,
    meaning: "",
    examples: [],
    contexts: [],
  }) as RelearnCard;

const gradeResponse = (
  overrides: Partial<SubmitRelearnAnswerResponse> = {},
): SubmitRelearnAnswerResponse =>
  ({ correct: true, meaning: "", reason: "", ...overrides }) as SubmitRelearnAnswerResponse;

describe("RelearnGrammarPost", () => {
  it("disables 'See answers' while a blank is grading, and never empty-submits the in-flight blank", async () => {
    let resolveGrade!: (r: SubmitRelearnAnswerResponse) => void;
    submitMock.mockReturnValueOnce(
      new Promise<SubmitRelearnAnswerResponse>((resolve) => {
        resolveGrade = resolve;
      }),
    );
    render(
      <ChakraProvider value={defaultSystem}>
        <RelearnGrammarPost
          content={CONTENT}
          blanks={[blank("the John", 1), blank("in school", 2)]}
          onComplete={vi.fn()}
        />
      </ChakraProvider>,
    );

    // Commit the first blank → it starts grading while the second is still
    // un-answered, so "See answers (1 left)" is on screen.
    const input = screen.getByLabelText('Correction for "the John"');
    fireEvent.change(input, { target: { value: "John" } });
    fireEvent.keyDown(input, { key: "Enter" });

    const grading = await screen.findByRole("button", { name: /Grading/i });
    expect(grading).toBeDisabled();
    expect(submitMock).toHaveBeenCalledTimes(1);
    expect(submitMock).toHaveBeenCalledWith(
      expect.objectContaining({ noteId: BigInt(1), answer: "John" }),
    );

    resolveGrade(gradeResponse({ correct: true }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /See answers/i })).toBeEnabled(),
    );
    expect(submitMock).toHaveBeenCalledTimes(1);
  });
});
