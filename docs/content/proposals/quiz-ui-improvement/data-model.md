---
title: "Data Model"
weight: 3
---

# Data Model

## Overview

Database schema and API contract changes to support quiz UI improvements:
1. Override Answer (both directions + undo)
2. Skip Word (+ undo from notebook page)
3. Next Review Date with user-chosen date

## Storage Changes

The project uses dual storage: YAML files for learning history and a MySQL database. Both must be updated for skip support.

### YAML Learning History

Add `skipped_at` to `LearningHistoryExpression`:

```go
type LearningHistoryExpression struct {
    Expression     string           `yaml:"expression"`
    LearnedLogs    []LearningRecord `yaml:"learned_logs"`
    EasinessFactor float64          `yaml:"easiness_factor,omitempty"`

    ReverseLogs           []LearningRecord `yaml:"reverse_logs,omitempty"`
    ReverseEasinessFactor float64          `yaml:"reverse_easiness_factor,omitempty"`

    SkippedAt string `yaml:"skipped_at,omitempty"` // NEW: RFC3339 date when skipped
}
```

Example YAML:

```yaml
expressions:
  - expression: break the ice
    easiness_factor: 2.5
    learned_logs:
      - status: understood
        learned_at: "2026-02-18"
        quality: 4
        quiz_type: notebook
        interval_days: 7
  - expression: ubiquitous
    skipped_at: "2026-03-15"   # Skipped from all quizzes
    easiness_factor: 2.5
    learned_logs:
      - status: understood
        learned_at: "2026-01-09"
        quality: 4
        quiz_type: notebook
        interval_days: 14
```

When filtering words for quizzes, expressions with a non-empty `skipped_at` are excluded.

### Database Schema

New migration to add skip support:

```sql
-- Migration: 00X_add_skip_words.up.sql

ALTER TABLE notes
  ADD COLUMN skipped_at TIMESTAMP NULL
  COMMENT 'When this note was marked to skip from quizzes';

CREATE INDEX idx_notes_skipped_at ON notes (skipped_at);
```

```sql
-- Migration: 00X_add_skip_words.down.sql

ALTER TABLE notes DROP COLUMN skipped_at;
```

### Design Decision: `skipped_at` timestamp

| Alternative | Why Rejected |
|-------------|-------------|
| Separate `skipped_words` table | Over-engineering for a boolean flag; no need for per-quiz-type skipping initially |
| Boolean `is_skipped` column | Timestamp provides audit trail of when the skip happened |
| Store only in DB, not in YAML | YAML is the primary source of truth for CLI workflows; both must stay in sync |

No schema changes are needed for Override Answer or Change Review Date — both operate on the existing `learning_logs` table (DB) or `learned_logs` / `reverse_logs` (YAML) by updating the most recent log entry.

## Proto API Changes

### New RPCs

```protobuf
service QuizService {
  // ... existing RPCs ...

  // Override a learning log entry (change correctness, review date, or both)
  rpc OverrideAnswer(OverrideAnswerRequest) returns (OverrideAnswerResponse);

  // Undo a previous override, restoring the original values
  rpc UndoOverrideAnswer(UndoOverrideAnswerRequest) returns (UndoOverrideAnswerResponse);

  // Mark a word to skip from all future quizzes
  rpc SkipWord(SkipWordRequest) returns (SkipWordResponse);

  // Re-enable a skipped word for quizzes
  rpc ResumeWord(ResumeWordRequest) returns (ResumeWordResponse);
}
```

### New Messages

