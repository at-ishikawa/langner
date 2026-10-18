// Langner "Lexicon" design system. One token source drives both themes; the
// app styles through these tokens (never hardcoded palette literals), so the
// whole product re-skins from here.
//
// Direction: a study tool about WORDS. Headwords/prompts get a reading serif;
// UI is a clean sans; phonetics and data are mono. One calm teal "ink" accent
// (colorPalette="brand"); state colors (due=orange, correct=green, missed=red)
// stay separate from the accent and come from Chakra's built-in palettes.
import { createSystem, defaultConfig, defineConfig } from "@chakra-ui/react";

const SERIF =
  '"Iowan Old Style", "Palatino Linotype", Palatino, Georgia, "Times New Roman", serif';
const SANS =
  'system-ui, -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif';
const MONO =
  'ui-monospace, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace';

const config = defineConfig({
  globalCss: {
    "html, body": { bg: "bg", color: "fg", lineHeight: "1.6" },
  },
  theme: {
    tokens: {
      fonts: {
        heading: { value: SERIF },
        body: { value: SANS },
        mono: { value: MONO },
      },
      // Teal "ink" accent ramp (Chakra cyan family).
      colors: {
        brand: {
          50: { value: "#ecfeff" },
          100: { value: "#cffafe" },
          200: { value: "#a5f3fc" },
          300: { value: "#67e8f9" },
          400: { value: "#22d3ee" },
          500: { value: "#06b6d4" },
          600: { value: "#0891b2" },
          700: { value: "#0e7490" },
          800: { value: "#155e75" },
          900: { value: "#164e63" },
          950: { value: "#083344" },
        },
      },
    },
    semanticTokens: {
      colors: {
        // Deliberately-chosen cool (blue-biased) neutrals, overriding Chakra's
        // default grays for the page ground + text so nothing reads as stock.
        bg: {
          DEFAULT: { value: { base: "#f4f6f7", _dark: "#0c131a" } },
          subtle: { value: { base: "#eef2f3", _dark: "#0f1920" } },
          muted: { value: { base: "#e7edef", _dark: "#131f28" } },
        },
        fg: {
          DEFAULT: { value: { base: "#14212b", _dark: "#e8eef2" } },
          muted: { value: { base: "#566a76", _dark: "#8ea3ae" } },
        },
        border: {
          DEFAULT: { value: { base: "#dde5e9", _dark: "#243742" } },
        },
        // Panels/cards sit just above the page ground.
        panel: {
          DEFAULT: { value: { base: "#ffffff", _dark: "#131f28" } },
        },
        brand: {
          solid: { value: { base: "{colors.brand.700}", _dark: "{colors.brand.400}" } },
          contrast: { value: { base: "white", _dark: "#04121a" } },
          fg: { value: { base: "{colors.brand.700}", _dark: "{colors.brand.300}" } },
          muted: { value: { base: "{colors.brand.100}", _dark: "{colors.brand.900}" } },
          subtle: { value: { base: "{colors.brand.50}", _dark: "{colors.brand.950}" } },
          emphasized: { value: { base: "{colors.brand.200}", _dark: "{colors.brand.800}" } },
          focusRing: { value: { base: "{colors.brand.600}", _dark: "{colors.brand.400}" } },
        },
      },
    },
  },
});

export const system = createSystem(defaultConfig, config);
