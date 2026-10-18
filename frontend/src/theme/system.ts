// Langner "Reading Room" design system. One token source drives both themes;
// the app styles through these tokens (never hardcoded palette literals), so the
// whole product re-skins from here.
//
// Direction: a beautifully typeset reader/dictionary. WARM PAPER ground with
// near-black INK text (light is the hero; a warm-charcoal dark mirrors it).
// Headwords, titles and prompts get a characterful serif (Fraunces); UI is a
// clean grotesque (Hanken Grotesk); phonetics and SR numbers are mono
// (JetBrains Mono). One confident INK-INDIGO accent (colorPalette="brand");
// state colors (due=amber, correct=green, missed=red) stay separate from the
// accent and come from Chakra's built-in palettes. Font families resolve to the
// CSS variables next/font sets in layout.tsx, with print-safe fallbacks.
import { createSystem, defaultConfig, defineConfig } from "@chakra-ui/react";

const SERIF = 'var(--font-display), "Iowan Old Style", Georgia, "Times New Roman", serif';
const SANS = 'var(--font-sans), system-ui, -apple-system, "Segoe UI", Roboto, sans-serif';
const MONO = 'var(--font-mono), ui-monospace, "SF Mono", Menlo, Consolas, monospace';

const config = defineConfig({
  globalCss: {
    "html, body": { bg: "bg", color: "fg", lineHeight: "1.6" },
    // Editorial headings: tighter leading, balanced wrapping, warm ink.
    "h1, h2, h3, h4": { fontFamily: "heading", letterSpacing: "-0.01em", textWrap: "balance" },
    "::selection": { background: "{colors.brand.emphasized}", color: "{colors.brand.contrast}" },
    "*:focus-visible": { outline: "2px solid {colors.brand.focusRing}", outlineOffset: "2px" },
  },
  theme: {
    tokens: {
      fonts: {
        heading: { value: SERIF },
        body: { value: SANS },
        mono: { value: MONO },
      },
      // Ink-indigo accent ramp.
      colors: {
        brand: {
          50: { value: "#eef1fb" },
          100: { value: "#dfe4f8" },
          200: { value: "#c2cbf0" },
          300: { value: "#9ba8e6" },
          400: { value: "#7482d7" },
          500: { value: "#555fc4" },
          600: { value: "#4444ab" },
          700: { value: "#39388c" },
          800: { value: "#302f70" },
          900: { value: "#2a2a5c" },
          950: { value: "#1a1938" },
        },
      },
    },
    semanticTokens: {
      colors: {
        // Warm "paper" neutrals (light is the hero); a warm charcoal dark mirror.
        // Deliberately warm so nothing reads as a stock cool-gray UI.
        bg: {
          DEFAULT: { value: { base: "#f7f5f0", _dark: "#171612" } },
          subtle: { value: { base: "#f1eee6", _dark: "#1c1b16" } },
          muted: { value: { base: "#e9e5d9", _dark: "#23221c" } },
        },
        fg: {
          DEFAULT: { value: { base: "#1f1d18", _dark: "#ece7dc" } },
          muted: { value: { base: "#6b6456", _dark: "#a7a090" } },
        },
        border: {
          DEFAULT: { value: { base: "#e4ded0", _dark: "#302e26" } },
        },
        // Panels/cards sit just above the paper ground.
        panel: {
          DEFAULT: { value: { base: "#fffdf8", _dark: "#201f19" } },
        },
        brand: {
          solid: { value: { base: "{colors.brand.700}", _dark: "{colors.brand.400}" } },
          contrast: { value: { base: "#fffdf8", _dark: "#14132a" } },
          fg: { value: { base: "{colors.brand.700}", _dark: "{colors.brand.300}" } },
          muted: { value: { base: "{colors.brand.100}", _dark: "{colors.brand.900}" } },
          subtle: { value: { base: "{colors.brand.50}", _dark: "{colors.brand.950}" } },
          emphasized: { value: { base: "{colors.brand.200}", _dark: "{colors.brand.800}" } },
          focusRing: { value: { base: "{colors.brand.600}", _dark: "{colors.brand.400}" } },
        },
      },
      radii: {
        l1: { value: "8px" },
        l2: { value: "12px" },
        l3: { value: "16px" },
      },
    },
  },
});

export const system = createSystem(defaultConfig, config);