```protobuf
// Override Answer
//
// Modifies a specific learning log identified by (note_id, quiz_type, learned_at).
// Can override correctness, review date, or both in a single call.
// In the database: matches the unique constraint (note_id, quiz_type, learned_at).
// In YAML: finds the log entry with matching learned_at in learned_logs or reverse_logs.
//
// The response returns the original values so the client can pass them back
// to UndoOverrideAnswer for restoration.

message OverrideAnswerRequest {
  int64 note_id = 1 [
    (buf.validate.field).int64.gt = 0
  ];
  QuizType quiz_type = 2;
  // Timestamp of the learning log to override (identifies the exact log entry)
  string learned_at = 3 [
    (buf.validate.field).string.min_len = 10
  ];
  // Optional: override correctness (omit to leave unchanged)
  optional bool mark_correct = 4;
  // Optional: override next review date (omit to let SM-2 recalculate)
  optional string next_review_date = 5;
}

message OverrideAnswerResponse {
  string next_review_date = 1;  // Updated next review date (YYYY-MM-DD)
  // Original values for undo — client caches these
  int32 original_quality = 2;
  string original_status = 3;
  int32 original_interval_days = 4;
  double original_easiness_factor = 5;
}

// Undo Override Answer
//
// Restores a specific learning log to the original values
// that were returned in OverrideAnswerResponse.

message UndoOverrideAnswerRequest {
  int64 note_id = 1 [
    (buf.validate.field).int64.gt = 0
  ];
  QuizType quiz_type = 2;
  // Timestamp of the learning log to restore (same as in OverrideAnswerRequest)
  string learned_at = 3 [
    (buf.validate.field).string.min_len = 10
  ];
  // Original values to restore (from OverrideAnswerResponse)
  int32 original_quality = 4;
  string original_status = 5;
  int32 original_interval_days = 6;
  double original_easiness_factor = 7;
}

message UndoOverrideAnswerResponse {
  bool correct = 1;             // Restored original correctness
  string next_review_date = 2;  // Restored next review date (YYYY-MM-DD)
}

// Skip Word

message SkipWordRequest {
  int64 note_id = 1 [
    (buf.validate.field).int64.gt = 0
  ];
}

message SkipWordResponse {}

// Resume Skipped Word

message ResumeWordRequest {
  int64 note_id = 1 [
    (buf.validate.field).int64.gt = 0
  ];
}

message ResumeWordResponse {}
```

### Override Behavior

The `OverrideAnswerRequest` supports three use cases with the same RPC:

| Use Case | `mark_correct` | `next_review_date` |
|----------|---------------|-------------------|
| Change correctness only | set (`true` or `false`) | omit (SM-2 recalculates) |
| Change review date only | omit (unchanged) | set (YYYY-MM-DD) |
| Change both | set | set |

When `mark_correct` is set, the server updates `quality`, `status`, `interval_days`, and `easiness_factor` using SM-2 recalculation. When `next_review_date` is set, the server computes `interval_days = next_review_date - learned_at`. If both are set, the explicit `next_review_date` takes precedence over the SM-2-calculated interval.

### Quality Mapping for Overrides

When the user overrides correctness, the SM-2 algorithm needs a quality grade. Since the user is manually correcting the grading (not answering again), the following fixed quality values are used:

| Override Direction | Quality | Rationale |
|-------------------|---------|-----------|
| Mark as Correct | 3 (correct but struggled) | Conservative: the user needed to override, so they likely struggled. Avoids inflating the interval as much as quality 4-5 would. |
| Mark as Incorrect | 1 (wrong) | Same as a wrong answer — the user is saying they didn't actually know it. |

These match the existing quality constants in `quiz_type.go`:

```go
const (
    QualityWrong       = 1  // Incorrect answer
    QualityCorrectSlow = 3  // Correct but struggled
    QualityCorrect     = 4  // Correct at normal speed
    QualityCorrectFast = 5  // Correct and fast
)
```

Quality 3 is chosen for "Mark as Correct" (rather than 4) because the fact that OpenAI graded it wrong suggests the answer was borderline — a perfect answer wouldn't have been misjudged. This gives a shorter interval than a clean correct answer, which is safer for learning.

### Modified Existing Messages

Add `is_skipped` to `NotebookWord` so the notebook page can display skip status:

```protobuf
message NotebookWord {
  // ... existing fields ...
  bool is_skipped = 14;  // NEW: true if word is skipped from quizzes
}
```

Add `next_review_date` and `learned_at` to all submit answer responses. The `learned_at` timestamp identifies the log entry for subsequent override or change-date calls.

```protobuf
message SubmitAnswerResponse {
  bool correct = 1;
  string meaning = 2;
  string reason = 3;
  WordDetail word_detail = 4;
  string next_review_date = 5;  // NEW: next review date (YYYY-MM-DD)
  string learned_at = 6;        // NEW: timestamp of the created log entry
}

message SubmitReverseAnswerResponse {
  bool correct = 1;
  string expression = 2;
  string meaning = 3;
  string reason = 4;
  repeated string contexts = 5;
  WordDetail word_detail = 6;
  string classification = 7;
  string next_review_date = 8;  // NEW: next review date (YYYY-MM-DD)
  string learned_at = 9;        // NEW: timestamp of the created log entry
}

message SubmitFreeformAnswerResponse {
  bool correct = 1;
  string word = 2;
  string meaning = 3;
  string reason = 4;
  string context = 5;
  string notebook_name = 6;
  WordDetail word_detail = 7;
  string next_review_date = 8;  // NEW: next review date (YYYY-MM-DD)
  string learned_at = 9;        // NEW: timestamp of the created log entry
}
```

### Next Review Date Calculation

The `next_review_date` is calculated as:

```
next_review_date = learned_at + interval_days
```

This is already available from the SM-2 calculation after saving a result. The server formats it as YYYY-MM-DD.
