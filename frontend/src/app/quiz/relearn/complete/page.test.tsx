import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import RelearnCompletePage from "./page";
import { useRelearnStore } from "@/store/relearnStore";
import type { RelearnCard } from "@/lib/client";

vi.mock("@/lib/client", () => ({ quizClient: {} }));
const pushMock = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: pushMock }) }));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <RelearnCompletePage />
    </ChakraProvider>,
  );
}

const card = (e: string): RelearnCard => ({ entry: e }) as RelearnCard;

describe("RelearnCompletePage", () => {
  beforeEach(() => {
    pushMock.mockReset();
    useRelearnStore.getState().reset();
  });

  it("summarises cleared words and total answers", () => {
    // Simulate a session: 1 word, wrong then correct → cleared 1, total 2.
    useRelearnStore.getState().seedQueue([card("a")]);
    useRelearnStore.getState().resolveFront(false);
    useRelearnStore.getState().resolveFront(true);
    renderPage();
    expect(screen.getByText("You cleared 1 word.")).toBeInTheDocument();
    expect(screen.getByText(/Total answers: 2/)).toBeInTheDocument();
    expect(screen.getByText(/came around more than once/)).toBeInTheDocument();
    expect(screen.getByText(/Nothing was saved/)).toBeInTheDocument();
  });

  it("Relearn again resets and navigates to the start", () => {
    useRelearnStore.getState().seedQueue([card("a")]);
    useRelearnStore.getState().resolveFront(true);
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Relearn again" }));
    expect(pushMock).toHaveBeenCalledWith("/quiz?tab=relearn");
    expect(useRelearnStore.getState().clearedCount).toBe(0);
  });

  it("Quiz Hub resets and navigates to the hub", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Quiz Hub" }));
    expect(pushMock).toHaveBeenCalledWith("/quiz");
  });
});
