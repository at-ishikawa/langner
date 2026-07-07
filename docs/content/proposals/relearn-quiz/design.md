---
title: "UI/UX Design"
weight: 2
---

# UI/UX Design

Mobile-first design (375px viewport) for the Relearn Quiz. The Relearn Quiz reuses the existing quiz layout patterns — start screen, progress bar, fixed-bottom submit/next buttons, green/red feedback banner, and the result card — so it feels like the other quizzes. What is new is (1) a **looping** session driven by a client-side working queue instead of a fixed card count, and (2) a **richer feedback screen** that folds the Learn-page context (conversations, etymology, related-word graph) in below the usual result card.

## Screen Flow

```
Quiz Hub ──"Relearn" card──▶ /quiz/relearn (Start) ──Start──▶ /quiz/relearn/session
                                                                     │
                                                   answer ──correct──▶ word leaves queue
                                                     │      wrong/skip─▶ word to back of queue
                                                     ▼
                                             queue empty ──▶ /quiz/relearn/complete
```

The Relearn Quiz is entered from the Quiz Hub like any other mode. Unlike the other modes, it has **no notebook selection** — the pool is always "all words I recently got wrong" — so its start screen only offers the time-window control.

## Screen R0: Quiz Hub Entry

The Relearn Quiz appears as an additional card on the existing Quiz Hub (or the vocabulary tab of it), alongside Standard / Reverse / Freeform.

- **Relearn** card: "Re-drill the words you recently got wrong. Off the record — nothing is saved."
- The description makes the practice-only nature explicit so the learner understands this session does not affect their schedule or stats.
- Tapping the card navigates to the Relearn start screen.

## Screen R1: Start Screen (`/quiz/relearn`)

The start screen for the Relearn Quiz. There is no notebook list — only the look-back window and the resulting pool size.

```
┌─────────────────────────────────────┐
│ ◀ Relearn Quiz                      │
│                                     │
│  Practice the words you recently    │
│  got wrong. Nothing here is saved   │
│  to your history or schedule.       │
│                                     │
│  Look back over the last:           │
│   ( ) 6 hours                       │
│   ( ) 12 hours                      │
│   (•) 24 hours   (default)          │
│   ( ) 48 hours                      │
│                                     │
│   14 words to relearn               │
│                                     │
│        [    Start    ]              │
└─────────────────────────────────────┘
```

- **Header**: back arrow + "Relearn Quiz", back link returns to the Quiz Hub.
- **Explainer line**: reiterates that the session writes nothing.
- **Window selector**: radio group of preset look-back windows; 24 hours is the default. (The value maps to the `window_hours` request field, clamped server-side.)
- **Pool count**: live count of words in the pool for the selected window, so the learner knows how much work the session holds. Updating the window refreshes the count.
- **Start button**: fixed at the bottom (same position as existing quizzes). Disabled while the count is loading.

### Screen R1-empty: Start Screen — Nothing to Relearn

Shown when the pool is empty for the selected window.

```
┌─────────────────────────────────────┐
│ ◀ Relearn Quiz                      │
│                                     │
│            ┌───────┐                │
│            │  🎉   │                │
│            └───────┘                │
│                                     │
│   Nothing to relearn.               │
│   You're all caught up for the      │
│   last 24 hours.                    │
│                                     │
│   Look back over the last: [48h ▾]  │
│                                     │
│        [ Back to Quiz Hub ]         │
└─────────────────────────────────────┘
```

- Positive empty state (mirrors the "All correct on this day" state in Quiz Analytics).
- The learner can widen the window inline to pull in older mistakes without leaving the screen.

## Screen R2: Quiz Card (`/quiz/relearn/session`)

The recognition card. Every word — no matter which quiz originally produced the wrong answer — is shown in the same format: the expression on top, an input for its meaning below.

