---
title: "Design & Data Model"
weight: 2
---

# Design & Data Model

## Notebook format

A journal notebook is a new notebook *kind* alongside stories, flashcards, and etymology. It lives under a configured `journal_directories` and follows the same `index.yml` + content-file convention. The content is the entry prose; mistakes are span annotations inside it.

```yaml
# examples/journal/index.yml
id: journal
name: "English Journal"
notebooks:
  - ./2026-07.yml
```

```yaml
# examples/journal/2026-07.yml
- title: "July 2026"
  date: 2026-07-31T00:00:00Z
  entries:
    - id: 2026-07-05-party
      date: 2026-07-05T00:00:00Z
      text: |
        Yesterday the John invited me to a party. I was in school until five,
        so I arrived late. He suggested to play a board game, and it was fun.
      mistakes:
        - id: 2026-07-05-party-the-john   # stable id -> its own SR history
          incorrect: "the John"           # exact substring of text (locatable)
          correct: "John"
          category: article
          note: "No definite article before a personal name."
```

**Rules.** Each entry needs an `id` and `text`. Each mistake needs a unique `id`, an `incorrect` span that is an exact substring of the entry text (so the UI can highlight it), and a `correct` fix that differs from the incorrect span. `category` and `note` are optional. These are enforced by a validator.

## Quiz flow

- **Unit.** Each *mistake instance* is one card; spaced repetition is tracked per mistake `id`. Category is a tag for analytics, not the drill unit.
- **Correction mode.** The card shows the entry sentence with the incorrect span highlighted. The learner types the corrected phrase or the whole rewritten sentence.
- **Grading.** A dedicated `GradeCorrection` inference call decides whether the answer resolves the mistake (not an exact-string match — any grammatically correct fix passes), and returns an SM-2 quality score.
- **Scheduling.** Results are written to the notebook's learning history and gated by the existing `NeedsForwardReview` SM-2 logic, so a mistake answered correctly leaves the due set until its next interval and a missed one stays due.

## Learning-history shape

Grammar reuses the "flat" learning-history shape (top-level `expressions:` keyed by id, `metadata.type: grammar`) that flashcards already use. The flat-vs-nested detection was generalized from flashcard-only to a small pair of helpers (`flatTypeForStory`, `isFlatMetadataType`) that also recognize the `journal` sentinel — flashcard behavior is unchanged. Each mistake's logs live under its `id`, bucketed by a `journal` sentinel story title, in `<notebook-id>.yml`.

## API

`quiz.proto` adds `QUIZ_TYPE_GRAMMAR`, a `GrammarCard` (notebook id, card id, sentence, incorrect span, category, note, status — deliberately **not** the reference correction, so the answer isn't leaked), and three RPCs: `StartGrammarQuiz`, `SubmitGrammarAnswer`, `BatchSubmitGrammarAnswers`. `NotebookSummary` gains `grammar_review_count`, and journal notebooks appear in `GetQuizOptions` with `kind: "Journal"`.

## Frontend

The Quiz hub gains a **Grammar** tab (mirroring the Relearn tab): a self-contained start component lists journal notebooks with their due counts and starts a session at `/quiz/grammar`. Grammar uses its own store slice rather than the vocabulary quiz store, because its cards are string-keyed and it doesn't share the override/skip machinery. The session page highlights the incorrect span, takes a correction, shows inline feedback (accepted?, reference correction, explanation), and ends with a per-mistake summary.

## Future work

- AI extraction of `incorrect → correct` pairs from raw journal text (a `parse`-style command) so annotations don't have to be authored by hand.
- A category-frequency analytics view built on the `CategoryCounts` helper.
- Additional drill modes (cloze / production-from-meaning).
