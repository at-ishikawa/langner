import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import SettingsPage from "./page";

vi.mock("@/components/SessionProvider", () => ({
  useSession: () => ({
    user: { username: "user_test", hasApiKey: true, credentials: [], availableProviders: ["openai"] },
    loading: false,
    refresh: vi.fn(),
  }),
}));

vi.mock("@/lib/client", () => ({
  quizClient: {
    getIntervalSettings: vi.fn(() =>
      Promise.resolve({
        settings: { algorithm: "modified_sm2", fixedIntervals: [] },
        isDefault: true,
        defaultSettings: { algorithm: "modified_sm2", fixedIntervals: [] },
      }),
    ),
    updateIntervalSettings: vi.fn(),
  },
}));

describe("SettingsPage", () => {
  it("shows the review schedule section", async () => {
    render(
      <ChakraProvider value={defaultSystem}>
        <SettingsPage />
      </ChakraProvider>,
    );
    expect(screen.getByText("Review schedule")).toBeInTheDocument();
    expect(await screen.findByText(/You are using the default schedule \(adaptive\)/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Adaptive" })).toHaveAttribute("aria-pressed", "true");
  });
});
