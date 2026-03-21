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

  it("shows Books feature link", () => {
    renderPage();
    expect(screen.getByText("Books")).toBeInTheDocument();
    expect(screen.getByText("Read books and look up words")).toBeInTheDocument();
    const link = screen.getByText("Books").closest("a");
    expect(link).toHaveAttribute("href", "/books");
  });

  it("shows Learn feature link", () => {
    renderPage();
    expect(screen.getByText("Learn")).toBeInTheDocument();
    expect(screen.getByText("Browse vocabulary notebooks and etymology origins")).toBeInTheDocument();
    const link = screen.getByText("Learn").closest("a");
    expect(link).toHaveAttribute("href", "/learn");
  });

  it("shows Quiz feature link", () => {
    renderPage();
    expect(screen.getByText("Quiz")).toBeInTheDocument();
    expect(screen.getByText("Practice vocabulary and etymology with spaced repetition")).toBeInTheDocument();
    const link = screen.getByText("Quiz").closest("a");
    expect(link).toHaveAttribute("href", "/quiz");
  });

  it("renders exactly 3 feature cards", () => {
    renderPage();
    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(3);
  });
});
