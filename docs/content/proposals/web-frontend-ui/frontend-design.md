---
title: "Frontend Design"
weight: 4
---

# Frontend Design

## File Structure

```
frontend/
  src/
    app/
      layout.tsx            # Root layout with Chakra UI provider
      page.tsx              # Quiz start screen
      quiz/
        page.tsx            # Quiz card + feedback screens
        complete/
          page.tsx          # Session complete screen
    components/             # Shared UI components
    store/                  # Zustand stores
    protos/                 # Generated protobuf/Connect code
  vitest.config.ts
  package.json
```

## Pages

| Route | Screen | Description |
|-------|--------|-------------|
| `/` | Quiz Start | Select notebooks, configure options, start quiz |
| `/quiz` | Quiz Card + Feedback | Show word, accept answer, show feedback |
| `/quiz/complete` | Session Complete | Show results with correct/incorrect word lists |

## State Management: Zustand

Zustand stores quiz state in memory at the module level. All pages access the same store without Context providers or prop drilling.

The store holds:
- Flashcards returned by `StartQuiz` RPC
- Current card index
- Accumulated results (correct/incorrect per card with meanings)

The quiz page reads flashcards and advances the index on each answer. The complete page reads the accumulated results. When the user returns to the start screen, the store resets.

### Why Zustand

- Shared across pages without wrapping in a Context provider
- Selective re-renders — components subscribe to only the state slices they need
- With 100-200 flashcards per session, avoids unnecessary re-renders that Context would cause on every state update
- ~1KB, minimal API

## Package Manager: pnpm

pnpm for fast installs and disk-efficient dependency management via a content-addressable store. Strict dependency resolution prevents accessing undeclared dependencies.

## Component Library: Chakra UI

Chakra UI provides interactive component primitives needed across screens: checkbox, button, input, card, progress bar, spinner, and layout utilities.

### Why Chakra UI

- Pre-built accessible components — checkbox groups, inputs, buttons, cards, progress bars
- Style props (`p={4}`, `bg="gray.100"`) without learning Tailwind CSS class conventions
- Easy theming for consistent look across 10-20 screens as the app grows
- Regular package — updates and bug fixes via `pnpm update`

## Connect RPC Client

The frontend calls the Go backend using `@connectrpc/connect-web`. TypeScript client code is generated from protobuf definitions by Buf tooling (`@bufbuild/protoc-gen-es` + `@connectrpc/protoc-gen-connect-es`). Generated code lives in `src/protos/`.

## Loading Strategy: Optimistic Transition

OpenAI grading adds latency to `SubmitAnswer` (1-3 seconds). To minimize perceived wait time:

1. User taps "Submit"
2. Immediately transition to the feedback card layout
3. Show the user's answer and a loading spinner for the verdict, meaning, and reason sections
4. When the RPC responds, fill in the correct/incorrect result, meaning, and reason

The UI feels responsive because the card layout transitions instantly — the user only waits for the small verdict section to appear.

## Test Framework: Vitest + React Testing Library

- **Vitest** — Test runner with Jest-compatible API (`describe`, `it`, `expect`). Fast, Vite-native, officially supported by Next.js.
- **React Testing Library** — Renders components and provides `render`, `screen`, `fireEvent` for testing user interactions.

Tests live next to their source files:
```
src/components/
  QuizCard.tsx
  QuizCard.test.tsx
```

No E2E testing for now.
