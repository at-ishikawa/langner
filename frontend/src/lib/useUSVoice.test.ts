import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useUSVoice } from "./useUSVoice";

function voice(name: string, lang: string): SpeechSynthesisVoice {
  return { name, lang, default: false, localService: true, voiceURI: name } as SpeechSynthesisVoice;
}

// installSpeech wires a fake speechSynthesis whose voice list starts empty and
// is filled only after a "voiceschanged" listener is invoked — mirroring how
// real browsers load voices asynchronously.
function installSpeech(loadedVoices: SpeechSynthesisVoice[]) {
  let voices: SpeechSynthesisVoice[] = [];
  const listeners: Array<() => void> = [];
  vi.stubGlobal("speechSynthesis", {
    getVoices: () => voices,
    addEventListener: (_type: string, cb: () => void) => listeners.push(cb),
    removeEventListener: vi.fn(),
  });
  return {
    fireVoicesChanged() {
      voices = loadedVoices;
      listeners.forEach((cb) => cb());
    },
  };
}

describe("useUSVoice", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns null before voices have loaded", () => {
    installSpeech([voice("Samantha", "en-US")]);
    const { result } = renderHook(() => useUSVoice());
    expect(result.current).toBeNull();
  });

  it("returns the preferred en-US voice after voiceschanged fires", () => {
    const { fireVoicesChanged } = installSpeech([
      voice("Daniel", "en-GB"),
      voice("Samantha", "en-US"),
      voice("Google US English", "en-US"),
    ]);
    const { result } = renderHook(() => useUSVoice());
    expect(result.current).toBeNull();

    act(() => {
      fireVoicesChanged();
    });

    // "Google US English" wins over "Samantha" and the generic en-US match.
    expect(result.current?.name).toBe("Google US English");
  });

  it("falls back to any en-US voice when no preferred name is installed", () => {
    const { fireVoicesChanged } = installSpeech([
      voice("Daniel", "en-GB"),
      voice("Alex", "en-US"),
    ]);
    const { result } = renderHook(() => useUSVoice());

    act(() => {
      fireVoicesChanged();
    });

    expect(result.current?.name).toBe("Alex");
  });
});
