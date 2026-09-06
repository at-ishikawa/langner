"use client";

import { useSyncExternalStore } from "react";
import { IconButton, type IconButtonProps } from "@chakra-ui/react";
import { useUSVoice } from "@/lib/useUSVoice";

// speechSynthesis support never changes at runtime, so there is nothing to
// subscribe to — the store is read once per mount.
const noopSubscribe = () => () => {};
const clientSupported = () => typeof window !== "undefined" && !!window.speechSynthesis;
const serverSupported = () => false;

interface PronounceButtonProps {
  /** The word or phrase to speak. Also used in the accessible label. */
  text: string;
  label?: string;
  size?: IconButtonProps["size"];
}

// PronounceButton plays the US-English pronunciation of `text` via the browser
// Web Speech API. It renders nothing when speech synthesis is unsupported and
// never throws. Presentation-only: it reads/writes no learning history and
// makes no network call. Only render it where `text` is already revealed — see
// quiz-ui-invariants (never in a reverse quiz's question state).
export function PronounceButton({ text, label, size = "xs" }: PronounceButtonProps) {
  const usVoice = useUSVoice();

  // Read support SSR-safely: the server snapshot is false and the client
  // snapshot is the real value, so the server and the first client (hydration)
  // render agree (both null) and React reveals the button after hydration
  // WITHOUT a mismatch. Reading `window` directly during render instead left
  // the button missing on the live SSR'd app (jsdom tests, which never SSR,
  // could not catch it).
  const supported = useSyncExternalStore(noopSubscribe, clientSupported, serverSupported);

  if (!supported) return null;

  const speak = () => {
    window.speechSynthesis.cancel();
    const utterance = new SpeechSynthesisUtterance(text);
    utterance.lang = "en-US";
    if (usVoice) utterance.voice = usVoice;
    window.speechSynthesis.speak(utterance);
  };

  return (
    <IconButton
      aria-label={label ?? `Play pronunciation of ${text}`}
      title="Play pronunciation"
      size={size}
      variant="ghost"
      color="fg.muted"
      flexShrink={0}
      // Prevent the tap from stealing focus from the answer input during a quiz.
      onMouseDown={(e) => e.preventDefault()}
      onClick={speak}
    >
      🔊
    </IconButton>
  );
}
