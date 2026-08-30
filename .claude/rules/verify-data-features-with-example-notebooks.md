# Verify data features with example notebooks + a real-config integration test

Any change that **loads, writes, quizzes, groups, or displays notebook data** — vocabulary (story / flashcard / definitions), etymology origins, grammar, journals, Relearn, Analytics — or that adds/changes a **loader, quiz mode, or card/pool builder**, MUST ship two things alongside the code:

1. **Example notebooks in the real on-disk format**, under `examples/` (the dirs `config.example.yml` already wires: `stories_directories`, `definitions_directories`, `etymology_directories`, `grammars_directories`, `journals_directories`, `flashcards_directories`), that exercise the new feature's data conditions.
2. **An integration test that constructs the `Service`/`Reader` from `config.example.yml`** — the *same* construction the running server (`make dev` → `./langner-server`) uses — and drives the feature end-to-end, asserting the observable behavior.

This is the concrete, in-repo mechanism that makes [[no-unverified-main-merges]] satisfiable: it lets us exercise a change against realistic input **without the user's private notebooks**, which are never present in the dev environment.

## Why (the failure this prevents)

Hand-assembled fixtures — building a `RelearnCard`, a `FreeformCard`, an `originMap`, or a `Note` directly in a test — **bypass the real load path**. They construct one tidy object that every downstream path agrees on, so they cannot reveal a **divergence between two code paths** that build the same data differently (e.g. the normal quiz resolving a word's origin one way and Relearn resolving it another). The fixture passes; production fails. Loading the *same example notebooks* through the *real* config is the only thing that exercises the true reader/originMap/loader construction both paths actually run.

## What counts

- **Real format, not a shortcut.** The example YAML must be the shape the app parses (scenes, `origin_parts`, `examples`/`highlight`, `origins`, `forms`/`english_forms`, `definitions` blocks, concept `members`, …). If you invented a struct literal in the test instead of a file under `examples/`, it does not count.
- **Real construction, not a helper.** The test must go through `config.example.yml` → the same `Service`/`Reader` the server builds. A test that calls the pool/grade helper with a pre-built card does not count — that is exactly the fixture that lies.
- **Cover the awkward conditions**, not just the happy path. Include the data shapes that actually break things: a word whose first origin ref is undeclared while a later one is declared; a `from_form` that is an `english_form` not in `forms`; the same expression present in two notebooks (dedup); multi-origin words; homographs folded under a concept head; a word missed in reverse vs. recognition.
- **Assert observable behavior end-to-end** (the quiz shows the origin; the reverse miss folds into the `ETYMOLOGY_ORIGIN` family card under the right root with its example scene), not just that a field is non-nil.
- Example notebooks are **shippable repo content** (they double as demo/onboarding data) — keep them clean, generic, and documented. **Never** use the user's personal notebook identifiers or content (see the no-personal-data guidance); invent neutral, public examples (public Latin/Greek roots, invented sentences).

## Example notebooks are NOT enough — seed the learning-history STATE too

A fresh example notebook loaded through `config.example.yml` starts with a **clean** learning history: no logs, no skips, no legacy markers. Real user data is not clean — it carries accumulated STATE, and some of that state is written by features **that no longer exist**. A reproduction that only ships notebooks (and lets the test create every log itself) can never surface a bug that depends on such state, because a fresh test never produces it.

So the test must also **seed the realistic learning-history state**, through the same write path that produced it on disk:

- **Markers left behind by removed features.** The trigger bug here was a stale `skipped_at["etymology_origin"]` marker written by the etymology-origin quiz that #41 removed — no current code sets it, and no fresh test creates it, yet it gated Relearn origin-grouping into permanent plain cards. The regression test writes that exact marker via the real `SkipWord`/`SetSkippedAt` path, then asserts grouping is independent of it.
- **Legacy/pre-migration shapes**, prior skips/exclusions, id-less entries, and any per-quiz-type state a long-lived notebook can accumulate.

## Rules

- **When a feature is removed, add a test for the data it leaves behind.** Deleting the code that *writes* a marker/log/field does not delete that data from users' on-disk histories. Seed the leftover state (via the real write path, or on-disk YAML in the real shape) and pin how the surviving code must treat it — otherwise a vestigial marker silently changes behavior forever.

- Do not mark a data/quiz feature "done" or propose a merge until its example notebooks + real-config integration test exist and pass. A green suite of hand-built unit tests is **not** sufficient evidence for these features.
- When a bug is reported against real data you cannot see, the first move is to **reproduce it by adding example notebooks that match the reported shape and loading them through the real config** — then fix, then keep that reproduction as the regression test. Do not ask the user to keep re-running and pasting results in place of building a reproduction you can run yourself.
- If a feature genuinely cannot be exercised through `config.example.yml` (a Postgres-/browser-only path), say so explicitly and treat it as unverified — same as [[no-unverified-main-merges]].

## Worked example — the etymology-Relearn origin-grouping saga (the trigger for this rule)

A series of PRs added: example scenes in the Relearn origin card, etymology-notebook words made quizzable, and reverse/standard-vocab misses grouped by origin. Each was "verified" with **hand-built Go fixtures** (a `RelearnCard`/`FreeformCard` constructed in the test with origins already attached) and passed. But on the user's real notebooks, Relearn kept showing a plain "Reverse — recall the word" card with no origin grouping and no example statement — for word after word (`deficient`→`facere`, `transact`→`agere`). The user rebuilt, gave the exact YAML proving the origin was valid, and it still failed; meanwhile the assistant wrongly blamed the user's build and asked the user to re-verify things they'd stated from message one (that the **normal** quiz already shows the origin — so the bug was Relearn-specific all along).

Root cause: a **path divergence** — the normal quiz path resolved `WordDetail.OriginParts` while the Relearn path (`LoadRelearnPool` → `LoadAllWords` → `buildOriginMap`) produced empty origins for the same word. The hand-built fixtures could never surface this because they never ran either real construction. It was only caught by doing what this rule now requires: **add example definitions + etymology notebooks in the real shape, load them through `config.example.yml`, drive a reverse quiz + a reverse miss through the real `Service`, and assert the grouping** — which reproduced the bug immediately and then proved the fix. Had that example-notebook + real-config test been mandatory when the first PR was written, none of the churn (or the misplaced blame on the user) would have happened.
