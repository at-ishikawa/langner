import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { PronounceButton } from "./PronounceButton";

// Minimal SpeechSynthesisUtterance stand-in — jsdom does not implement it.
class FakeUtterance {
  text: string;
  lang = "";
  voice: SpeechSynthesisVoice | null = null;
  constructor(text: string) {
    this.text = text;
  }
}

function installSpeech() {
  const speak = vi.fn();
  const cancel = vi.fn();
  vi.stubGlobal("speechSynthesis", {
    speak,
    cancel,
    getVoices: () => [],
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  });
  vi.stubGlobal("SpeechSynthesisUtterance", FakeUtterance);
  return { speak, cancel };
}

function renderButton(text: string) {
  return render(
    <ChakraProvider value={defaultSystem}>
      <PronounceButton text={text} />
    </ChakraProvider>,
  );
}

describe("PronounceButton", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe("when speech synthesis is available", () => {
    beforeEach(() => {
      installSpeech();
    });

    it("renders an accessible labeled button", () => {
      renderButton("break the ice");
      expect(
        screen.getByRole("button", { name: "Play pronunciation of break the ice" }),
      ).toBeInTheDocument();
    });

    it("cancels any ongoing speech before speaking, then speaks an en-US utterance", async () => {
      const { speak, cancel } = installSpeech();
      renderButton("break the ice");

      await userEvent.click(screen.getByRole("button"));

      expect(cancel).toHaveBeenCalledOnce();
      expect(speak).toHaveBeenCalledOnce();
      // cancel must run before speak.
      expect(cancel.mock.invocationCallOrder[0]).toBeLessThan(
        speak.mock.invocationCallOrder[0],
      );

      const utterance = speak.mock.calls[0][0] as FakeUtterance;
      expect(utterance.text).toBe("break the ice");
      expect(utterance.lang).toBe("en-US");
    });
  });

  describe("when speech synthesis is unsupported", () => {
    it("renders nothing and never throws", () => {
      vi.stubGlobal("speechSynthesis", undefined);
      const { container } = renderButton("break the ice");
      expect(container.firstChild).toBeNull();
    });
  });
});
