import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import RelearnStart from "./RelearnStart";
import { useRelearnStore } from "@/store/relearnStore";
import type { RelearnCard } from "@/lib/client";

const startRelearnQuiz = vi.fn();
vi.mock("@/lib/client", () => ({
  quizClient: {
    startRelearnQuiz: (...args: unknown[]) => startRelearnQuiz(...args),
  },
  QuizType: { QUIZ_TYPE_UNSPECIFIED: 0, STANDARD: 1, REVERSE: 2 },
}));

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: pushMock }) }));

function renderStart() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <RelearnStart />
    </ChakraProvider>,
  );
}

const card = (entry: string): RelearnCard => ({ entry, noteId: BigInt(1) }) as RelearnCard;

describe("RelearnStart", () => {
  beforeEach(() => {
    startRelearnQuiz.mockReset();
    pushMock.mockReset();
    useRelearnStore.getState().reset();
  });

  it("loads the pool and shows the word count", async () => {
    startRelearnQuiz.mockResolvedValue({ cards: [card("a"), card("b")] });
    renderStart();
    expect(await screen.findByText("2 words to relearn")).toBeInTheDocument();
    expect(startRelearnQuiz).toHaveBeenCalledWith({ windowHours: 24 });
  });

  it("shows the empty state when nothing is pooled", async () => {
    startRelearnQuiz.mockResolvedValue({ cards: [] });
    renderStart();
    expect(await screen.findByText("Nothing to relearn.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Start" })).toBeDisabled();
  });

  it("reloads with a different window when a preset is chosen", async () => {
    startRelearnQuiz.mockResolvedValue({ cards: [card("a")] });
    renderStart();
    await screen.findByText("1 word to relearn");
    fireEvent.click(screen.getByText(/6 hours/));
    await waitFor(() => expect(startRelearnQuiz).toHaveBeenLastCalledWith({ windowHours: 6 }));
  });

  it("seeds the queue and navigates on Start", async () => {
    startRelearnQuiz.mockResolvedValue({ cards: [card("a"), card("b")] });
    renderStart();
    await screen.findByText("2 words to relearn");
    fireEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(useRelearnStore.getState().queue.map((c) => c.entry)).toEqual(["a", "b"]);
    expect(pushMock).toHaveBeenCalledWith("/quiz/relearn/session");
  });

  it("shows an error when loading fails", async () => {
    startRelearnQuiz.mockRejectedValue(new Error("boom"));
    renderStart();
    expect(await screen.findByRole("alert")).toHaveTextContent("Failed to load");
  });
});
