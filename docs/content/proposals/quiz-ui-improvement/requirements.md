---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

Three enhancements to the quiz experience that apply to all quiz types (Standard, Reverse, Freeform):

1. **Override Answer** — Let users correct OpenAI grading mistakes (mark incorrect as correct, or correct as incorrect)
2. **Skip Word** — Permanently remove a word from all future quizzes
3. **Next Review Date** — Display when a word will next appear for review, and allow the user to change it

## Problem

### Incorrect Grading by OpenAI

OpenAI grading can be wrong in both directions:

- **False negative (marks correct as incorrect)**: A user answers "to deceive" for the word "dupe", but OpenAI grades it wrong because it expected "to trick". The word gets quality 1, the interval resets, and the easiness factor drops — all for a correct answer.
- **False positive (marks incorrect as correct)**: A user gives a vague or partially wrong answer, but OpenAI accepts it. The word gets a high quality score and a long interval, so the user won't see it again for a while despite not truly knowing it.

There is currently no way to correct either case without manually editing learning history files.

### Words That Cannot Be Graded

Some words consistently fail OpenAI grading due to ambiguity, unusual definitions, or context-dependent meanings. Other words the user has already mastered and no longer needs to review. In both cases, users want to stop seeing these words in quizzes, but there is no way to exclude them without removing them from the notebook YAML files.

### No Visibility Into Review Schedule

After answering a word, users cannot see when it will next appear for review. This makes the spaced repetition schedule opaque and prevents users from understanding their learning progress during the quiz itself. Additionally, sometimes the SM-2 algorithm schedules a review too far in the future or too soon, and users want to adjust the date manually.

## Goals

- Allow users to override OpenAI grading mistakes in both directions (incorrect→correct and correct→incorrect)
- Allow users to skip words from quizzes (reversible from the notebook page)
- Show the next review date after each answer and on the results page
- Allow users to change the next review date to a date of their choosing
- Apply all three features consistently across Standard, Reverse, and Freeform quizzes

## User Stories

### Feature 1: Override Answer

#### Mark as Correct

As a user, when OpenAI incorrectly marks my answer as wrong, I want to override it to correct.

- When an answer is marked incorrect, a "Mark as Correct" option is available
- Overriding changes the result to correct, re-saves the learning log with quality 4 (correct), and recalculates the SM-2 interval
- The incorrect indicator updates to show correct with an "(overridden)" label

#### Mark as Incorrect

As a user, when OpenAI incorrectly marks my answer as right, I want to override it to incorrect.

- When an answer is marked correct, a "Mark as Incorrect" option is available
- Overriding changes the result to incorrect, re-saves the learning log with quality 1 (wrong), and recalculates the SM-2 interval
- The correct indicator updates to show incorrect with an "(overridden)" label

#### Undo Override

As a user, when I accidentally override an answer, I want to undo the override.

- After overriding in either direction, an "Undo" option appears
- Clicking it reverts the learning log to the original grading (quality, status, interval, easiness factor)
- The indicator returns to its original state

#### Override From Results

As a user, when I review my quiz results and notice a grading mistake, I want to override it from the results summary.

- Each incorrect result shows a "Mark as Correct" option
- Each correct result shows a "Mark as Incorrect" option
- Overriding moves the word between the Correct and Incorrect sections
- The summary counts update accordingly
- An "Undo" option is available after overriding

### Feature 2: Skip Word

#### Skip After Answering

As a user, I want to skip a word so it stops appearing in quizzes.

- After answering any word (correct or incorrect), a "Skip" option is available
- Clicking it immediately marks the word as skipped — no confirmation dialog needed since the action is reversible from the notebook page
- The word will not appear in Standard, Reverse, or Freeform quizzes until the skip is undone

#### Skip From Results

As a user, I want to skip a word from the results summary.

- Each result shows a "Skip" option
- Clicking it immediately marks the word as skipped (dimmed with "Skipped" label)
- The word remains visible for the current session but will not appear in future quizzes

### Feature 3: Next Review Date

#### View Next Review Date

As a user, I want to see when a word will next appear for review after I answer it.

- After answering a word, the next review date is displayed
- The date is also shown for each word on the results summary
- Format: human-readable date (e.g., "March 25, 2026"), with relative phrasing for near dates (e.g., "tomorrow", "in 3 days")

#### Change Next Review Date

As a user, I want to change the next review date for a word.

- After answering or on the results summary, the user can change the next review date
- The user can pick a specific date using a date picker
- This is useful when:
  - The user feels unsure about a word even though they answered correctly (move the date closer)
  - The SM-2 interval is too short and the user wants to push the review further out
- The selected date overrides the SM-2-calculated interval for this review cycle only; future SM-2 calculations continue normally from the new interval

### Feature 4: Resume Skipped Words (Notebook Page)

#### Filter Skipped Words

As a user, I want to see which words in a notebook have been skipped so I can re-enable them if needed.

- The notebook page adds a filter option to show only skipped words (words that won't be reviewed anymore)
- Skipped words are visually distinct (e.g., dimmed or with a "Skipped" badge)

#### Resume

As a user, I want to re-enable a skipped word so it appears in quizzes again.

- Each skipped word shows a "Resume" option
- Clicking it removes the skip and the word will appear in future quizzes again based on its SM-2 schedule
- No confirmation dialog needed (low-risk, easily re-skippable action)

## Applicability

| Feature | Where |
|---------|-------|
| Override Answer (both directions + undo) | Quiz feedback + Results page (all quiz types) |
| Skip | Quiz feedback + Results page (all quiz types) |
| Next Review Date | Quiz feedback + Results page (all quiz types) |
| Change Review Date | Quiz feedback + Results page (all quiz types) |
| Resume Skipped Word | Notebook page |

## Out of Scope

- Bulk skip/override from the results page
- Showing the full spaced repetition schedule or learning history
