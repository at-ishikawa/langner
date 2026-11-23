import { StrictMode } from "react";
import { render } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import AuthCallbackPage from "./page";
import { getAccessToken, clearAccessToken } from "@/lib/authToken";

const replaceMock = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace: replaceMock }) }));

function renderPage() {
  return render(
    <StrictMode>
      <ChakraProvider value={defaultSystem}>
        <AuthCallbackPage />
      </ChakraProvider>
    </StrictMode>,
  );
}

describe("AuthCallbackPage", () => {
  beforeEach(() => {
    replaceMock.mockReset();
    clearAccessToken();
  });

  it("stores the token and forwards to next, even when Strict Mode runs the effect twice", () => {
    window.history.replaceState(null, "", "/auth/callback#access_token=abc&next=%2Fbooks");
    renderPage();
    expect(getAccessToken()).toBe("abc");
    expect(replaceMock.mock.calls).toEqual([["/books"]]);
    expect(window.location.hash).toBe("");
  });

  it("sends a callback without a token to /login", () => {
    window.history.replaceState(null, "", "/auth/callback");
    renderPage();
    expect(replaceMock.mock.calls).toEqual([["/login"]]);
  });
});
