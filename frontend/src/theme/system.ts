// Shared Chakra design system. Replaces the stock `defaultSystem` so every page
// draws from one set of tokens (brand accent, fonts, radii) and theme-aware
// semantic colors — instead of hardcoded palette literals + per-element `_dark`
// overrides. Extend the tokens here (not per page) to evolve the look globally.
import { createSystem, defaultConfig, defineConfig } from "@chakra-ui/react";

const SANS =
  'ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Apple Color Emoji", "Segoe UI Emoji", sans-serif';

const config = defineConfig({
  // Comfortable defaults for readability on desktop. body/html inherit the
  // theme-aware semantic bg/fg so a page never has to set them itself.
  globalCss: {
    "html, body": {
      bg: "bg",
      color: "fg",
      lineHeight: "1.6",
    },
  },
  theme: {
    tokens: {
      fonts: {
        heading: { value: SANS },
        body: { value: SANS },
      },
      // Brand accent ramp (a calm blue). Used via `colorPalette="brand"`.
      colors: {
        brand: {
          50: { value: "#eef3ff" },
          100: { value: "#dbe5ff" },
          200: { value: "#bccfff" },
          300: { value: "#92afff" },
          400: { value: "#6286fb" },
          500: { value: "#3d63ed" },
          600: { value: "#2b49cf" },
          700: { value: "#2239a6" },
          800: { value: "#203686" },
          900: { value: "#1f3270" },
          950: { value: "#161f45" },
        },
      },
    },
    semanticTokens: {
      colors: {
        // Makes `colorPalette="brand"` resolve (solid button, subtle bg, fg, …)
        // in both light and dark.
        brand: {
          solid: { value: "{colors.brand.500}" },
          contrast: { value: "white" },
          fg: { value: { base: "{colors.brand.700}", _dark: "{colors.brand.300}" } },
          muted: { value: { base: "{colors.brand.100}", _dark: "{colors.brand.900}" } },
          subtle: { value: { base: "{colors.brand.50}", _dark: "{colors.brand.950}" } },
          emphasized: { value: { base: "{colors.brand.200}", _dark: "{colors.brand.800}" } },
          focusRing: { value: "{colors.brand.500}" },
        },
      },
    },
  },
});

export const system = createSystem(defaultConfig, config);
