import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// Polyfill PointerEvent for jsdom (used by Chakra UI / zag-js)
if (typeof globalThis.PointerEvent === "undefined") {
  // @ts-expect-error minimal polyfill for testing
  globalThis.PointerEvent = class PointerEvent extends MouseEvent {
    readonly pointerId: number;
    constructor(type: string, params: PointerEventInit = {}) {
      super(type, params);
      this.pointerId = params.pointerId ?? 0;
    }
  };
}

afterEach(() => {
  cleanup();
});
