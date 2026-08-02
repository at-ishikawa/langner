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

**Consequence:** "Exclude from quizzes" is a separate, deliberate, separately-labeled user action (the **Exclude / Resume** footer button, e.g. `FeedbackActions` / `GrammarFeedbackCard`). Never reach it from a "Don't know" / skip. A skipped item must never be labeled **"Excluded"** in the UI — that word is reserved for `skipped_at`-excluded items.

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

## Worked example — grammar-relearn "Don't know" excluding words (this PR, #38)

The grammar Relearn post (`RelearnGrammarPost.tsx`) had **no per-blank "Don't know"** — the only pass control was the whole-post "See answers", which graded every remaining blank as skipped and the detail panel then rendered the header **"– Excluded"** (and the shared `PILL_STYLES.skipped.label` reads `"excluded"`). Root cause: a **pure display/label collision** — `SubmitRelearnAnswer` writes nothing (Relearn persists no state), so a skipped blank was never actually excluded (`skipped_at` stayed unset and the correction stayed `misunderstood`, hence still due), but the UI told the learner it was "Excluded." **U1 violation** (skip labeled as exclude). Fix: added a per-blank **"Don't know"** control (skips just that blank, stays due), relabeled a skipped blank **"Don't know · still due"** / pill "don't know" (never "Excluded"), and confirmed live that the learning-history YAML is byte-identical after a skip (no `skipped_at`) and the correction reappears in the pool on the next `StartRelearnQuiz`.

## Worked example — grammar-relearn feedback below the Next button (this PR, #38)

The same post rendered its per-blank feedback (`GrammarCorrectionBody`) at the very bottom of the component, **below the "See answers" / "Next" button**. On a long journal post the answer/explanation was off-screen right when the learner needed it, and it only appeared when a graded pill was tapped. **U3 violation.** Fix: the feedback now opens automatically for a wrong or "Don't know" blank (never for a correct one) in a **pinned bottom sheet** (mirroring the live grammar quiz), scrolls the answered blank into view, and is keyed to the blank just graded — so the mistake / suggested / why is visible without hunting at the bottom of the post, and Next no longer sits above the feedback it follows.
