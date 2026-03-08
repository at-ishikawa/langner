import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import HomePage from "./page";

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <HomePage />
    </ChakraProvider>
  );
}

describe("HomePage", () => {
  it("renders app title", () => {
    renderPage();
    expect(screen.getByText("Langner")).toBeInTheDocument();
  });

  it("shows Quiz feature link", () => {
    renderPage();
    expect(screen.getByText("Quiz")).toBeInTheDocument();
    expect(screen.getByText("Practice vocabulary with spaced repetition")).toBeInTheDocument();
    const link = screen.getByText("Quiz").closest("a");
    expect(link).toHaveAttribute("href", "/quiz");
  });

  it("shows Notebooks feature link", () => {
    renderPage();
    expect(screen.getByText("Notebooks")).toBeInTheDocument();
    expect(screen.getByText("Browse stories, scenes, and vocabulary")).toBeInTheDocument();
    const link = screen.getByText("Notebooks").closest("a");
    expect(link).toHaveAttribute("href", "/notebooks");
  });
});
