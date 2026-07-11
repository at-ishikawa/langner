---
title: "Frontend Design"
weight: 4
---

# Frontend Design

The Relearn Quiz reuses the existing Next.js / React / Chakra UI / Zustand / Connect-web stack. The one genuinely new piece of frontend surface is the **client-driven working queue** — the existing quiz flow advances through a fixed card list with a monotonically increasing index and has no requeue primitive, so the loop-until-cleared behavior is new store code. Everything else (the result card, the connect client, the Learn-page context blocks) is reused.

This document grounds the design in the current code. Where the restart prompt referred to helpers by name (`buildFromConversations`, `buildWordDetail`, `LoadEtymologyExampleWords`), those names do not exist in the tree; the real symbols that provide that behavior are cited below so the implementation targets actual code.

## File Structure

New and touched files:

```
frontend/src/
  app/quiz/
    page.tsx                     # MODIFY: add a "Relearn" mode card to the hub
    relearn/
      page.tsx                   # NEW: start screen (window selector + pool count)
      session/
        page.tsx                 # NEW: looping card + rich feedback
      complete/
        page.tsx                 # NEW: session-complete summary
  store/
    relearnStore.ts              # NEW: isolated working-queue store (kept out of quizStore)
  components/
    RelearnContext.tsx           # NEW: Learn-page context block (scenes + related words + graph)
  app/quiz/page.tsx              # MODIFY: add a Relearn entry card to the hub
  lib/
    client.ts                    # MODIFY: re-export Relearn request/response types
```

The working queue lives in its **own** `relearnStore.ts` rather than in `quizStore.ts`: the loop needs a requeue primitive the linear `currentIndex` store lacks, and isolating it keeps the existing store (and its tests) untouched. The feedback verdict is a small self-contained block in the session page rather than a reuse of `QuizResultCard` — the Relearn response has no `learned_at`/`next_review_date`, so a dedicated banner is simpler than mapping to a `ResultItem`. It carries a **session-only** Mark-as-Correct/Incorrect override (see below) that affects the working queue and the off-the-record clear marker, never learning history.

## How Relearn Slots Into the Existing Modes