```
┌─────────────────────────────────────┐
│ ████████░░░░░░░░  9 words left       │
│                                     │
│           ephemeral                 │
│        missed in Reverse            │
│                                     │
│  Your meaning:                      │
│  ┌───────────────────────────────┐  │
│  │                               │  │
│  └───────────────────────────────┘  │
│                                     │
│         [ Skip ]   [ Submit ]       │
└─────────────────────────────────────┘
```

- **Progress bar + counter** at the top, styled like the existing quizzes — but the counter shows **"N words left"** (distinct words remaining in the working queue), not "card X of Y", because the queue only shrinks as words are cleared and a wrong/skipped word comes back later.
- **Word card**: the expression, prominent. Below it, a small muted **origin label** ("missed in Reverse", "missed in Etymology Breakdown", "missed in Notebook") for context only — it does not change how the word is asked.
- **Meaning input**: a single text field, auto-focused.
- **Submit** and **Skip** buttons fixed at the bottom.
  - **Submit** grades the typed meaning.
  - **Skip** counts as not-correct: the word goes to the back of the queue (same as a wrong answer) and the feedback screen is shown so the learner still sees the meaning and context.

## Screen R3: Feedback (`/quiz/relearn/session`, feedback state)

After Submit or Skip, the card transitions to the feedback state. The top of the screen is the **familiar result card**; below it is the **new Learn-page context** section.

Following the existing optimistic-transition pattern, the layout switches to the feedback shape immediately and the verdict/meaning/reason fill in when the grading RPC returns.

### R3a: Result card (top)

```
┌─────────────────────────────────────┐
│  ✓ Correct                          │  ← green banner (or red ✗ Incorrect)
│                                     │
│  ephemeral                          │
│  lasting a very short time          │
│                                     │
│  Your answer: "short-lived"         │
│  Looks right — a synonym of the     │
│  intended meaning.                  │
│                                     │
└─────────────────────────────────────┘
```

- Reuses the existing quiz result card: green/red banner, the expression, the canonical meaning, the learner's answer, and the grader's reason.
- **Crucially, this card shows NO "next review date" and NO Override / Change-Date actions.** Those surfaces exist only for quizzes that write history. Because the Relearn Quiz records nothing, there is nothing to override and no schedule to show. The result card is display-only here.

### R3b: Learn-page context (below the result card)

The distinguishing feature of the Relearn Quiz. Below the verdict, the same context the Learn page shows for a word is rendered inline, so re-drilling doubles as re-reading.

```
├─────────────────────────────────────┤
│  Where it appears                   │
│  ┌───────────────────────────────┐  │
│  │ “It was an ephemeral moment,   │  │
│  │  gone before she could speak.” │  │
│  │  — Scene 3, chapter 2          │  │
│  └───────────────────────────────┘  │
│                                     │
│  Origin                             │
│  ephemeros (Greek) — lasting a day  │
│  Related: ephemera · ephemeral      │
│  ┌───────────────────────────────┐  │
│  │   [ relation graph preview ]   │  │
│  └───────────────────────────────┘  │
│                                     │
│           [    Next    ]            │
└─────────────────────────────────────┘
```

- **"Where it appears"** — the conversations/statements that contain the expression, with the expression highlighted, exactly as the Learn page renders them. For story notebooks this is the scene conversation; for flashcards it is the example sentences.
- **"Origin"** — the word's etymology origin (if any) plus the related words that share that origin, and the relation graph, reusing the Learn page's etymology assembly and graph.
- **Next** button, fixed at the bottom, advances to the next word in the working queue (or to the complete screen if the queue is now empty).
- If a word has no context or no etymology, that sub-section is omitted (not shown empty), matching the Learn page's behavior.

The context sections are the same building blocks the Learn page already uses (see the [Frontend Design]({{< relref "frontend-design" >}})), extracted into a shared component so the two surfaces cannot drift.

## Screen R4: Session Complete (`/quiz/relearn/complete`)

Shown when the working queue is empty — every word has been answered correctly at least once.

