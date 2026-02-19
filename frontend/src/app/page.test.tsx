import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import QuizStartPage from "./page";

describe("QuizStartPage", () => {
  it("renders the quiz start heading", () => {
    render(<QuizStartPage />);
    expect(screen.getByText("Quiz Start")).toBeInTheDocument();
  });
});
