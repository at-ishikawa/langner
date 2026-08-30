# Quiz UI invariants

These invariants govern the **quiz-facing UI and its request/grade contract** for every quiz mode — vocabulary (flashcard / story / definitions), grammar (live quiz + grammar Relearn), the Relearn origin family card, and any future mode. They are the surface-layer complement to [[learning-history-invariants]] (which governs how a learning log is written, read, and displayed). Where a learning record is touched, both documents apply.

There is **no standalone etymology-origin quiz**. Etymology words are ordinary definition entries, so they are quizzed through the normal vocabulary quizzes and their origin shows in vocabulary feedback. The origin family card (an origin + its meaning + the words that derive from it) now lives **only in Relearn**: when origin-bearing words are missed as vocabulary, they are grouped by origin into one family card.

Check these before touching any of:

- `frontend/src/components/RelearnGrammarPost.tsx`, `frontend/src/components/RelearnOriginPost.tsx`, `frontend/src/app/quiz/relearn/session/page.tsx` — the grammar Relearn post, the Relearn origin family card, and the single-card relearn screen
- `frontend/src/app/quiz/grammar/page.tsx`, `frontend/src/components/GrammarFeedbackCard.tsx`, `frontend/src/components/GrammarCorrectionBody.tsx`, `frontend/src/lib/grammarSegments.ts` — the live grammar quiz + shared grammar feedback
- `frontend/src/store/relearnStore.ts` (`groupIntoItems`) — how missed cards fold into grammar posts / origin family screens; `internal/quiz/relearn.go` (`primaryOriginPart`) — how a missed vocabulary word becomes an origin family card
- `frontend/src/components/QuizResultCard.tsx`, `frontend/src/components/FeedbackActions.tsx`, `frontend/src/components/AnswerInput.tsx` — the shared answer/feedback surfaces and the flags they read
- backend: `internal/server/quiz_handler_grammar.go`, `internal/server/quiz_handler_relearn.go`, `internal/server/quiz_handler_batch.go` (`skippedGradeResult`), `internal/quiz/word_actions.go` (`SkipWord` / `ResumeWord`), `internal/notebook/learning_history_updater.go` (`SetSkippedAt` / `ClearSkippedAt`)

---

## U1 — "Don't know" / skip is NOT "exclude from quizzes"

A per-item **"Don't know" / skip** means *"I can't answer this right now."* The item stays **due** and remains eligible for every future session, **including Relearn**. It is recorded as a normal not-yet-learned grade (a `misunderstood` / lowest-quality attempt) on the item's canonical storage key — the same key any other attempt uses (see [[learning-history-invariants]] L1–L4) — or, in Relearn, as nothing at all (Relearn persists no state).

A skip MUST NOT write the **exclude-from-quizzes marker**. In this codebase that marker is the per-quiz-type `skipped_at` field on a learning-history expression (`SkippedAtMap`), set only through the deliberate `SkipWord` RPC (`quiz.Service.SkipWord` → `LearningHistoryUpdater.SetSkippedAt`) and cleared by `ResumeWord` / `ClearSkippedAt`. Card loaders filter out any expression whose `skipped_at` is set, so writing it **removes the word from all future quizzes and from the Relearn pool** — the opposite of what "Don't know" should do.

**Consequence:** "Exclude from quizzes" is a separate, deliberate, separately-labeled user action, offered **only in the normal quizzes** (the **Exclude / Resume** button, e.g. `GrammarFeedbackCard` / the vocabulary result cards). It is the ONLY thing that writes `skipped_at`. Never reach it from a "Don't know" / skip, from a normal wrong answer, or from *not* answering. A skipped or incorrect item must never be labeled **"Excluded"** in the UI — that word is reserved for `skipped_at`-excluded items. **Relearn has no Exclude control at all** (see the Relearn section below).

### Relearn has NO skip and NO exclude — it persists nothing

Relearn re-drills recently-missed items and **persists no state at all**. Neither of its progressive posts — the grammar Relearn post (`RelearnGrammarPost.tsx`) and the origin family card (`RelearnOriginPost.tsx`) — has a skip / "Don't know" control **or** an Exclude control. A blank / word is exactly one of two states:

- **unanswered** → on "See answers" it is graded **incorrect** (a normal miss: empty answer, `is_skipped=false`, graded wrong deterministically — for a grammar blank an empty answer can never be the correction, so no LLM call). It stays **due** (Relearn persists nothing), and it MUST NOT be labeled or recorded as skipped and MUST NOT set `skipped_at`.
- **answered** → correct / incorrect from the grader (the same recognition grader every vocabulary card uses — the origin/post is presentation only).