```
┌─────────────────────────────────────┐
│            ┌───────┐                │
│            │  ✅   │                │
│            └───────┘                │
│                                     │
│   Relearn complete                  │
│   You cleared all 14 words.         │
│                                     │
│   Total answers: 21                 │
│   (some came around more than once) │
│                                     │
│   Nothing was saved to your         │
│   history or schedule.              │
│                                     │
│   [ Relearn again ]  [ Quiz Hub ]   │
└─────────────────────────────────────┘
```

- **Summary**: number of distinct words cleared, and the total number of answers given (which can exceed the word count because wrong/skipped words looped back).
- **Reassurance line**: restates that nothing was saved — the defining property of this quiz.
- **"Relearn again"**: re-fetches the pool for the same window. Words cleared in the session just finished are excluded (via the relearn-clear markers), so a second run only surfaces anything the learner *newly* got wrong elsewhere in the meantime — usually the empty state.
- **"Quiz Hub"**: returns to the hub.
- Unlike the other quizzes' Session Complete screen, there is **no per-word result list with Override / Change-Date actions**, because there is no history to act on.

## The Looping Session (Interaction Model)

The core interaction that differs from every existing quiz: the session is a **loop over a working queue**, not a fixed list.

| Event | Effect on the working queue |
|-------|-----------------------------|
| Answer **correct** | The word is removed from the queue and never asked again this session. A relearn-clear marker is recorded for it. |
| Answer **wrong** | The word is moved to the **back** of the queue; it will come around again later this session. |
| **Skip** | Same as wrong: moved to the back of the queue. |
| Queue becomes **empty** | The session ends and navigates to the Complete screen. |

- The queue is seeded once, at session start, from the pool returned by the start RPC. No new words enter mid-session.
- The **"N words left"** counter reflects the number of distinct words still in the queue, so it strictly decreases over the session (a wrong answer keeps the count the same; a correct answer lowers it by one).
- Because the loop is entirely client-side, the learner can leave and the session simply ends — there is nothing to persist or resume. Re-entering starts a fresh pool.

## Reuse of Existing UI Patterns

The Relearn Quiz intentionally reuses the current quiz UI so it feels native:

| Pattern | Source | Used In |
|---------|--------|---------|
| Start screen shell + fixed-bottom Start | Existing quiz start | Screen R1 |
| Progress bar + counter | Standard/Reverse quiz | Screen R2 (counter relabeled "words left") |
| Word card + meaning input | Standard quiz | Screen R2 |
| Fixed-bottom Submit / Next | All existing quizzes | Screens R2, R3 |
| Green/red feedback banner + result card | All existing quizzes | Screen R3a |
| Optimistic transition to feedback | Freeform/Standard quiz | Screen R3 |
| Positive empty state | Quiz Analytics "all correct" | Screens R1-empty, R4 |
| Conversation / example context with highlight | Learn page | Screen R3b |
| Etymology origin + related words + relation graph | Learn page | Screen R3b |

## What Is Deliberately Absent

Because the Relearn Quiz never writes history, several familiar quiz surfaces are intentionally **removed**, not just hidden:

- **No next-review date** on the feedback card.
- **No Override / Mark-as-Correct / Change-Review-Date** actions anywhere (no log exists to override).
- **No Session Complete result list** with per-word history actions.
- **No appearance in Quiz Analytics** — a Relearn session leaves no trace on the Day List or Day Detail pages.

These absences are the visible expression of the invariant that the Relearn Quiz is off the record.

## Accessibility

- The word card exposes the expression as an `h1`/`h2` and the origin label as muted supporting text with an `aria-label` ("originally missed in the Reverse quiz").
- Submit and Skip are real `button` elements; Skip has `aria-label="Skip and see the answer"`.
- The "N words left" counter uses `aria-live="polite"` so screen-reader users hear the queue shrink.
- The result banner pairs the ✓ / ✗ glyph with the words "Correct" / "Incorrect" so the verdict is never color-only.
- The relation graph preview has a text alternative listing the related words, since the graph itself is visual.
