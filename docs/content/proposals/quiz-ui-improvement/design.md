---
title: "UI/UX Design"
weight: 2
---

# UI/UX Design

Mobile-first wireframes (375px viewport) for three quiz enhancements across all quiz types.

## Feedback Phase Changes

The feedback phase (shown after submitting an answer) gains three new elements for all quiz types:
1. Next review date with date change option
2. Override button: "Mark as Correct" (on incorrect answers) or "Mark as Incorrect" (on correct answers)
3. "Skip" button

The layout below shows the Standard Quiz. The same pattern applies to Reverse and Freeform quizzes — only the content fields differ (word vs. meaning, context format, etc.).

### Feedback — Incorrect

When the answer is incorrect, "Mark as Correct", next review date with "Change", and "Skip" are shown.

![Feedback Incorrect](/proposals/quiz-ui-improvement/wireframes/01-feedback-incorrect.svg?v=5)

### Feedback — Correct

When the answer is correct, "Mark as Incorrect" (red outline), next review date with "Change", and "Skip" are shown.

![Feedback Correct](/proposals/quiz-ui-improvement/wireframes/03-feedback-correct.svg?v=5)

### Feedback — After Override

After clicking either "Mark as Correct" or "Mark as Incorrect":
- The indicator flips to the opposite state with an "(overridden)" label
- The override button is replaced by an "Undo" link next to the label
- The next review date updates to reflect the recalculated interval
- Clicking "Undo" restores the original grading, interval, and easiness factor

The wireframe below shows overriding from incorrect to correct. The reverse direction (correct→incorrect) follows the same pattern with red styling.

![Feedback Overridden](/proposals/quiz-ui-improvement/wireframes/02-feedback-overridden.svg?v=5)

## Change Review Date

When the user clicks "Change" next to the review date, a date picker appears. The user selects a new date and the review date updates immediately.

- Defaults to the current next review date
- Minimum date: tomorrow
- No maximum date
- Shows the original date for reference after changing

![Change Date](/proposals/quiz-ui-improvement/wireframes/04-change-date.svg?v=5)

## Skip Behavior

When the user clicks "Skip", the word is immediately marked as skipped — no confirmation dialog since the action is reversible from the notebook page. The "Skip" button is replaced with a "Skipped" label (dimmed text).

## Results Page Changes

Each result card on the Session Complete page is enhanced with the new actions.

- **Incorrect cards**: Show "Mark as Correct" and "Skip" buttons, plus next review date with "Change"
- **Correct cards**: Show "Mark as Incorrect" and "Skip" buttons, plus next review date with "Change"
- **Skipped cards**: Normal appearance with a "Skipped" badge and a "Resume" button to re-enable
- When an override is clicked, the card moves between sections and summary counts update
- An "Undo" link appears after overriding to restore the original grading

![Result Cards](/proposals/quiz-ui-improvement/wireframes/06-result-cards.svg?v=5)

## Notebook Page — Resume Skipped Words

The existing notebook detail page (`/notebooks/[id]`) has a status filter dropdown (All, Learning, Misunderstood, Understood, Usable, Intuitive). A new **"Skipped"** filter option is added.

- Selecting "Skipped" shows only words that have been skipped from quizzes
- Each skipped word card is dimmed and shows a "Skipped" badge, the skip date, and a "Resume" button
- Clicking "Resume" restores the word — it disappears from the "Skipped" filter and will appear in future quizzes again based on its SM-2 schedule

![Notebook Skipped](/proposals/quiz-ui-improvement/wireframes/07-notebook-skipped.svg?v=5)

## Quiz-Type-Specific Notes

The wireframes above show the Standard Quiz layout. For other quiz types:

### Reverse Quiz

- The heading shows the meaning (blue) instead of the word
- Feedback shows the "Word" field instead of "Meaning" field
- Context shows numbered context sentences
- All new elements (override, skip, review date) appear in the same positions

### Freeform Quiz

- No progress bar (open-ended quiz)
- Feedback shows "Word" and "Correct meaning" fields
- "Found in: [notebook]" line appears
- Additional buttons: "Next Word", "See Results", "Back to Start"
- All new elements appear in the same positions relative to existing content

## Button Hierarchy

| Button | Style | Placement |
|--------|-------|-----------|
| Next / See Results | Primary (blue, full width) | Main action |
| Mark as Correct | Outline (blue) | Below Next, only on incorrect |
| Mark as Incorrect | Outline (red) | Below Next, only on correct |
| Undo | Link (blue, small text) | Next to "(overridden)" label, after override |
| Change (review date) | Link/ghost (small text) | Next to review date |
| Skip | Outline (gray border) | Below override button |
