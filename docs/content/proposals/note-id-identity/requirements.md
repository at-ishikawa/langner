---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

A word's learning history (success/failure logs, SRS schedule, status, streaks, analytics) is currently keyed by its **expression string** (per notebook). Any two source entries that share a spelling collapse into one series. This proposal replaces string identity with a stable, opaque **`id`** carried on every vocabulary entry — the single primary key for that entry's identity everywhere.

## Problem

Keying on the expression (even combined with part of speech) cannot separate entries that legitimately share those attributes:

- **Two parts of speech** — `record` (noun) vs `record` (verb).
- **Same part of speech, different meaning** — `bank` (noun, riverside) vs `bank` (noun, financial institution) in the same notebook. A part-of-speech discriminator does **not** split these.
- **Same spelling across notebooks** — already handled per-notebook today, but only by string.

Because the key is derived from mutable, non-unique attributes, there is no reliable way to keep two senses' histories apart, and any attribute-based scheme is either incomplete (part of speech) or fragile (meaning changes when edited, silently orphaning history).

## Goals

- Every vocabulary entry (flashcards, definitions books, story definitions) carries a stable **`id`**, **globally unique** across all notebooks, assigned to **every** entry.
- The `id` is the **sole identity** for the learning-log series and the database note row — independent of spelling, part of speech, and meaning.
- Two entries that differ only by meaning get **independent** series across every surface (quiz, relearn, analytics, override, SRS).
- Editing a meaning, wording, or part of speech **never** loses learning history (the id is unchanged).
- Existing data keeps working: entries without an `id` fall back to the current expression-based lookup, and migrate as they are answered — no history is lost on rollout.
- A migration tool assigns ids to all existing source entries and (best-effort) re-keys existing learning history to them.

## Non-Goals

- **Part of speech as identity.** It is dropped from the key. It may remain as display text, but it is not a discriminator.
- **Changing the SRS algorithm, grading, or quiz UX** beyond routing by id.
- **Merging a word's history across notebooks.** Global-unique ids mean the same spelling in two notebooks stays two series, matching today's per-notebook behavior.
- **Auto-splitting already-merged history for same-spelling duplicates.** Existing logs don't record which sense they belong to; ambiguous duplicates are left on a legacy (id-less) entry and split going forward.

## Success Criteria

- Two same-spelling, same-part-of-speech entries with different meanings in one notebook produce two independent learning-log series in YAML and two rows in Postgres.
- Answering, relearning, analytics, and Mark-as-Correct all operate on the correct entry by id.
- Editing an entry's meaning leaves its series intact.
- A notebook with no ids yet behaves exactly as today until `assign-ids` runs.
