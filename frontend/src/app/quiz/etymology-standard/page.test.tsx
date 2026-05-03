import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import EtymologyStandardPage from "./page";
import { useQuizStore } from "@/store/quizStore";
import { quizClient } from "@/lib/client";

// Next.js router stub. Page calls router.push on completion / when redirected.
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

// quizClient is the only network boundary. We capture the actual payload
// the page sends so we can assert the buffer contents at flush time.
vi.mock(import("@/lib/client"), async (importOriginal) => {
  const actual = await importOriginal();
  return {
    ...actual,
    quizClient: {
      ...(actual.quizClient as object),
      batchSubmitEtymologyStandardAnswers: vi.fn(async ({ answers }: { answers: { cardId: bigint }[] }) => ({
        responses: answers.map((_a, i) => ({
          correct: true,
          reason: "",
          correctMeaning: "",
          nextReviewDate: "",
          learnedAt: "2026-04-25T10:00:00Z",
          noteId: BigInt(100 + i),
        })),
      })),
      // Stub other RPCs in case useQuizResultActions touches them on a
      // re-render. The page's keyboard flow doesn't actually call them.
      overrideAnswer: vi.fn(),
      undoOverrideAnswer: vi.fn(),
      skipWord: vi.fn(),
      resumeWord: vi.fn(),
    },
  };
});

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <EtymologyStandardPage />
    </ChakraProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useQuizStore.getState().reset();
  useQuizStore.getState().setQuizType("etymology-standard");
  // Generic etymology cards. After the backend dedup fix the cards array has
  // unique origins; this test verifies the *frontend* doesn't re-introduce
  // duplicates from the same single user action.
  useQuizStore.getState().setEtymologyOriginCards([
    { cardId: BigInt(1), origin: "tele", type: "root", language: "Greek", meaning: "far", notebookName: "greek-roots", sessionTitle: "Session 1", exampleWords: [] },
    { cardId: BigInt(2), origin: "graph", type: "root", language: "Greek", meaning: "to write", notebookName: "greek-roots", sessionTitle: "Session 1", exampleWords: [] },
    { cardId: BigInt(3), origin: "phone", type: "root", language: "Greek", meaning: "sound", notebookName: "greek-roots", sessionTitle: "Session 1", exampleWords: [] },
  ]);
  useQuizStore.getState().setFeedbackInterval(3);
});

// REPRODUCES THE USER-REPORTED BUG: "I only answered 2 origins, but the
// feedback screen showed 4 entries of one origin and 3 of another".
//
// The page nests two onKeyDown handlers — one on the outer Box and one on
// the inner AnswerInput's input. A keydown on the input bubbles up to the
// Box, so a single Enter press dispatches handleSubmit twice — one buffer
// entry per handler invocation. With Zustand's currentIndex reading lagging
// the closure, the second call captures the SAME card as the first, so the
// batch ends up with the same origin recorded multiple times.
//
// Expected behavior: pressing Enter once on a card produces exactly one
// answer in the batch payload.
it("a single Enter press records exactly one answer per card", async () => {
  renderPage();

  const input = await screen.findByPlaceholderText("type the meaning...");

  // Card 1: tele
  fireEvent.change(input, { target: { value: "far" } });
  fireEvent.keyDown(input, { key: "Enter" });

  // Card 2: graph
  await waitFor(() =>
    expect(useQuizStore.getState().currentIndex).toBeGreaterThan(0),
  );
  fireEvent.change(input, { target: { value: "to write" } });
  fireEvent.keyDown(input, { key: "Enter" });

  // Card 3: phone — also the batch boundary (interval=3), so flushBatch fires.
  await waitFor(() =>
    expect(useQuizStore.getState().currentIndex).toBeGreaterThanOrEqual(2),
  );
  fireEvent.change(input, { target: { value: "sound" } });
  fireEvent.keyDown(input, { key: "Enter" });

  await waitFor(() => {
    expect(quizClient.batchSubmitEtymologyStandardAnswers).toHaveBeenCalled();
  });

  const callArgs = vi.mocked(quizClient.batchSubmitEtymologyStandardAnswers).mock.calls[0]![0]!;
  const cardIds = callArgs.answers.map((a: { cardId: bigint }) => a.cardId.toString());

  expect(cardIds).toEqual(["1", "2", "3"]);
});
