import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import SignupPage from "./page";
import * as auth from "@/lib/auth";

let searchParams = new URLSearchParams();

vi.mock("next/navigation", () => ({
  useSearchParams: () => searchParams,
}));

vi.mock("next/link", () => ({
  default: ({ children, ...props }: { children: React.ReactNode; href: string }) => (
    <a {...props}>{children}</a>
  ),
}));

vi.mock("@/lib/auth", () => ({
  signup: vi.fn(),
}));

function renderPage() {
  return render(
    <ChakraProvider value={defaultSystem}>
      <SignupPage />
    </ChakraProvider>,
  );
}

describe("SignupPage", () => {
  afterEach(() => {
    searchParams = new URLSearchParams();
    vi.clearAllMocks();
  });

  it("starts the Google OAuth flow when the sign-up button is clicked", () => {
    renderPage();
    const button = screen.getByRole("button", { name: /Sign up with Google/ });
    fireEvent.click(button);
    expect(auth.signup).toHaveBeenCalledTimes(1);
  });

  it("offers an existing-user sign-in link", () => {
    renderPage();
    expect(screen.getByRole("link", { name: "Sign in" })).toHaveAttribute(
      "href",
      "/login",
    );
  });

  it("surfaces a not_allowed error if routed back from a rejected sign-up", () => {
    searchParams = new URLSearchParams("error=not_allowed");
    renderPage();
    expect(screen.getByRole("alert")).toHaveTextContent(/not on the allowlist/);
  });
});
