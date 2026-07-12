# Browser Extension (PoC)

A proof-of-concept browser extension that captures subtitles from streaming video services and generates a Langner story notebook. The notebook can be previewed in the popup and downloaded as a YAML file or copied to the clipboard.

The extension works by intercepting the subtitle files that the video player downloads (WebVTT, TTML, YouTube JSON). This is more stable than reading the page DOM and is portable across services.

## Currently supported

- **YouTube** — captures via `timedtext` JSON or `get_transcript` API
- **Netflix** — captures TTML subtitle files
- **Generic** — any site that loads `.vtt` or `.ttml` files

Adding a new service usually means adding a URL pattern in `src/subtitle/patterns.ts` and, if it uses a custom subtitle format, a parser in `src/subtitle/parsers.ts`.

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

1. Open a video in a supported service (YouTube, Netflix, etc.)
2. Make sure captions/subtitles are turned on — this triggers the subtitle file download
3. Play the video for a few seconds so the subtitle file is loaded
4. Click the Langner extension icon in the toolbar
5. Click "Capture Subtitles"
6. Review the preview (scenes, captions count)
7. Click "Download YAML" or "Copy"

## Development

```bash
pnpm dev          # HMR dev server; auto-rebuilds on file change
pnpm build        # Production build
```

On WSL, `pnpm dev` cannot auto-launch Chrome; load `.output/chrome-mv3-dev/` manually via "Load unpacked".

## Tests

```bash
pnpm test         # Unit tests (vitest)
pnpm test:watch   # Unit tests in watch mode
pnpm build && pnpm test:e2e   # E2E tests (playwright, requires build first)
```

## Architecture

```
browser/
  src/
    types.ts                   Shared types (StoryNotebook, TranscriptCue, etc.)
    subtitle/
      patterns.ts              URL patterns identifying subtitle requests
      parsers.ts               WebVTT / TTML / YouTube JSON parsers
    notebook/
      segmentation.ts          Splits cues into scenes (chapter- or gap-based)
      builder.ts               Assembles a StoryNotebook from captured data
      yaml.ts                  Serializes to Langner story notebook YAML
  entrypoints/
    interceptor.content.ts     Content script — injects page script that wraps
                               fetch/XHR to capture subtitle responses
    popup/                     Popup UI (capture button, preview, download/copy)
```

## Limitations

- Subtitle capture requires captions to be enabled and the video to have played long enough for the subtitle file to load.
- No automatic definition/idiom extraction (the `definitions` field is empty).
- Netflix support is implemented at the pattern level but not end-to-end tested.
- Subtitle URLs are often session-bound; if the extension is opened on a stale tab, reloading the page may be needed.
