---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

When a notebook defines two vocabulary entries with the **same spelling but a different sense** — most commonly the same word used as two parts of speech (a noun and a verb, an adjective and a verb) — the app stores them as two notes in the source but collapses them into a **single learning-log series**. Their success/failure logs, spaced-repetition (SM-2) schedule, status, streaks, and analytics are commingled and cannot be told apart, and the Relearn quiz can show a different meaning than the one the learner was tested on.

This proposal adds a **sense discriminator** (`part_of_speech`) to the learning key so each sense carries its own independent series across every surface.

## Problem

Using the generic homograph `record`:

- entry A — `part_of_speech: noun`, meaning "a piece of evidence / an account"
- entry B — `part_of_speech: verb`, meaning "to set down in writing"

1. Answering `record` in the standard quiz shows **entry A's** meaning (whichever card was scheduled), and the log is appended to a single `record` series.
2. Relearning the same word shows **entry B's** meaning — the Relearn index (`relearnVocabIndex`) is a last-write-wins map keyed by expression, so it resolves to whichever sense loaded last (file order), not the sense that was failed.
3. Inspecting the learning history shows only **one** `expression: record` series; both attempts are in it.

### Why it happens

The learning record has no *sense* discriminator:

- `LearningHistoryExpression` keys on `Expression` + `Type`, where `Type` only separates **vocabulary vs. etymology-origin** entries. It does not separate two vocabulary senses.
- The write path `UpdateOrCreateExpressionWithQuality*` matches purely by name, so both senses find the same entry and append to it.
- Read/index surfaces (`GetLatestLearnedInfo`, `relearnVocabIndex`, the analytics resolver) key by expression alone, so they collide too.

This violates invariant **L3 (display = storage = lookup)** and **L4 (one log series per sense)**: the stored key doesn't carry the sense, so a displayed meaning can mismatch the tested one, and the two series can never be separated.

## Goals

- Two same-spelling notes with different `part_of_speech` produce **two independent learning-log series** — separate logs, SM-2 schedule, status badge, streaks, and analytics.
- The Relearn quiz shows the **sense that was actually failed**, not whichever loaded last.
- The sense discriminator lives in **one shared canonicalization helper** called from both the write side and every read/lookup side (per invariant L2), so the rule can never drift.
- Existing data is preserved: non-homograph words keep full continuity; genuinely merged homograph history is left intact under a legacy key rather than guessed apart.
- The same discriminator is applied to the PostgreSQL note identity, so the DB shadow is correct and ready for the planned "DB as source of truth" migration — no second homograph fix later.

## Non-Goals

- **Completing the YAML→DB read migration.** Every user-visible read (quiz cards, learning history, relearn, analytics) reads YAML today; the DB `learning_logs`/`notes` tables are a write-only shadow. Repointing reads to the DB is a separate, larger effort. This proposal fixes the bug in the authoritative YAML path *and* makes the DB identity correct, but does not flip authority.
- **Etymology origins.** The vocab-vs-origin collision is already handled by `Type`, and multi-sense origins are disambiguated by scene title (and a DB `sense` column). Out of scope here.
- **Splitting already-merged homograph history by guessing.** Logs never recorded which sense was displayed, so a merged series is left as a legacy entry; only new writes split.
- **Changing the SM-2 algorithm, the grading, or the quiz UX** beyond showing the correct per-sense meaning.

## Success Criteria

- Answering each sense of a two-sense word in the standard, reverse, and freeform quizzes produces two distinct series in the learning-history YAML, each carrying its own `part_of_speech`.
- The Analytics Day Detail / Word History pages list the two senses separately with the correct meaning each.
- A word failed in sense A shows sense A's meaning in the Relearn quiz.
- The "Mark as Correct" override targets the correct sense's log.
- A word that is **not** a homograph behaves exactly as before, with no history loss.
- The PostgreSQL `notes` table can hold two rows for the two senses (`UNIQUE (usage, entry, part_of_speech)`), and a quiz answer resolves to the right row.