The existing quiz modes are represented in three parallel places (the `quizStore.ts` string `QuizType` union, the proto `QuizType` enum, and the hub's `vocabularyModes`/`etymologyModes` config with per-mode `/quiz/<mode>` routes). Relearn deliberately does **not** thread through all of them, because it is a self-contained flow with its own store and routes:

1. **Proto enum** — `QUIZ_TYPE_RELEARN` is added to the proto `QuizType` (label-only, never persisted; see [Backend Design]({{< relref "backend-design" >}})). It labels a pooled word's `source_quiz_type`.
2. **Store** — Relearn uses its own `relearnStore.ts`; it is **not** added to the `quizStore.ts` string union (which drives the override/analytics result actions Relearn has none of).
3. **Hub** — because Relearn spans all quiz types (not one tab), the hub gets a first-class **Relearn card above the tabs** (`app/quiz/page.tsx`), styled like the mode cards (purple accent, title + description + `›`) so it's easy to find and consistent, routing to `/quiz/relearn` — which then owns its own start → session → complete pages. It is not wired into `handleStart`.

## Start Screen — `app/quiz/relearn/page.tsx`

Unlike the other modes, Relearn has **no notebook selection**. The screen holds a window selector and a live pool count.

- On mount (and whenever the window changes), call `quizClient.startRelearnQuiz({ windowHours })` to get the pool. The start screen can either call it once on Start, or use a lightweight count — the [Backend Design]({{< relref "backend-design" >}}) specifies `StartRelearnQuiz` returning the cards, so the simplest approach is to call it on Start and show the count from a prior fetch; a dedicated count RPC is not required for v1.
- The window presets (6/12/24/48h) map to the `window_hours` request field. 24 is the default.
- On **Start**: seed the working queue (below) from `response.cards` and `router.push("/quiz/relearn/session")`.
- Empty pool → the positive empty state; the learner can widen the window inline.

Client setup is unchanged — `frontend/src/lib/client.ts` already exports `quizClient = createClient(QuizService, transport)`; the new Relearn RPCs come along with the regenerated `QuizService`.

## Working Queue (New Store Surface)

The existing store advances linearly: `nextCard: () => set((s) => ({ currentIndex: s.currentIndex + 1 }))` (`quizStore.ts:256–265`) over a fixed `flashcards: Flashcard[]`, with append-only `results`. There is **no requeue**. The Relearn loop needs a queue that can send a word to the back.

Proposed slice (either added to `quizStore.ts` under the `relearn` mode or in a small `relearnStore.ts`):

```ts
interface RelearnCardVM {
  noteId: bigint;
  entry: string;                 // the expression shown; also the meaning-grading key
  sourceQuizType: string;        // "reverse" | "notebook" | "etymology_breakdown" | …  (label only)
  // context payload for the feedback screen (scenes, graph, example words) — see below
}

interface RelearnState {
  queue: RelearnCardVM[];        // working queue; front is the current card
  clearedCount: number;          // distinct words answered correctly
  totalAnswers: number;          // grows past clearedCount because wrong/skip re-queue
  seedQueue: (cards: RelearnCardVM[]) => void;
  // pull the front card off; if correct -> drop it; else push it to the back
  resolveFront: (correct: boolean) => void;
  reset: () => void;
}
```

- `seedQueue` is called once from the start screen; no cards enter later.
- `resolveFront(true)` removes the front word (`clearedCount++`); `resolveFront(false)` moves the front word to the back. Both increment `totalAnswers`.
- **"N words left"** on the progress bar is simply `queue.length` (distinct remaining), which strictly decreases as words clear. This replaces the "card X of Y" counter used by the linear quizzes.
- Session ends when `queue.length === 0` → `router.push("/quiz/relearn/complete")`.

Because the loop is entirely client-side, leaving the page just discards the queue; there is nothing to persist or resume, matching the stateless-backend design.

## Card + Feedback — `app/quiz/relearn/session/page.tsx`

### Answering

- Render the front card **in the format of its `sourceQuizType`**, reusing the shared `AnswerInput` so the layout matches the other quizzes. The prompt heading, hint, input label, and placeholder branch on the format: recognition shows the expression + examples and asks the meaning; **reverse shows the meaning + masked `contexts` and asks the word**; etymology-standard shows the origin (+ type · language) and asks the meaning; etymology-reverse shows the meaning (+ type · language) and asks the origin. The card data comes from the new `RelearnCard` fields (`meaning`, `examples`, `contexts`, `type`, `language`).
- **Submit** calls `quizClient.submitRelearnAnswer({ noteId, answer, responseTimeMs })`. Following the existing optimistic-transition pattern (`QuizPhase = "answering" | "grading" | "feedback"` mirrors `quiz/standard/page.tsx:22`), the UI flips to the feedback layout immediately and fills in the verdict when the RPC returns.
- **Skip** submits with `isSkipped: true`; the backend grades it as wrong but still returns the meaning + context, so the learner sees the answer. It is treated exactly like a wrong answer for the queue.
- Relearn submits **one card at a time** (not batched). The batched path used by the linear quizzes exists to write many logs efficiently; Relearn writes nothing and the loop needs a per-card decision, so the single-shot `submitRelearnAnswer` (with `batchSubmitRelearnAnswers` available for parity) is used. See [Backend Design]({{< relref "backend-design" >}}).

### Result verdict + session-only override

The feedback verdict is a small self-contained block in the session page: a green/red banner (`✓ Correct` / `✗ Incorrect`), the expression, its meaning, and the grader's reason. It is **not** a reuse of `QuizResultCard` (the Relearn response carries no `learned_at` / next-review date, so there is no schedule to show).

It **does** carry a **Mark as Correct / Mark as Incorrect** toggle — because the OpenAI meaning grader is imperfect and can mark a correct answer wrong. But unlike the normal quizzes, where that button edits the learning log and reschedules SM-2, here it is **session-only**: it flips the verdict the working queue uses (`resolveFront`) so the learner isn't forced to re-drill a word they know, and it calls `quizClient.overrideRelearnCard({ noteId, markCorrect })` to record/remove the off-the-record clear marker for the *next* session. It writes **no** learning history. The banner reflects the effective (post-override) verdict and shows an "(overridden)" tag; the override RPC is called on **Next** only when the learner actually flipped the grader's verdict.

### Learn-page context block (the distinguishing feature)

Below the result card, the feedback screen renders the Learn-page context for the word. The restart prompt named helpers that do not exist in the tree (`buildFromConversations`, `buildWordDetail`, `LoadEtymologyExampleWords`); the real building blocks are:

| Restart-prompt name | Actual code | Location |
|---------------------|-------------|----------|
| `buildFromConversations` / `SceneContent` | conversations/statements rendering (`SceneContent` + `renderHighlightedText`) | `app/learn/[id]/page.tsx:290`, `:42` (inline) |
| `LoadEtymologyExampleWords` | server-delivered `example_words` | `SubmitRelearnAnswerResponse.exampleWords` |
| `graph_context` / `RelationGraph` | `RelationGraph` (read-only, `compact`) | `components/RelationGraph.tsx` (already shared) |

Implementation: a new shared component **`components/RelearnContext.tsx`** renders the context the backend ships on `SubmitRelearnAnswerResponse`:

1. **"Where it appears"** — the response's `contextScenes` (statements + speaker/quote conversation lines), with every occurrence of the expression bolded by a small local `highlightEntry` helper (unit-tested).
2. **"Related words"** — the `exampleWords` chip row.
3. **"Origin"** — the etymology `graphContext` via `<RelationGraph prompt={graphContext} value="" onValueChange={() => {}} disabled compact />`, exactly as `QuizResultCard.tsx:258–268` renders it.

Every sub-section is omitted when its data is empty, so a word with no context or no etymology renders nothing extra. Because the whole payload arrives on the submit response, the feedback screen needs no extra round-trips.

**Deferred:** the restart prompt suggested extracting `SceneContent` + `renderHighlightedText` out of `app/learn/[id]/page.tsx` into a shared module so the Learn page and the Relearn feedback share one implementation. That refactor is intentionally **not** done here — those helpers are tightly coupled to the story reader's deep-link/target-word ref logic, and rewiring them risks destabilizing the Learn page and its e2e coverage. `RelearnContext` implements its own small, self-contained highlighter instead; folding the two into one shared component is a low-risk follow-up.

## Complete Screen — `app/quiz/relearn/complete/page.tsx`

- Reads `clearedCount` and `totalAnswers` from the queue slice.
- Renders the summary, the "nothing was saved" reassurance, and **"Relearn again" / "Quiz Hub"** buttons.
- Unlike `app/quiz/complete/page.tsx` (which builds `allResults` from the store result arrays and renders `QuizResultsGroupedList` with Override/Skip/Resume handlers), the Relearn complete screen shows **no per-word result list with history actions**, because there is no history to act on. "Relearn again" calls `reset()` then re-fetches the pool for the same window.

## Connect RPC Client

No client wiring changes beyond regeneration. `frontend/src/lib/client.ts` already does:

```ts
export const quizClient = createClient(QuizService, transport);
```

After the proto adds `StartRelearnQuiz` / `SubmitRelearnAnswer` / `BatchSubmitRelearnAnswers`, the regenerated `QuizService` exposes `quizClient.startRelearnQuiz(...)` / `quizClient.submitRelearnAnswer(...)` with no manual client changes. Request/response types are re-exported from `client.ts` alongside the existing ones.

## Testing

**Vitest + React Testing Library** (unit), covering the new code:

- **Queue reducer** (`relearnStore.test.ts`): `resolveFront(true)` drops the front and increments `clearedCount`; `resolveFront(false)` moves the front to the back and leaves `clearedCount` unchanged; the session ends only when `queue.length === 0`; a word answered wrong then correct is cleared exactly once; `resolveFront` on an empty queue is a no-op.
- **Session loop** (`session/page.test.tsx`): submit → feedback → Next clears the word; a wrong answer requeues it to the back; skip submits `isSkipped`; clearing the last word routes to `/quiz/relearn/complete`; a grading error returns to the answering phase.
- **Start / complete pages**: pool count, empty state, window change, Start seeds the queue; the summary counts and the reset-and-navigate buttons.
- **Context**: `RelearnContext` renders nothing when empty, and renders statements/conversations, related-word chips, and the relation graph when present; `highlightEntry` bolds occurrences.

**Playwright BDD** (e2e, `relearn.feature`): answer a Standard card wrong to seed a fresh `misunderstood` log, then open the Relearn Quiz, start the session, and clear every pooled word through to the complete page — exercising all three routes end-to-end against the real backend.
