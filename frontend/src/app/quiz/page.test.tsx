import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import QuizHubPage from "./page";

vi.mock("next/link", () => ({
  default: ({ children, ...props }: { children: React.ReactNode; href: string }) => (
    <a {...props}>{children}</a>
  ),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <QuizHubPage />
    </ChakraProvider>
  );
}

describe("QuizHubPage", () => {
  it("renders Quiz title and back link to Home", () => {
    renderPage();
    expect(screen.getByText("Quiz")).toBeInTheDocument();
    const backLink = screen.getByText("< Home").closest("a");
    expect(backLink).toHaveAttribute("href", "/");
  });

  it("shows Vocabulary tab with 3 quiz mode cards by default", () => {
    renderPage();
    expect(screen.getByText("Standard")).toBeInTheDocument();
    expect(screen.getByText("See a word, type its meaning")).toBeInTheDocument();
    expect(screen.getByText("Reverse")).toBeInTheDocument();
    expect(screen.getByText("See a meaning, type the word")).toBeInTheDocument();
    expect(screen.getByText("Freeform")).toBeInTheDocument();
    expect(screen.getByText("Type any word and its meaning")).toBeInTheDocument();
  });

  it("links vocabulary modes to quiz start page", () => {
    renderPage();
    const standardLink = screen.getByText("Standard").closest("a");
    expect(standardLink).toHaveAttribute("href", "/quiz/start?mode=standard");
    const reverseLink = screen.getByText("Reverse").closest("a");
    expect(reverseLink).toHaveAttribute("href", "/quiz/start?mode=reverse");
    const freeformLink = screen.getByText("Freeform").closest("a");
    expect(freeformLink).toHaveAttribute("href", "/quiz/start?mode=freeform");
  });

  it("switches to Etymology tab and shows etymology quiz modes", () => {
    renderPage();
    fireEvent.click(screen.getByText("Etymology"));

    expect(screen.getByText("Breakdown")).toBeInTheDocument();
    expect(screen.getByText("See a word, identify its origins and meanings")).toBeInTheDocument();
    expect(screen.getByText("Assembly")).toBeInTheDocument();
    expect(screen.getByText("See origins and meanings, type the word")).toBeInTheDocument();
    expect(screen.getByText("Freeform")).toBeInTheDocument();
    expect(screen.getByText("Type any word and break down its origins")).toBeInTheDocument();
  });

  it("links etymology modes to etymology-start page", () => {
    renderPage();
    fireEvent.click(screen.getByText("Etymology"));

    const breakdownLink = screen.getByText("Breakdown").closest("a");
    expect(breakdownLink).toHaveAttribute("href", "/quiz/etymology-start?mode=breakdown");
    const assemblyLink = screen.getByText("Assembly").closest("a");
    expect(assemblyLink).toHaveAttribute("href", "/quiz/etymology-start?mode=assembly");
    const freeformLink = screen.getByText("Freeform").closest("a");
    expect(freeformLink).toHaveAttribute("href", "/quiz/etymology-start?mode=freeform");
  });
});
