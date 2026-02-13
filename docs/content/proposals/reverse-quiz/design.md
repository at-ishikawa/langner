---
title: "Technical Design"
weight: 2
---

# Technical Design

## Overview

This design adds a new quiz mode that tests word recall (meaning → word). The key design challenge is handling learning history separately from the existing quiz types, since recognition (word → meaning) and production (meaning → word) are different cognitive skills.

## Learning History Design

### Problem

The existing `learned_logs` field stores progress for both Notebook Quiz and Freeform Quiz. These quiz types both show the word first, testing recognition/passive knowledge.

Reverse Quiz tests a fundamentally different skill: producing the word from memory. Progress on recognition doesn't imply progress on production. For example:
- A user might recognize "excited" immediately (Notebook Quiz: 5 correct streak)
- But struggle to recall "excited" when given the meaning (Reverse Quiz: new learning)

Storing both in the same `learned_logs` would conflate these different skills and break spaced repetition scheduling.

### Solution: Separate Fields

Add `reverse_logs` and `reverse_easiness_factor` fields to `LearningHistoryExpression`:

```go
type LearningHistoryExpression struct {
    Expression            string           `yaml:"expression"`
    LearnedLogs           []LearningRecord `yaml:"learned_logs"`
    EasinessFactor        float64          `yaml:"easiness_factor,omitempty"`
    ReverseLogs           []LearningRecord `yaml:"reverse_logs,omitempty"`
    ReverseEasinessFactor float64          `yaml:"reverse_easiness_factor,omitempty"`
}
```

### YAML Format

```yaml
expressions:
  - expression: excited
    easiness_factor: 2.5
    learned_logs:
      - status: understood
        learned_at: "2025-02-01"
        quality: 4
        quiz_type: notebook
        interval_days: 6
    reverse_easiness_factor: 2.5
    reverse_logs:
      - status: understood
        learned_at: "2025-02-05"
        quality: 4
        quiz_type: reverse
        interval_days: 1
```

### Alternatives Considered

| Alternative | Why Rejected |
|-------------|--------------|
| Separate files (`friends.reverse.yml`) | More file management, harder to see complete status |
| Filter by `quiz_type` in single `learned_logs` | SM-2 needs separate `easiness_factor`; streak calculation breaks with interleaved logs |

## Data Model Changes

### QuizType

Add new constant in `internal/notebook/quiz_type.go`:

```go
const QuizTypeReverse QuizType = "reverse"
```

### WordOccurrence

Add field to track notebook type:

```go
type WordOccurrence struct {
    // ... existing fields ...
    IsFlashcard bool // true for flashcard notebooks
}
```

### New Helper Methods

Add to `LearningHistoryExpression`:
- `GetLogs(quizType QuizType)` - returns `ReverseLogs` or `LearnedLogs` based on quiz type
- `GetEasinessFactor(quizType QuizType)` - returns appropriate easiness factor
- `NeedsReverseReview()` - checks if word needs review based on `reverse_logs`

## Answer Validation

### Validation Flow

```
User Input (1st attempt)
    │
    ▼
┌───────────────────┐
│ Normalize input   │  (trim, lowercase)
└─────────┬─────────┘
          │
          ▼
┌───────────────────┐     ┌─────────────────────────┐
│ Exact match?      │────▶│ ✓ Correct (Quality 4-5) │
│ (expression)      │ yes │                         │
└─────────┬─────────┘     └─────────────────────────┘
          │ no
          ▼
┌───────────────────┐     ┌─────────────────────────┐
│ Match definition  │────▶│ ✓ Correct (Quality 4-5) │
│ field?            │ yes │                         │
└─────────┬─────────┘     └─────────────────────────┘
          │ no
          ▼
┌───────────────────┐
│ OpenAI classifies │
│ user's answer     │
└─────────┬─────────┘
          │
    ┌─────┴─────┬────────────────┐
    │           │                │
    ▼           ▼                ▼
┌─────────┐ ┌─────────┐    ┌───────────┐
│ Same    │ │ Valid   │    │ Wrong     │
│ word    │ │ synonym │    │ answer    │
│ form    │ │         │    │           │
└────┬────┘ └────┬────┘    └─────┬─────┘
     │           │               │
     ▼           ▼               ▼
┌─────────┐ ┌─────────┐    ┌───────────────────┐
│✓ Correct│ │ Allow   │    │ ✗ Incorrect       │
│(Q 4-5)  │ │ retry   │    │ (Quality 1)       │
└─────────┘ └────┬────┘    └───────────────────┘
                 │
                 ▼
          User Input (retry)
                 │
                 ▼
          ┌──────┴──────┐
          │             │
          ▼             ▼
     ┌─────────┐  ┌───────────────────┐
     │✓ Correct│  │ ✗ Incorrect       │
     │(Q 3)    │  │ (Quality 1)       │
     └─────────┘  └───────────────────┘
```

### OpenAI Classification

OpenAI classifies user answers into three categories:

| Classification | Description | Example |
|----------------|-------------|---------|
| `same_word` | Different form/conjugation of expected word | "ran" for "run", "exciting" for "excited" |
| `synonym` | Different word with same meaning | "thrilled" for "excited" |
| `wrong` | Doesn't match meaning or context | "banana" for "excited" |

New interface method needed in `internal/inference/interface.go`:

```go
type ValidateWordFormRequest struct {
    Expected   string
    UserAnswer string
    Meaning    string
    Context    string // optional
}

type ValidateWordFormResponse struct {
    Classification string // "same_word", "synonym", or "wrong"
    Reason         string
}
```

## Context Sources

| Notebook Type | Context Source | Display Label |
|---------------|----------------|---------------|
| Story | `scene.Conversations` | "Context" + speaker |
| Flashcard | `note.Examples` | "Example" |

Words without context/examples show only the meaning.

## List Missing Context

The `--list-missing-context` flag scans all notebooks and reports words that lack context:

| Notebook Type | Has Context When |
|---------------|------------------|
| Story | Word appears in at least one conversation in the scene |
| Flashcard | `note.Examples` is not empty AND word appears in at least one example |

## Quality Scoring

| Quality | Condition |
|---------|-----------|
| 5 | Correct on first attempt, fast (< 3 seconds) |
| 4 | Correct on first attempt, normal speed (3-10 seconds) |
| 3 | Correct on first attempt, slow (> 10 seconds) OR correct on retry |
| 1 | Incorrect (wrong answer or failed retry) |

Response time is measured from question display to first answer submission.

## Files to Modify

```
internal/
├── cli/
│   └── reverse_quiz.go          # NEW: ReverseQuizCLI
├── notebook/
│   ├── learning_history.go      # MODIFY: add reverse fields
│   ├── learning_history_updater.go # MODIFY: support reverse logs
│   └── quiz_type.go             # MODIFY: add QuizTypeReverse
├── inference/
│   ├── interface.go             # MODIFY: add ValidateWordForm
│   └── openai/
│       └── client.go            # MODIFY: implement ValidateWordForm
cmd/langner/
└── quiz.go                      # MODIFY: add reverse subcommand
```

## Migration

No migration needed. New fields are optional and default gracefully:
- `reverse_logs`: empty means word needs first reverse review
- `reverse_easiness_factor`: 0 defaults to 2.5
