# Browser Extension (PoC)

A proof-of-concept Chrome extension that captures the transcript of a YouTube video and generates a Langner story notebook. The notebook can be previewed in the popup and downloaded as a YAML file.

This is a standalone PoC with no backend integration. The downloaded YAML file can be placed manually into the configured `notebooks.stories_directories` to be picked up by Langner.

## Prerequisites

- Node.js 20+
- pnpm

## Setup

```bash
cd browser
pnpm install
```

## Build

```bash
pnpm build
```

The built extension is in `.output/chrome-mv3/`.

## Load in Chrome

1. Open `chrome://extensions`
2. Enable "Developer mode" (top right)
3. Click "Load unpacked"
4. Select the `browser/.output/chrome-mv3/` directory

## Usage

1. Open a YouTube video in Chrome
2. Open the transcript panel (click `...` below the video, then "Show transcript")
3. Click the Langner extension icon in the toolbar
4. Click "Capture Transcript"
5. Review the preview (scenes, captions count)
6. Click "Download YAML" to save the file, or "Copy" to copy to clipboard

## Development

```bash
pnpm dev          # HMR dev server with auto-reload
pnpm build        # Production build
```

## Tests

```bash
pnpm test         # Unit tests (vitest)
pnpm test:watch   # Unit tests in watch mode
pnpm build && pnpm test:e2e   # E2E tests (playwright, requires build first)
```

## Limitations

- YouTube only (no other video services)
- YouTube DOM selectors may need updating if YouTube changes its page structure
- No speaker identification (YouTube captions do not label speakers)
- No automatic definition/idiom extraction (the `definitions` field in the output is empty)
- The transcript panel must be open before capturing; the extension reads the DOM, not a network request
