---
title: "Technical Design"
weight: 2
---

# Technical Design

## Overview

Add `part_of_speech` as a **sense discriminator** on the learning key. The canonical key for a vocabulary series becomes:

```
(notebook, scene, expression, Type, part_of_speech)
```

where `Type` still separates vocabulary from etymology-origin, and `part_of_speech` (normalized: trimmed + lowercased) separates two vocabulary senses. Legacy entries with no `part_of_speech` match the empty sense, so pre-change data stays readable.

The rule that derives the sense from a card or note lives in **one shared helper** called from both the write side (`SaveResult` → updater) and every read/lookup side (`GetLatestLearnedInfo`, the relearn index, the analytics resolver, skip/override), satisfying invariant **L2**. The same discriminator is added to the PostgreSQL `notes` identity so the DB shadow is correct.

## Data model

### `LearningHistoryExpression` (`internal/notebook/learning_history.go`)

Add one field, mirroring the existing `Type` discriminator:

```go
type LearningHistoryExpression struct {
    Expression string `yaml:"expression"`
    Type       string `yaml:"type,omitempty"`
    // PartOfSpeech distinguishes two vocabulary entries that share a
    // spelling but a different sense (e.g. "record" the noun vs the
    // verb). Empty means "unspecified sense" — the legacy value that
    // pre-discriminator entries and single-sense words carry. Combined
    // with Expression + Type it forms the canonical series key.
    PartOfSpeech string `yaml:"part_of_speech,omitempty"`
    ...
}
```

Resulting YAML for a homograph:

```yaml
expressions:
  - expression: record
    part_of_speech: noun
    learned_logs: [...]
  - expression: record
    part_of_speech: verb
    learned_logs: [...]
```

### The shared canonicalization helper

A single matcher, used by every writer and reader:

```go
// normalizePartOfSpeech is the canonical sense token: trimmed + lowercased.
func normalizePartOfSpeech(pos string) string {
    return strings.ToLower(strings.TrimSpace(pos))
}

// MatchesSense reports whether an entry belongs to the given sense.
// An empty query matches an empty (legacy/unspecified) entry only, so a
// sense-tagged write never lands on the legacy commingled entry and a
// legacy read never picks up a sense-tagged entry.
func MatchesSense(expr *LearningHistoryExpression, partOfSpeech string) bool {
    return normalizePartOfSpeech(expr.PartOfSpeech) == normalizePartOfSpeech(partOfSpeech)
}
```

Matching an entry now means: name matches (`Expression`/`OriginalExpression`) **and** `MatchesExpressionType(expr, type)` **and** `MatchesSense(expr, pos)`. This composite is the only place the rule lives.

## Write path

`part_of_speech` is already carried in memory on the quiz `Card`/`WordDetail` (loaded from the source note by `notebook.Reader`). Thread it to the store:

1. **`quiz.Card` / `ReverseCard` / `FreeformCard`** — ensure each exposes the note's `PartOfSpeech` (add an explicit field where it isn't already reachable; `WordDetail.PartOfSpeech` is populated today).
2. **`learning.LearningLog`** (`internal/learning/repository.go`) — add `PartOfSpeech string`.
3. **`SaveResult` / `SaveReverseResult` / `SaveFreeformResult`** (`internal/quiz/service.go`) — set `log.PartOfSpeech` from the card. For a concept card (`card.ConceptHead != ""`) the series keys on the head's sense, consistent with L1 folding members under the head.
4. **YAML repository** (`internal/learning/yaml_repository.go`) — pass `partOfSpeech` into `UpdateOrCreateExpressionWithQuality*`.
5. **`UpdateOrCreateExpressionWithQuality*` and `createNewExpressionWithQuality*`** (`internal/notebook/learning_history_updater.go`) — add `partOfSpeech` to the match predicate and stamp it on newly created entries.

## Read / lookup path (L2 — same key both sides)

Every reader adds the sense to its lookup, via the shared matcher:

- **`GetLatestLearnedInfo(notebookName, expression, partOfSpeech, quizType)`** (`internal/quiz/service.go`) — the response builders in `quiz_handler.go` / `quiz_handler_batch.go` have the card, so they pass its sense. This is the surface behind the "Mark as Correct" button gate; a matching key means `learned_at` is populated for the right sense.
- **`GetLogs` / `GetReverseLogs` / `IsExpressionSkipped` / `FindExpressionByName` / `FindExpressionByAnyName`** (`learning_history.go`, `learning_history_updater.go`) — sense-aware variants; callers that legitimately lack a sense (bulk maintenance) pass `""` and match legacy entries.
- **Relearn index** (`internal/quiz/relearn.go`) — the candidate key already includes `(format, notebook, expression)`; add the entry's `part_of_speech`. `relearnVocabIndex` changes from `byExpr[expr]` (last-write-wins) to a sense-keyed map `byNotebookExprSense[notebook + expr + sense]`, so the failed sense resolves to its own meaning/context.
- **Analytics** (`internal/analytics/yaml_repository.go`, `notebook_resolver.go`) — the per-word key (`wordKey`) and the meaning resolver include the sense, so Day Detail / Word History list the two senses independently.

