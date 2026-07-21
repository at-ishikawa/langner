---
title: "Technical Design"
weight: 2
---

# Technical Design

## Overview

Introduce a stable `id` on every vocabulary entry and make it the canonical key for a word's identity. The lookup rule lives in **one shared helper** used by every writer and reader (per invariant L2): resolve by `id` when present, else fall back to today's expression match (legacy), and upgrade a matched legacy entry to the id on first write. The database note identity switches from `(usage, entry)` to the id.

## The `id`

- A short, stable, **globally-unique** string on each source entry, e.g.
  ```yaml
  cards:
    - id: bank-river
      expression: bank
      meaning: the land alongside a river
    - id: bank-money
      expression: bank
      meaning: a financial institution
  ```
- **Format (decision point — see "Open decisions"):** default is a readable slug of the expression, de-duplicated to stay globally unique (`bank`, `bank-2`, …); the tool guarantees uniqueness and the author may rename to something meaningful (`bank-river`).
- Carried onto `Note`, the quiz `Card`/`ReverseCard`/`FreeformCard`, `NoteRecord`, and `LearningHistoryExpression`.

## Source model

- Add `id string yaml:"id,omitempty"` to the parsed vocabulary `Note` (`internal/notebook/notebook.go`) and to flashcard/definition entry structs.
- `convertNoteToRecord` and the card loaders copy `id` through (today they drop even `part_of_speech`; the id must not be lost the same way).
- **`YAMLNoteRepository.FindAll` dedups by `(usage, entry)` — this itself collapses same-spelling senses.** Change the dedup key to `id` (falling back to `(usage, entry)` only for id-less legacy entries), so two ids produce two records.

## Learning-history model

- `LearningHistoryExpression` gains `id string yaml:"id,omitempty"`.
- Shared helper:
  ```go
  // matchesEntry reports whether a learning entry is the one for a card.
  // Prefer id equality; fall back to the expression match for legacy
  // (id-less) entries so pre-migration data still resolves.
  func matchesEntry(e *LearningHistoryExpression, id, expression, original string) bool
  ```
  Used by the updater, `GetLogs`/`GetReverseLogs`, `IsExpressionSkipped`, `FindExpression*`, `OverrideLog`/`Undo`.
- **Write:** `SaveResult*` route by the card's `id`. On match, if the found entry is legacy (no id) and the card has an id, stamp the id in place (upgrade-in-place — single-sense words never fork; a duplicate's commingled legacy history attaches to the first-answered sense, the other starts fresh).
- **Read:** `GetLatestLearnedInfo(notebook, id, expression, quizType)` and the relearn/analytics indexes key by id, expression as legacy fallback.

## Quiz / relearn / analytics / override

Mirrors the surfaces threaded before, but the discriminator is `id`:

- `Card`/`ReverseCard`/`FreeformCard` carry `ID`; `SaveResult`/`SaveReverseResult`/`SaveFreeformResult` set `LearningLog.ID`.
- Relearn: candidate key and vocab index key by `id` (id-less → expression fallback). The candidate carries the failed entry's id.
- Analytics: series keyed by `id`; the meaning resolver resolves by id.
- Override RPC: `id` added to the SubmitAnswer/override/word-history proto messages and threaded through `CardInfo` → `learning.UpdateLogInput` and the frontend, so Mark-as-Correct / Undo target the right entry.

## PostgreSQL identity

- Migration: `ALTER TABLE notes ADD COLUMN sense_id VARCHAR(128)`; backfill from import; then `UNIQUE(sense_id)` becomes the identity (drop `UNIQUE(usage, entry)`), with `sense_id` `NOT NULL` once populated.
- `ensureNoteExists` / `UpdateLog` resolve the note by `sense_id` (from the card's id). `learning_logs.note_id` FK is unchanged; the note is found via `sense_id`.
- `datasync` import/export round-trips `id`.
- Legacy rows with no `sense_id` keep resolving by `(usage, entry)` during the transition.

## Migration

`langner migrate assign-ids`:

1. Walk every source vocabulary entry; assign a globally-unique `id` where absent (readable slug, de-duplicated), and **write it into the source YAML** (render-to-temp + `os.WriteFile`, cloud-mount safe). Idempotent: entries that already have an id are left alone. Diff is add-only (`id:` lines).
2. Best-effort re-key of existing learning history: for each id-less learning entry, if exactly one source entry in that notebook matches its expression, stamp that entry's id; ambiguous duplicates are left id-less (legacy) — split on next answer. No log is ever moved.
3. `validate` gains a check that every source entry has a unique id (globally) once the migration is adopted.

## Back-compat & rollout

- Before `assign-ids`, nothing has an id → every lookup uses the expression fallback → behavior is identical to today.
- After: new writes carry ids; reads prefer id; legacy entries upgrade in place. Fully incremental, no history loss.

## Testing

- Updater: two entries, same expression+part-of-speech, different id → two independent series; legacy id-less entry upgrades in place on first id write.
- `FindAll` dedups by id (two ids → two records; same id merges NotebookNotes).
- Read symmetry: `GetLatestLearnedInfo` by id returns the series `SaveResult` wrote.
- Relearn/analytics: two ids resolve to their own meaning/series.
- Override targets the right entry by id.
- DB: `ensureNoteExists` creates two rows for two ids; `UNIQUE(sense_id)` permits the pair.
- Migration: `assign-ids` is add-only and idempotent; single-match learning entries get the id; duplicates left legacy.
- Back-compat: id-less YAML loads, reads, and (for non-duplicates) accrues on the same entry after migration.

## Open decisions (confirm before build)

1. **`id` format** — readable slug + numeric de-dup (`bank`, `bank-2`), or opaque short token (`b7f3k2`)? (Recommend readable slug.)
2. **Source rewrite scope** — assigning ids to *every* entry rewrites ~all source notebook files (add-only). Confirm that's intended (you chose "every single entry").
3. **`part_of_speech`** — drop entirely, or keep as a non-identity display field on the card?
