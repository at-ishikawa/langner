---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Problem

The learner keeps a free-text English journal that contains recurring grammar mistakes — for example a definite article before a personal name ("the John"), the wrong preposition for a place ("in school" instead of "at school"), or the wrong verb complement ("suggested to go" instead of "suggested going"). Vocabulary quizzes don't help here: the unit of learning is a *mistake pattern in a real sentence*, not a word and its meaning.

## Goals

- Keep journal entries as the notebook content, so practice happens on the learner's own sentences rather than invented ones.
- Let the learner annotate each mistake with its correction and a free-form category.
- Drill each mistake as a correction exercise (see the sentence, type the fix), graded by AI.
- Track each mistake with the existing spaced-repetition schedule so mastered mistakes stop appearing.
- Rank mistakes by category so the learner can see which kinds of mistakes are most frequent.

## Non-goals

- Automatic mistake detection from raw journal text (a future AI-extraction step). v1 authors annotations by hand.
- Multiple-choice / cloze variants. v1 ships correction only.
- A closed category taxonomy. Categories are free-form strings with a suggested starter set.

## User stories

1. As a learner, I add a journal notebook (a folder of dated entries) and annotate the mistakes in each entry.
2. As a learner, I open **Quiz → Grammar**, pick a journal, and correct each highlighted mistake.
3. As a learner, I see whether my correction was accepted, the reference correction, and a short explanation.
4. As a learner, a mistake I fix correctly disappears from review until its next scheduled date; one I get wrong comes back.
5. As a learner (future), I see a ranking of my most frequent mistake categories.

## Suggested category set

`article`, `preposition`, `verb-pattern`, `tense`, `agreement`, `word-order`, `plural`, `punctuation`. Free-form, so the learner can add their own.
