import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { LearningStatusBadge } from "./LearningStatusBadge";

function renderBadge(status: string) {
  return render(
    <ChakraProvider value={defaultSystem}>
      <LearningStatusBadge status={status} />
    </ChakraProvider>,
  );
}

describe("LearningStatusBadge", () => {
  it.each([
    { status: "", label: "Learning" },
    { status: "misunderstood", label: "Misunderstood" },
    { status: "understood", label: "Understood" },
    { status: "usable", label: "Usable" },
    { status: "intuitive", label: "Intuitive" },
  ])("renders $label for status '$status'", ({ status, label }) => {
    renderBadge(status);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("falls back to Learning for unknown status", () => {
    renderBadge("unknown_status");
    expect(screen.getByText("Learning")).toBeInTheDocument();
  });
});
