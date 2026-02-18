---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

Add a web UI for the notebook recognition quiz. This is the same flow as `langner quiz notebook` in recognition mode, with the ability to take actions on previous results during the session.

## Problem

In the CLI quiz, once a word is answered, the result scrolls away and the user cannot interact with it further. If the user wants to take an action on a previously answered word — such as marking it to not show again — they must stop the quiz and manually edit YAML files. This breaks the study flow and is error-prone.

## Goals

- Provide a web UI for the notebook recognition quiz
- Keep the CLI fully functional — the web UI is an alternative interface, not a replacement

## User Stories

### Start a Quiz

As a learner, I want to start a notebook recognition quiz from the web UI.

- Select a notebook (or "All notebooks") from a list
- Optionally toggle "Include unstudied words"
- See how many words are due for review

### Answer a Question

As a learner, I want to see a word with its context and type what I think it means.

- The word is displayed with context sentences from the notebook, with the word highlighted
- If no context is available, only the word is shown
- Type my answer and submit

### Get Feedback

As a learner, I want to know if my answer was correct and understand why.

- See correct or incorrect indicator
- See the canonical meaning of the word
- See the grading reason from OpenAI
- Proceed to the next word

### Complete a Session

As a learner, I want to see a summary when I finish all cards.

- See number of words practiced
- See correct and incorrect counts

## Out of Scope

- Freeform quiz mode
- Reverse quiz mode
- Notebook creation or editing
- Learning statistics or progress tracking
- Push notifications or review reminders
- Offline quiz support
- Multi-user support
