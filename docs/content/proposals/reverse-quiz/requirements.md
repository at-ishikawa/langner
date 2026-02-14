---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

Add a new quiz mode that tests word recall by showing the meaning and asking the user to provide the word. This is the reverse direction of the existing Notebook Quiz (which shows the word and asks for the meaning).

## Problem

The current Notebook Quiz tests recognition: given a word, can you understand its meaning? However, productive vocabulary (being able to produce the right word when you need it) requires the reverse skill: given a meaning or context, can you recall the word?

A key challenge is that multiple words can share similar meanings. For example, "feeling very happy" could be "excited", "thrilled", or "eager". The quiz must be unambiguous about which word is expected.

## Solution

Show the meaning along with the context sentence (when available), with the target word masked. This makes it clear which specific word the user should recall.

## Goals

- Test productive vocabulary (meaning → word recall)
- Provide unambiguous questions using masked context
- Accept alternate word forms (e.g., "run" for "running") via OpenAI validation
- Reuse the existing spaced repetition system

## User Stories

### Take a Reverse Quiz

As a user, I want to quiz myself by seeing meanings and recalling the words.

```bash
langner quiz reverse
```

This will:
- Show words that need review based on spaced repetition
- Display the meaning and masked context for each word
- Validate the user's answer
- Update learning history

### Quiz with Context Available

When a word has context from a story, show the meaning and the context with the word masked:

```
Meaning: feeling very happy and enthusiastic

Context: "I'm so _______ about the trip tomorrow!"
         - Alice

Your answer: excited

✓ Correct!
```

### Quiz without Context

When a word has no context (e.g., flashcard entries), show only the meaning:

```
Meaning: feeling very happy and enthusiastic

Your answer: excited

✓ Correct!
```

### Alternate Word Forms

The quiz accepts alternate word forms. OpenAI validates whether the user's answer is an acceptable form of the expected word.

```
Meaning: to move quickly on foot

Context: "She _______ to catch the bus."

Your answer: ran

✓ Correct! (Expected: run)
```

### Synonym Retry

When the user enters a valid synonym (a different word that fits the context) instead of the expected word, they get one retry opportunity:

```
Meaning: feeling very happy and enthusiastic

Context: "I'm so _______ about the trip tomorrow!"
         - Alice

Your answer: thrilled

"thrilled" is a valid synonym, but not the word we're looking for. Try again.

Your answer: excited

✓ Correct!
```

If the user fails the retry, the answer is marked as incorrect:

```
Meaning: feeling very happy and enthusiastic

Context: "I'm so _______ about the trip tomorrow!"
         - Alice

Your answer: thrilled

"thrilled" is a valid synonym, but not the word we're looking for. Try again.

Your answer: eager

✗ Incorrect. The answer was "excited".
```

Note: Only one retry is allowed per question. If the user enters a completely wrong answer (not a synonym), no retry is given.

### Quiz a Specific Notebook

As a user, I want to quiz myself on a specific notebook.

```bash
langner quiz reverse --notebook friends
```

### Flashcard Examples

For flashcard notebooks, show examples instead of story contexts:

```
Meaning: present, appearing, or found everywhere

Example: "Smartphones have become _______ in modern society."

Your answer: ubiquitous

✓ Correct!
```

### List Words Without Context

As a user, I want to see which words don't have contexts (stories) or examples (flashcards), so I can add them before taking the reverse quiz.

```bash
langner quiz reverse --list-missing-context
```

Output for story notebooks:
```
Words without context in story notebooks:

friends:
  - S01E01 - The Pilot > Central Perk:
    - excited (no conversations contain this word)
    - nervous

the-office:
  - S01E01 - Pilot > Conference Room:
    - inappropriate
```

Output for flashcard notebooks:
```
Words without examples in flashcard notebooks:

vocabulary:
  - ephemeral (no examples)
  - cacophony (no examples)
```

This helps users identify words that will only show the meaning (no masked context) in the reverse quiz, which makes them harder to answer correctly.

## Display Format

### Story Notebooks (With Context)

```
┌─────────────────────────────────────────────────────────────┐
│  Meaning: <definition from notebook>                        │
│                                                             │
│  Context: "<conversation with _______ replacing the word>"  │
│           - <speaker>                                       │
│                                                             │
│  Your answer: _                                             │
└─────────────────────────────────────────────────────────────┘
```

### Flashcard Notebooks (With Example)

```
┌─────────────────────────────────────────────────────────────┐
│  Meaning: <definition from notebook>                        │
│                                                             │
│  Example: "<example sentence with _______ replacing word>"  │
│                                                             │
│  Your answer: _                                             │
└─────────────────────────────────────────────────────────────┘
```

### Without Context/Example

```
┌─────────────────────────────────────────────────────────────┐
│  Meaning: <definition from notebook>                        │
│                                                             │
│  Your answer: _                                             │
└─────────────────────────────────────────────────────────────┘
```

## Answer Validation

1. **Exact match**: Case-insensitive match with the expected expression → Correct
2. **Definition match**: Match with the `definition` field (alternate base form) → Correct
3. **OpenAI validation**: For other cases, ask OpenAI to classify the answer:
   - **Same word, different form** (conjugation, tense, plural) → Correct
   - **Valid synonym** (different word that fits the context) → Allow one retry
   - **Wrong answer** (doesn't fit the context) → Incorrect

## Learning History

- Uses the same spaced repetition system (SM-2 algorithm)
- Records quiz type as `"reverse"` in learning history
- Uses separate `reverse_logs` field (independent from notebook/freeform quiz progress)
- Quality scoring with retry:
  - Correct on first attempt: Quality 4-5 (based on response time)
  - Correct on retry (after synonym): Quality 3 (correct but struggled)
  - Incorrect (after retry or wrong answer): Quality 1

## Commands

| Command | Description |
|---------|-------------|
| `langner quiz reverse` | Start reverse quiz for all notebooks |
| `langner quiz reverse --notebook <name>` | Quiz a specific notebook |
| `langner quiz reverse --list-missing-context` | List words without context/examples |

## Out of Scope

- Hints (first letter, word length) - may be added in future iterations
- Multiple choice mode - may be added in future iterations
