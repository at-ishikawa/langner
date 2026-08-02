# Quiz UI invariants

These invariants govern the **quiz-facing UI and its request/grade contract** for every quiz mode — vocabulary (flashcard / story / definitions), etymology origin, grammar (live quiz + grammar Relearn), and any future mode. They are the surface-layer complement to [[learning-history-invariants]] (which governs how a learning log is written, read, and displayed). Where a learning record is touched, both documents apply.

Check these before touching any of:

- `frontend/src/components/RelearnGrammarPost.tsx`, `frontend/src/app/quiz/relearn/session/page.tsx` — the grammar Relearn post + single-card relearn screen
- `frontend/src/app/quiz/grammar/page.tsx`, `frontend/src/components/GrammarFeedbackCard.tsx`, `frontend/src/components/GrammarCorrectionBody.tsx`, `frontend/src/lib/grammarSegments.ts` — the live grammar quiz + shared grammar feedback
- `frontend/src/app/quiz/etymology-origin/*`, the etymology origin family card and its per-word skip
- `frontend/src/components/QuizResultCard.tsx`, `frontend/src/components/FeedbackActions.tsx`, `frontend/src/components/AnswerInput.tsx` — the shared answer/feedback surfaces and the flags they read
- backend: `internal/server/quiz_handler_grammar.go`, `internal/server/quiz_handler_relearn.go`, `internal/server/quiz_handler_batch.go` (`skippedGradeResult`), `internal/quiz/word_actions.go` (`SkipWord` / `ResumeWord`), `internal/notebook/learning_history_updater.go` (`SetSkippedAt` / `ClearSkippedAt`)

---

## U1 — "Don't know" / skip is NOT "exclude from quizzes"

A per-item **"Don't know" / skip** means *"I can't answer this right now."* The item stays **due** and remains eligible for every future session, **including Relearn**. It is recorded as a normal not-yet-learned grade (a `misunderstood` / lowest-quality attempt) on the item's canonical storage key — the same key any other attempt uses (see [[learning-history-invariants]] L1–L4) — or, in Relearn, as nothing at all (Relearn persists no state).

A skip MUST NOT write the **exclude-from-quizzes marker**. In this codebase that marker is the per-quiz-type `skipped_at` field on a learning-history expression (`SkippedAtMap`), set only through the deliberate `SkipWord` RPC (`quiz.Service.SkipWord` → `LearningHistoryUpdater.SetSkippedAt`) and cleared by `ResumeWord` / `ClearSkippedAt`. Card loaders filter out any expression whose `skipped_at` is set, so writing it **removes the word from all future quizzes and from the Relearn pool** — the opposite of what "Don't know" should do.

**Consequence:** "Exclude from quizzes" is a separate, deliberate, separately-labeled user action (the **Exclude / Resume** button, e.g. `FeedbackActions` / `GrammarFeedbackCard`, or the per-blank **Exclude** in the grammar Relearn post). It is the ONLY thing that writes `skipped_at`. Never reach it from a "Don't know" / skip, from a normal wrong answer, or from *not* answering. A skipped or incorrect item must never be labeled **"Excluded"** in the UI — that word is reserved for `skipped_at`-excluded items.

### Grammar Relearn has no skip — unanswered defaults to *incorrect*

Grammar Relearn (`RelearnGrammarPost.tsx`) has **no skip / "Don't know"** control. A blank is exactly one of three states:

- **unanswered** → on "See answers" it is graded **incorrect** (a normal miss: empty answer, `is_skipped=false`, graded wrong deterministically). It stays **due** (Relearn persists nothing), and it MUST NOT be labeled or recorded as skipped and MUST NOT set `skipped_at`.
- **answered** → correct / incorrect from the grader.
- **Excluded** → the deliberate per-blank **Exclude** button, which calls `SkipWord` for the correction's `(notebookID, senseID)` — the same RPC every other card uses — and drops it from the post's active blanks and from all future quizzes / the Relearn pool.

A normal miss (incorrect) never sets the exclude marker; only the Exclude button does. The grammar card loader (`grammarMistakeDue`) filters out any correction whose `skipped_at` is set for `grammar`, so Exclude removes it from both the live grammar quiz and the Relearn pool. (The live grammar quiz and the single-card vocab/etymology Relearn screen still have their own "Don't know" skip, which U1/U2 continue to govern; only the grammar Relearn post dropped skip.)

## U2 — Skip and exclude are separate flags/paths, end to end

Keep "skip / don't-know" and "exclude" as **distinct flags and distinct code paths** at every layer: frontend action → request field → backend grade/write.

- **Skip:** a per-item request field (`is_skipped` on the submit/grade request) → the handler produces `skippedGradeResult()` (a plain wrong attempt) → written to the item's canonical key (or, in Relearn, not written at all). It is graded and displayed as a normal wrong/passed attempt.
- **Exclude:** its own RPC (`SkipWord` / `ResumeWord`) → sets/clears `skipped_at`. Nothing on the answering screen fires this.

The recurring bug is a **shared result-card / feedback component whose `isSkipped` (or similarly named) field the display and/or backend treat as "excluded from future quizzes."** Reusing that one field to also mean "the learner tapped Don't know" is what conflates the two — the item shows an "Excluded" badge / "Resume" button (and, if the write path is shared, actually gets excluded) even though the learner only meant to pass.

