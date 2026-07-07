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
    quizStore.ts                 # MODIFY: add "relearn" to QuizType; add a working-queue slice
    relearnStore.ts              # NEW (option): isolate the queue slice if quizStore grows too large
  components/
    LearnContext.tsx             # NEW: shared context block (extracted from learn/[id]/page.tsx)
    QuizResultCard.tsx           # REUSE: result card (override buttons stay hidden — see below)
  lib/
    client.ts                    # MODIFY: re-export Relearn request/response types
    quizResultItems.ts           # MODIFY: relearnResultToItem mapper
  app/learn/[id]/page.tsx        # MODIFY: extract SceneContent + renderHighlightedText into LearnContext
```

## Quiz Mode Representation

A quiz mode exists in three parallel places today; a consistent `relearn` mode touches each:

1. **Store string union** — `frontend/src/store/quizStore.ts:3`:
   ```ts
   export type QuizType = "standard" | "reverse" | "freeform"
     | "etymology-standard" | "etymology-reverse" | "etymology-freeform";
   ```
   Add `"relearn"`.

2. **Proto enum** — `QuizType` in `gen-protos/api/v1/quiz_pb.ts` (`STANDARD=1 … ETYMOLOGY_FREEFORM=6`). Add the label-only `QUIZ_TYPE_RELEARN`. Note (see [Backend Design]({{< relref "backend-design" >}})) this enum value is **never persisted** — it is a UI label only.

3. **Hub UI config + routing** — `app/quiz/page.tsx` has `vocabularyModes` / `etymologyModes` arrays of `{ key, title, description }` (around lines 28–38) and a `handleStart` switch (lines 303–391) that calls the right `start*` RPC and `router.push`es to a literal `/quiz/<mode>` folder (there is no dynamic `[mode]` route). Add a Relearn card and a branch that pushes to `/quiz/relearn`.

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

- Render the front card: the expression as a heading, the muted `sourceQuizType` label ("missed in Reverse"), and a single meaning `AnswerInput` (reused from the standard quiz).
- **Submit** calls `quizClient.submitRelearnAnswer({ noteId, answer, responseTimeMs })`. Following the existing optimistic-transition pattern (`QuizPhase = "answering" | "grading" | "feedback"` mirrors `quiz/standard/page.tsx:22`), the UI flips to the feedback layout immediately and fills in the verdict when the RPC returns.
- **Skip** resolves the front card as not-correct without an RPC grade, but still shows the feedback screen (so the learner sees the meaning + context). It is treated exactly like a wrong answer for the queue.
- Relearn submits **one card at a time** (not batched). The batched `batchSubmitAnswers` path used by the linear quizzes exists to write many logs efficiently; Relearn writes nothing and the loop needs a per-card decision, so the single-shot `submitRelearnAnswer` (with a `batchSubmitRelearnAnswers` available for parity) is used. See [Backend Design]({{< relref "backend-design" >}}).

### Result card (reused, with override intentionally inert)

The feedback verdict reuses `components/QuizResultCard.tsx`. Its override / undo controls are gated on **both `noteId` and `learnedAt`** (`QuizResultCard.tsx:272–284`):

```tsx
{!item.isOverridden && !item.isSkipped && item.noteId && item.learnedAt && (
  <Button onClick={() => onOverride(item)}>…</Button>
)}
```

The Relearn response deliberately carries **no `learnedAt`** (and no next-review date), so mapping a Relearn result into a `ResultItem` with `learnedAt` left undefined makes the override buttons **stay hidden automatically** — no special-casing needed. This is the frontend expression of the "writes nothing to history" invariant: there is no log to override, and the existing gate already hides the control. The `relearnResultToItem` mapper (in `lib/quizResultItems.ts`) sets `entry`, `meaning`, `correct`, `reason`, `userAnswer`, and the context fields, and leaves `learnedAt`/`nextReviewDate` unset.

Note `ResultItem` already has no `nextReviewDate` field (`QuizResultCard.tsx:15–47`) — the card never renders a review date — so nothing extra is needed to suppress it.

### Learn-page context block (the distinguishing feature)

Below the result card, the feedback screen renders the same context the Learn page shows. The real reusable pieces (the restart prompt's names do not exist):

| Restart-prompt name | Actual code | Location | Status |
|---------------------|-------------|----------|--------|
| `buildFromConversations` / `SceneContent` | `SceneContent` + `renderHighlightedText` | `app/learn/[id]/page.tsx:290`, `:42` | **inline — extract** |
| `buildWordDetail` | `WordDetailView` component + `buildOriginBreakdown` mapper | `components/WordDetailView.tsx`, `lib/quizResultItems.ts:10` | already shared |
| `LoadEtymologyExampleWords` | server-delivered `exampleWords` + the origin/related-words view | `notebooks/etymology/[id]/page.tsx` (`OriginDetailView`, `relatedDefs`) | server-provided |
| `graph_context` / `RelationGraph` | `RelationGraph` (read-only, `compact`) | `components/RelationGraph.tsx` | already shared |

Plan:

1. **Extract** `SceneContent` and `renderHighlightedText` from `app/learn/[id]/page.tsx` into a shared `components/LearnContext.tsx`. Today the module comment says they are "kept here rather than in /lib because it is only used by the story reader" — the Relearn feedback screen becomes the second consumer, which justifies extraction. The Learn page imports them back from the shared component (no behavior change), satisfying the "one source of truth" goal so the two surfaces cannot drift.
2. `LearnContext` renders, for the word: the **conversations/statements** it appears in (via `SceneContent`, with the expression highlighted by `renderHighlightedText`), the **word detail / origin** (via `WordDetailView`), the **related words** (`exampleWords` chip row), and the **relation graph** (`<RelationGraph prompt={graphContext} value="" onValueChange={()=>{}} disabled compact />`, exactly as `QuizResultCard.tsx:258–268` already does).
3. The context payload (scenes, `graphContext`, `exampleWords`) arrives on `SubmitRelearnAnswerResponse` so the client does not need extra round-trips. Sub-sections with no data are omitted (matching `SceneContent` hiding empty scenes and `WordDetailView` rendering nothing when empty).

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

## Testing (Vitest + React Testing Library)

- **Queue reducer**: `resolveFront(true)` drops the front and increments `clearedCount`; `resolveFront(false)` moves the front to the back and leaves `clearedCount` unchanged; the session ends only when `queue.length === 0`. A word answered wrong then correct is cleared exactly once.
- **Override buttons hidden**: mapping a Relearn result to a `ResultItem` with `learnedAt` undefined renders no Override / Mark-as-Correct button (asserts the gate).
- **Context omission**: a word with no scenes/etymology renders the result card with the context sub-sections absent, not empty.
- Tests live next to source (`*.test.tsx`), consistent with the existing frontend test layout (e.g. `quizStore.test.ts`).
