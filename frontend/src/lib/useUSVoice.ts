"use client";

import { useEffect, useMemo, useState } from "react";

// Preferred US-English voice names, in priority order. Falls back to any
// voice whose lang is "en-US" when none of these are installed.
const preferredNames = ["Google US English", "Samantha"];

function pickUSVoice(voices: SpeechSynthesisVoice[]): SpeechSynthesisVoice | null {
  for (const name of preferredNames) {
    const match = voices.find((v) => v.name === name);
    if (match) return match;
  }
  return voices.find((v) => v.lang === "en-US") ?? null;
}

// useUSVoice returns a memoized US-English SpeechSynthesisVoice, or null when
// speech synthesis is unavailable (SSR, unsupported browser) or no en-US voice
// is installed. getVoices() is often empty on first call, so it also subscribes
// to voiceschanged and updates once the browser has loaded its voice list.
export function useUSVoice(): SpeechSynthesisVoice | null {
  const [voices, setVoices] = useState<SpeechSynthesisVoice[]>([]);

  useEffect(() => {
    if (typeof window === "undefined" || !window.speechSynthesis) return;

    const synth = window.speechSynthesis;
    const update = () => setVoices(synth.getVoices());
    update();
    synth.addEventListener("voiceschanged", update);
    return () => synth.removeEventListener("voiceschanged", update);
  }, []);

  return useMemo(() => pickUSVoice(voices), [voices]);
}
