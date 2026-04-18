import { describe, it, expect, afterEach } from "vitest";
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

function renderPageDark() {
  document.documentElement.classList.add("dark");
  document.documentElement.setAttribute("data-theme", "dark");
  return renderPage();
}

describe("HomePage", () => {
  afterEach(() => {
    document.documentElement.classList.remove("dark");
    document.documentElement.removeAttribute("data-theme");
  });

  it("renders app title", () => {
    renderPage();
    expect(screen.getByText("Langner")).toBeInTheDocument();
  });

  it("shows Learn feature link", () => {
    renderPage();
    expect(screen.getByText("Learn")).toBeInTheDocument();
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

  it("renders exactly 2 feature cards", () => {
    renderPage();
    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(2);
  });

  it("renders in dark mode without errors", () => {
    renderPageDark();
    expect(screen.getByText("Langner")).toBeInTheDocument();
    expect(screen.getByText("Learn")).toBeInTheDocument();
    expect(screen.getByText("Quiz")).toBeInTheDocument();
    expect(screen.getAllByRole("link")).toHaveLength(2);
  });
});