## Override RPC / API surface (L3 at the boundary)

The "Mark as Correct" override and its undo identify a log by `(expression, quiz_type, learned_at)`. To target the right sense of a homograph, the API must carry `part_of_speech`:

- Add `part_of_speech` to the relevant proto messages (the SubmitAnswer/batch response items and the override request), regenerate with `buf`.
- `OverrideLogInput` / `UndoOverrideLogInput` (`learning_history_updater.go`) gain a `PartOfSpeech` field, threaded into `FindExpressionByAnyName`.
- Frontend (`QuizResultCard.tsx`, `FeedbackActions.tsx`, `WrongWordCard.tsx`) passes the sense back on override, alongside the `noteId`/`learnedAt` it already sends.

For the DB store (`UpdateLog` in `internal/learning/repository.go`), the note lookup `SELECT id FROM notes WHERE "usage"=$1 AND entry=$2` gains `AND part_of_speech=$3`.

## Validation

Duplicate-detection currently treats two same-spelling entries as an error and offers to merge them (`Validate` in `learning_history.go`: the flashcard duplicate check keyed on expression, and the across-scenes check keyed on `(name, type)`). Add `part_of_speech` to both keys so a legitimate homograph is **not** flagged, and the `validate --fix` merge never fuses two senses.

## PostgreSQL identity

Make the DB note identity sense-aware so it is ready to become authoritative:

- **Migration `019_add_notes_part_of_speech`**:
  ```sql
  ALTER TABLE notes ADD COLUMN part_of_speech VARCHAR(50) NOT NULL DEFAULT '';
  ALTER TABLE notes DROP CONSTRAINT notes_usage_entry_key;      -- UNIQUE (usage, entry)
  ALTER TABLE notes ADD CONSTRAINT notes_usage_entry_pos_key
      UNIQUE ("usage", entry, part_of_speech);
  ```
  Existing rows default to `''`; because homographs were previously merged, no duplicate violates the new constraint. `NOT NULL DEFAULT ''` (not NULL) keeps the unique constraint meaningful in Postgres.
- **`NoteRecord.PartOfSpeech`** loses its `db:"-"` tag and becomes a persisted column.
- **`ensureNoteExists`** (`internal/learning/repository.go`) includes `part_of_speech` in both the upsert lookup and the insert.
- The `datasync` import/export (`langner migrate import-db` / `export-db`) already round-trips notes; it carries the new column so a DB→YAML export stays faithful (supporting the "keep YAML as an export format" direction).

## Migration of existing YAML history

Safe, best-effort — never guesses on a genuine homograph:

- A one-shot pass (a `validate --fix`-style maintenance command) over each learning-history entry: resolve the source notes for that expression in that notebook.
  - **Exactly one matching note** → stamp the entry's `part_of_speech` from it. Non-homographs (the overwhelming majority) get full continuity and key identically to future writes.
  - **Multiple matching notes with different senses** (a real homograph) → leave `part_of_speech` empty. The commingled history stays as a legacy entry; new sense-tagged answers create fresh per-sense series going forward.
- No log is ever reassigned between senses, so no history is corrupted.

## Testing

- **Updater**: two same-spelling notes (noun/verb) answered in standard/reverse/freeform produce two independent `LearningHistoryExpression` entries, each with its `part_of_speech`; a single-sense word still updates one entry.
- **Symmetry (L2)**: `GetLatestLearnedInfo` returns the series that `SaveResult` just wrote for the matching sense, and empty for the other sense.
- **Relearn**: a word failed in sense A yields a card showing sense A's meaning; the two senses do not collide in the index.
- **Analytics**: Day Detail lists both senses separately with correct meanings.
- **Validation**: a homograph is not reported as a duplicate; `--fix` does not merge senses.
- **Migration**: single-note expression is tagged; homograph expression is left legacy; no logs move.
- **DB**: `ensureNoteExists` creates two rows for two senses and resolves an answer to the right `note_id`; the unique constraint permits the pair.
- **Back-compat**: legacy YAML with no `part_of_speech` loads, reads, and (for non-homographs) continues to accrue on the same entry after migration.