**Consequence:** when a result carries a skip, do NOT populate the shared "excluded" display flag from it, and do NOT route it through the exclude write path. Give "skip-for-now" its own field/label ("Don't know", "still due") separate from the exclude flag. Changing what "skip" does is then a one-path change that cannot leak into exclusion.

## U3 — Per-item feedback renders adjacent to its item and stays visible

On progressive / multi-item screens (the grammar Relearn post, the live grammar post, the etymology origin family card), the per-item feedback (Mistake / You wrote / Suggested / Why you missed it / grammar note) MUST render **adjacent to the item it concerns** and remain visible **without scrolling past the navigation controls**. Never render feedback *below* the "Next" / "See answers" button at the bottom of a long post — on a long journal entry it lands off-screen exactly when the learner needs it.

Acceptable placements: inline directly under the answered item, or a pinned/sticky panel (e.g. a `position="fixed"` bottom sheet) combined with scrolling the answered item into view. The feedback must be **associated with the correct item** (keyed by the item's id), and it opens **only when feedback is warranted** — for a wrong answer or a "Don't know" — never for a correct answer.

**Consequence:** the "Next" button must not sit above the feedback it is meant to follow. If feedback is a bottom sheet, reserve bottom padding so it never covers the item or the controls.

---

## Worked example — etymology-origin "Don't Know" wrongly excluded the origin

On `feat/etymology-origin-group-quiz` (commits `4c7fd4de`, `991816dd`), the Etymology Origin answering screen had one whole-card "Don't Know" wired to `handleSkip`, which sent a request-level `is_skipped=true` and — separately — passed that same `isSkipped` straight into the freshly submitted `EtymologyOriginResult`. The shared `QuizResultCard` reads `ResultItem.isSkipped` as "excluded from future quizzes" (rendering an "Excluded" badge + "Resume" button), a meaning that field only otherwise carries via the deliberate `SkipWord` flow. So tapping "Don't Know" displayed the origin as **Excluded** even though the YAML never set `skipped_at`. **U1 + U2 violation** (field collision between skip and exclude). Fix: make "Don't Know" a per-word `skipped` field that grades each word independently, never populates the shared `isSkipped` display flag, and records a normal `misunderstood` attempt — excluding an origin stays a distinct `SkipWord` action that path never touches.

## Worked example — grammar-relearn skip removed; unanswered is incorrect, Exclude is deliberate (this PR, #38)

An earlier iteration of the grammar Relearn post (`RelearnGrammarPost.tsx`) modelled a not-answered blank as a **skip / "Don't know"**: the whole-post "See answers" graded every remaining blank as skipped and a per-blank "Don't know" existed. Users found this wrong — an unanswered blank should be a *miss*, not a pass — and there was no way to exclude an individual correction. The final model removes skip entirely:

- "See answers" now grades every unanswered (and non-excluded) blank **incorrect** — an empty answer with `is_skipped=false`, graded wrong deterministically by `GradeGrammarBlank` (an empty answer can never be the correction, so no LLM call). The correction stays `misunderstood` in the learning history (Relearn persists nothing) and therefore stays **due**. Confirmed against real on-disk YAML: after an unanswered/incorrect blank the learning-history file is byte-identical (no `skipped_at`) and the correction reappears in the pool on the next `StartRelearnQuiz`.
- A per-blank **Exclude** button is the only exclusion. It calls the same `SkipWord` RPC every other card uses, resolved from the relearn store to the correction's `(notebookID, senseID)` flat "journal" bucket. Confirmed against real on-disk YAML: after Exclude the file HAS `skipped_at` for that correction and it no longer appears in the pool. The grammar loader's due-check (`grammarMistakeDue`) now filters `skipped_at`, which is what makes Exclude effective for both the live grammar quiz and Relearn.

The contrast is the whole point: **not answering ⇒ incorrect + still due (no marker); Exclude ⇒ `skipped_at` set + gone.** A wrong answer must never write the exclude marker.

## Worked example — grammar-relearn feedback below the Next button (this PR, #38)

The same post once rendered its per-blank feedback (`GrammarCorrectionBody`) at the very bottom of the component, **below the "See answers" / "Next" button**. On a long journal post the answer/explanation was off-screen right when the learner needed it. **U3 violation.** Fix: the feedback opens automatically for a wrong / unanswered blank (never for a correct one) in a **pinned bottom sheet** (mirroring the live grammar quiz), scrolls the answered blank into view, and is keyed to the blank just graded — so the mistake / suggested / why is visible without hunting at the bottom of the post, and Next no longer sits above the feedback it follows. The pinned sheet also carries an **Exclude from quizzes** button for the selected blank.

## Worked example — grammar-relearn horizontal overflow (this PR, #38)

The progressive-post rework wrapped each ungraded blank's struck word + inline textbox + control in a `whiteSpace="nowrap"` span so they stayed on one line — but a long post or a long unbroken token then forced the whole post to scroll horizontally on a phone. **UI-wrapping expectation:** the quiz surfaces must wrap within the viewport and never scroll horizontally on long content or a long single token. Fix: dropped `nowrap`; the post box uses `whiteSpace="pre-wrap"` + `overflowWrap="anywhere"` + `wordBreak="break-word"` + `maxW="100%"`, and each ungraded blank is an `inline-flex` group with `flexWrap="wrap"` so the word/input/Exclude wrap together instead of overflowing. Long tokens break; inputs shrink to fit.
