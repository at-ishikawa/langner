# Learning history invariants

These four rules govern every code path that **writes**, **reads**, or **displays** a quiz learning record (a word's success/failure log under a given quiz mode). Apply them to vocabulary notebooks (story, flashcard, definitions), etymology origins, and any future quiz mode.

When making changes, check the rules before touching any of:

- `internal/quiz/service.go` — `SaveResult`, `SaveReverseResult`, `SaveFreeformResult`, `SaveEtymologyOriginResult`
- `internal/server/quiz_handler.go`, `internal/server/quiz_handler_batch.go` — the response builders that call `GetLatestLearnedInfo`
- `internal/notebook/learning_history_updater.go` — `UpdateOrCreateExpressionWithQuality*`, `FindExpressionByName`, `OverrideLog`
- `internal/learning/yaml_repository.go`, `internal/learning/repository.go`, `internal/learning/multi_repository.go` — the write path
- `internal/analytics/yaml_repository.go`, `internal/analytics/notebook_resolver.go` — the read path used by the Analytics page
- `frontend/src/components/QuizResultCard.tsx`, `frontend/src/components/FeedbackActions.tsx`, `frontend/src/components/WrongWordCard.tsx` — the surfaces that gate buttons on `noteId` / `learnedAt`

## L1 — One canonical storage key per learning log

A single quiz attempt is persisted under exactly **one** storage key. Never write the same attempt twice — not once under the concept head and once under each member, not once under the etymology origin and once under each definition that references it, not once per notebook the word lives in.

**Consequence:** when a word belongs to multiple containers (e.g. the same flashcard appears in two notebooks), pick one canonical container at write time and only write there.

## L2 — Symmetric read and write

Any code that reads a learning log MUST use the same key the writer used. If the canonical key depends on a rule (e.g. "family concept members fold under the head; other kinds stay per-member"), that rule lives in **one** function called from both sides — `SaveResult` and `GetLatestLearnedInfo` (or its equivalent) MUST NOT each open-code the rule.

**Consequence:** changing the canonicalization rule is a one-file change. If two sides drift, the response carries an empty `learned_at` (or the analytics card shows blank, or the override RPC no-ops silently) — the symptom always traces back to L2.

## L3 — Display = storage = lookup

The expression text the user sees on a quiz card / Learn page row / analytics card IS the key the system uses to write that word's logs and to look them up afterwards. No silent renaming between the surface and the persistence layer.

**Consequence:** if a card displays `cardiogram`, the YAML log for that attempt lives under `Expression: cardiogram`. If a concept is consolidated under the head `cardiograph`, the card must display `cardiograph` (with member chips), not display `cardiogram` while storing under `cardiograph`.

## L4 — Cross-notebook / cross-context consistency

A word has exactly one log series **per quiz mode**, regardless of how many notebooks, concepts, etymology origins, or definitions reference it. Concept membership / etymology origin reference / definition alias / notebook membership are metadata that *select* the right series — they never create a parallel series.

**Consequence:** if `cardiogram` appears in two notebooks, both notebooks display the same status badge, the same next-review date, and the same Day Detail entry. There is no notebook-A version vs notebook-B version of `cardiogram`.

---

## Worked example — the "Mark as Correct" button regression

The BatchFeedback view gates the **Mark as Correct** override button on `item.noteId && item.learnedAt`. Both come from the SubmitAnswer response. When `learnedAt` arrives empty, the button hides.

Trace the failure backwards using the rules:

1. Frontend gate failed → backend returned empty `learnedAt`.
2. Backend response built `learnedAt` via `GetLatestLearnedInfo(notebookName, card.Entry, quizType)`.
3. The just-written log SaveResult produced was indexed by a different key than `card.Entry`. **This is an L2 violation.**

If the canonicalization rule (e.g. "fold to concept head when kind is family") lived in **one** function called from both `SaveResult` and `GetLatestLearnedInfo`, the divergence couldn't happen. Fix path: move the rule into a shared helper that returns the canonical storage key for a card; both sides call it. Any future quiz mode or canonicalization rule then adds itself in one place.

## Worked example — analytics card showing the wrong expression

The Analytics Day Detail page displays the expression from the learning log. If a card displays `cardiograph` (the head) but the user remembers answering `cardiogram` (the member), the user is confused. **L3 violation.**

Fix path: the card displays whatever key the log was stored under. If that key is the head, every other surface that mentions this word also says "head". Don't mix.

## Worked example — duplicate logs across notebooks

If the user has `cardiogram` in both `word-power-made-easy.yml` and `medical-terms.yml`, and a quiz attempt writes to both files, **L1 + L4 violations**.

Fix path: at write time, pick one canonical notebook (e.g. the source the card was loaded from) and only write there. At read time, every surface that asks "what's the status of cardiogram?" consults the canonical entry — not whichever notebook happens to be loaded first.
