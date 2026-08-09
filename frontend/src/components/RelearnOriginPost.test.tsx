import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { RelearnOriginPost } from "./RelearnOriginPost";
import type { RelearnCard } from "@/lib/client";

vi.mock("@/lib/client", () => ({
  quizClient: { submitRelearnAnswer: vi.fn() },
  QuizType: { QUIZ_TYPE_UNSPECIFIED: 0, STANDARD: 1, REVERSE: 2, FREEFORM: 3, ETYMOLOGY_ORIGIN: 4, RELEARN: 7, GRAMMAR: 8 },
}));

const word = (entry: string, noteId: number): RelearnCard =>
  ({
    entry,
    noteId: BigInt(noteId),
    sourceQuizType: 4,
    meaning: `${entry}-meaning`,
    examples: [],
    contexts: [],
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
});