There is no third "Excluded" state on these posts: **nothing in Relearn calls `SkipWord` / `ResumeWord`.** Excluding a word from future quizzes is a deliberate action reserved for the **normal quizzes** (the live grammar quiz's `GrammarFeedbackCard`, the vocabulary result cards). Because a Relearn miss writes nothing, a missed item simply reappears due next session. **Within** a session it is also re-drilled: every progressive post — the grammar Relearn post (wrong blanks re-queue via `completeGrammarPost`) and the origin family card (wrong words re-queue via `completeOrigin`), like the single-card screen (`resolveFront`) — folds the items answered wrong this pass into a smaller post at the back of the queue and re-asks them until every one is answered correctly. This is **queue-only** (local React state, no RPC, no `skipped_at`), so it changes nothing about persistence: an item not answered correctly is still due next session too.

The single-card vocab/reverse Relearn screen (`FeedbackActions` inside `quiz/relearn/session/page.tsx`) also has no Exclude: it passes `showExclude={false}`. `FeedbackActions` keeps its Exclude control (default `showExclude=true`) for the normal single-card quizzes; only Relearn opts out. The live grammar quiz and this single-card Relearn screen still have their own "Don't know" skip, which U1/U2 continue to govern; the two progressive Relearn posts have **no** skip — an unanswered item is a normal miss.

### Relearn's in-session Mark-as-Correct/Incorrect override is allowed — it persists nothing

"Relearn persists nothing" is about the **backend/learning-history** (no log, no `skipped_at`, no RPC). It does **not** forbid an **in-session override** that only reshapes the working queue for the current session. Both the single-card Relearn screen (`session/page.tsx`, `const effective = override ?? feedback.correct; resolveFront(effective)`) and the origin family card (`RelearnOriginPost.tsx`, a per-word `overrides` map feeding `effectiveCorrect`) let the learner flip a grader verdict — e.g. mark a wrongly-✓ word as ✗ so it re-drills this session (its wrong words re-queue via `completeOrigin`), or mark a ✗ word ✓ so it drops. This is **local React state only**: it calls no RPC, sets no `is_skipped`, and writes no `skipped_at`. It is NOT skip and NOT exclude (U1/U2 untouched) — a re-drilled word is simply still due next session like any other Relearn miss. Do not "remove it as a violation": it is the intended parity between the origin card and the single-card screen.

### Relearn origin card shows a POST-ANSWER, display-only "related words" reference

The origin family card (`RelearnOriginPost.tsx`) shows, **only after the card is answered** (`remainingCount === 0`), a "Related words from this origin" section: the OTHER words sharing this origin, each as **word + short meaning**. It is pure reference — **not quizzed, no input, no grading, no RPC, no persistence** — so U1/U2 are untouched. Rules that must hold:

- **Hidden during the question state**, for BOTH recognition and reverse. Never render it while any word on the card is still being asked — in reverse a same-root sibling could hint the answer.
- **Excludes the drilled words** (they are the quiz items on this card). Deduped.
- **Does NOT filter `skipped_at`-excluded words.** This is display-only *reference*, not a quiz pool. A word the learner excluded from quizzes (they learned it and turned quizzing off) is exactly a known same-origin word worth showing — so it MUST still appear here. `skipped_at` filtering belongs ONLY to the quiz/drilled pool (excluded words are never *quizzed*), a separate code path (the card loaders / the `consider` step of `LoadRelearnPool`). The family builder must not reach for it.
- Sourced from the SAME origin resolution the card already uses: the backend groups every vocabulary word by its `primaryOriginPart` (`buildRelearnOriginFamilies` in `internal/quiz/relearn.go`), reusing the one folding rule (L2) — it does NOT fork a second origin lookup. Carried on `RelearnCard.related_words` (`OriginFamilyMember{word, meaning}`) and threaded through `relearnStore` (`RelearnOriginGroup.relatedWords`).

Do not move it into the question state, do not drop the drilled-word exclusion, and do not re-add a `skipped_at` filter here.

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

*The historical PR-by-PR worked examples (the #37/#38/#41 iterations that removed the standalone etymology-origin quiz, removed skip/Exclude from Relearn, moved feedback into a pinned sheet, fixed horizontal overflow, and added in-session re-drill of wrong grammar blanks) previously lived here. They have been dropped as changelog — that narrative belongs in git history. U1–U3 above are the current contract; when they change, update them here rather than appending another PR note.*
