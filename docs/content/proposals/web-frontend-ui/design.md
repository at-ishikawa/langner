---
title: "UI/UX Design"
weight: 2
---

# UI/UX Design

Mobile-first wireframes (375px viewport) for the notebook recognition quiz.

## No Menu

Since only the notebook recognition quiz is in scope, there is no menu or navigation. The app opens directly to the Quiz screen.

## Screen Flow

![Screen Flow](/proposals/web-frontend-ui/wireframes/00-screen-flow.svg)

## Screens

### Screen 1: Quiz (Start)

- Notebooks are listed with checkboxes for multi-select. Each row shows the notebook name and the number of words due for review.
- "All notebooks" is a convenience checkbox that selects/deselects all.
- Toggle for "Include unstudied words".
- Total words due shown above the Start button.

![Quiz Start](/proposals/web-frontend-ui/wireframes/01-quiz-start.svg)

### Screen 2: Quiz Card

- Progress shown as "3 / 25" text and a progress bar at the top.
- The word is displayed prominently in a card with example sentences formatted as `Speaker: "quote"`.
- Content area is scrollable for long examples.
- Text input and Submit button are fixed at the bottom of the screen.
- Text input is auto-focused so the user can start typing immediately.

![Quiz Card](/proposals/web-frontend-ui/wireframes/02-quiz-card.svg)

### Screen 3a: Feedback (Correct)

- A green bar and checkmark indicate correct. Shows the user's answer, the canonical meaning, and the grading reason.
- Content area is scrollable. Text input and Submit button remain fixed at the bottom, auto-focused for the next word.

![Feedback Correct](/proposals/web-frontend-ui/wireframes/03-feedback-correct.svg)

### Screen 3b: Feedback (Incorrect)

- A red bar and X mark indicate incorrect. The user's wrong answer is shown with strikethrough.
- Same layout as the correct feedback screen.

![Feedback Incorrect](/proposals/web-frontend-ui/wireframes/03-feedback-incorrect.svg)

### Screen 4: Session Complete

- Shows total words practiced, correct count (green), and incorrect count (red).
- Lists all correct and incorrect words with their meanings, grouped by result.
- "Back to Start" button returns to the Quiz screen.

![Session Complete](/proposals/web-frontend-ui/wireframes/04-session-complete.svg)
