---
title: "Backend & Data Model"
weight: 3
---

# Backend & Data Model

## Overview

Backend design for etymology-based word learning, covering YAML notebook format, database schema, API contracts, and OpenAI grading integration.

### Goals

- Add a new etymology notebook type (`kind: Etymology`) that defines origins (roots, prefixes, suffixes)
- Add `origin_parts` to definitions in existing notebooks (book, flashcard, story) to link vocabulary to origins
- Expose etymology data through the existing NotebookService and QuizService APIs
- Grade etymology quiz answers (Breakdown, Assembly, Freeform) using OpenAI
- Track etymology quiz learning history with SM-2 spaced repetition

### Non-Goals

- Migrate existing notebook types to use etymology structures
- Change the SM-2 algorithm
- Auto-generate etymology data from dictionaries

## Data Model

### YAML: Etymology Notebook (Origins)

Etymology notebooks use the existing `Index` structure with `kind: Etymology`. Session files contain a flat list of origins — no words/definitions.

**index.yml:**

```yaml
id: greek-latin-roots
kind: Etymology
name: "Greek & Latin Roots"
notebooks:
  - ./session1.yml
  - ./session2.yml
```

**session1.yml:**

```yaml
- origin: tele
  type: prefix
  language: Greek
  meaning: far

- origin: phone
  type: root
  language: Greek
  meaning: "sound, voice"

- origin: scope
  type: root
  language: Greek
  meaning: "to look at"

- origin: bi
  type: prefix
  language: Latin
  meaning: two

- origin: kyklos
  type: root
  language: Greek
  meaning: "circle, wheel"

- origin: graphein
  type: root
  language: Greek
  meaning: to write

- origin: scribere
  type: root
  language: Latin
  meaning: to write
```

**Key: `(origin, language)` uniqueness.** Within a notebook, no two origins may share the same `origin` + `language` pair. If the same spelling exists in different languages (e.g., Greek `pan` = "all" vs. Old English `pan` = "cooking vessel"), they are distinct origins.

### YAML: Definitions with `origin_parts`

Definitions in existing notebooks (book, flashcard, story) gain an optional `origin_parts` field. This links a vocabulary entry to origins defined in etymology notebooks.

```yaml
# In a book, flashcard, or story notebook's definitions
- expression: telephone
  meaning: "a device for transmitting sound over a distance"
  origin_parts:
    - origin: tele
      language: Greek
    - origin: phone
      language: Greek

- expression: microscope
  meaning: "an instrument for viewing very small objects"
  origin_parts:
    - origin: micro
      language: Greek
    - origin: scope
      language: Greek

- expression: describe
  meaning: "to give an account of something"
  origin_parts:
    - origin: de
      language: Latin
    - origin: scribere
      language: Latin
```

`origin_parts` is distinct from the existing `origin` field on definitions. The `origin` field stores a free-text etymology description (e.g., "from Latin describere"). The `origin_parts` field stores structured references to origins defined in etymology notebooks.

### Database Schema

The `etymology_origins` table caches origins from YAML. Definitions with `origin_parts` are linked via the existing `notes` table plus a new junction table.

```sql
-- Origins (loaded from etymology notebook YAML, cached in DB)
CREATE TABLE etymology_origins (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    notebook_id VARCHAR(255) NOT NULL COMMENT 'Etymology notebook ID from index.yml',
    origin VARCHAR(255) NOT NULL COMMENT 'The morpheme (e.g., tele, phone)',
    type VARCHAR(50) COMMENT 'root, prefix, or suffix',
    language VARCHAR(100) COMMENT 'Source language (e.g., Greek, Latin)',
    meaning TEXT NOT NULL COMMENT 'Meaning of the origin',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, origin, language),
    INDEX (origin, language),
    INDEX (meaning(100))
) COMMENT='Etymological origins (roots, prefixes, suffixes)';

-- Links notes (definitions from any notebook) to their origins (ordered)
CREATE TABLE note_origin_parts (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL COMMENT 'Reference to notes table',
    origin_id BIGINT NOT NULL COMMENT 'Reference to etymology_origins',
    sort_order INT NOT NULL COMMENT 'Position in the word composition',
    FOREIGN KEY (note_id) REFERENCES notes(id),
    FOREIGN KEY (origin_id) REFERENCES etymology_origins(id),
    UNIQUE (note_id, sort_order),
    INDEX (origin_id)
) COMMENT='Ordered links between vocabulary notes and their etymological origins';
```

This reuses the existing `notes` table for definitions. Definitions that have `origin_parts` are regular notes with rows in `note_origin_parts`.

### Learning History

Learning history reuses the existing `learning_logs` table with new quiz type values. Etymology quizzes reference the same `note_id` as other quizzes (since definitions live in the `notes` table).

YAML learning history also reuses the existing format. Etymology quiz results are stored in the source notebook's learning history file:

```yaml
metadata:
  notebook_id: frankenstein    # The book notebook, not the etymology notebook
  title: "Frankenstein"
  type: book
expressions:
  - expression: telephone
    easiness_factor: 2.5
    learned_logs:
      - status: understood
        learned_at: "2026-03-17"
        quality: 4
        quiz_type: etymology_breakdown
        interval_days: 7
```

### SM-2 Quiz Type Mapping

Etymology quizzes use distinct quiz types for independent SM-2 scheduling:

| Quiz Mode | `quiz_type` value | SM-2 Track |
|-----------|-------------------|------------|
| Breakdown | `etymology_breakdown` | Independent |
| Assembly | `etymology_assembly` | Independent |
| Freeform | `etymology_freeform` | Shares track with Breakdown |

Breakdown and Freeform share the same SM-2 track because they test the same skill (word → origins). Assembly tests the reverse direction and gets its own track, similar to how the existing reverse quiz has a separate track from standard/freeform.

### Entity Relationship

```
YAML Files                              Database
──────────────────                      ──────────────────

┌─────────────────┐                     ┌──────────────────┐
│ Etymology       │ ──────────────────> │ etymology_origins│
│ Notebook (YAML) │                     └──────────────────┘
│                 │                              │
│ - origin: tele  │                              │
│ - origin: phone │                     ┌──────────────────┐
└─────────────────┘                     │ note_origin_parts│
                                        └──────────────────┘
┌─────────────────┐                              │
│ Book / Flashcard│                              │
│ Notebook (YAML) │                     ┌──────────────────┐
│                 │ ──────────────────> │      notes       │
│ - expression:   │                     └──────────────────┘
│     telephone   │                              │
│   origin_parts: │                              ▼
│     - tele ...  │                     ┌──────────────────┐
└─────────────────┘                     │  learning_logs   │
                                        └──────────────────┘
```

## Proto API

### Shared Messages

```protobuf
message EtymologyOriginPart {
  string origin = 1;
  string type = 2;       // "root", "prefix", "suffix"
  string language = 3;
  string meaning = 4;
  int32 word_count = 5;  // Number of definitions using this origin
}

// A definition with its origin breakdown (used in browse and quiz responses)
message EtymologyDefinition {
  string expression = 1;
  string meaning = 2;
  string part_of_speech = 3;
  string note = 4;
  repeated EtymologyOriginPart origin_parts = 5;
  string notebook_name = 6;  // Source notebook (e.g., "Frankenstein")
}

message EtymologyMeaningGroup {
  string meaning = 1;
  repeated EtymologyOriginPart origins = 2;
}

enum EtymologyQuizMode {
  ETYMOLOGY_QUIZ_MODE_UNSPECIFIED = 0;
  ETYMOLOGY_QUIZ_MODE_BREAKDOWN = 1;
  ETYMOLOGY_QUIZ_MODE_ASSEMBLY = 2;
}

message EtymologyOriginAnswer {
  string origin = 1;
  string meaning = 2;
}

message EtymologyOriginGrade {
  string user_origin = 1;
  string user_meaning = 2;
  bool origin_correct = 3;
  bool meaning_correct = 4;
  EtymologyOriginPart correct_origin = 5;
}
```

### NotebookService: Browse

```protobuf
service NotebookService {
  // ... existing RPCs ...
  rpc GetEtymologyNotebook(GetEtymologyNotebookRequest)
    returns (GetEtymologyNotebookResponse);
}

message GetEtymologyNotebookRequest {
  string notebook_id = 1 [(buf.validate.field).string.min_len = 1];
}

message GetEtymologyNotebookResponse {
  string notebook_id = 1;
  string name = 2;
  repeated EtymologyOriginPart origins = 3;
  repeated EtymologyDefinition definitions = 4;  // From all notebooks
  repeated EtymologyMeaningGroup meaning_groups = 5;
  int32 total_origin_count = 6;
  int32 total_definition_count = 7;
}
```

The `definitions` field returns all definitions (from any notebook) whose `origin_parts` reference origins in this etymology notebook. The `meaning_groups` field pre-computes origin grouping by meaning for the "By Meaning" tab.

### QuizService: Structured Quiz

```protobuf
service QuizService {
  // ... existing RPCs ...
  rpc StartEtymologyQuiz(StartEtymologyQuizRequest)
    returns (StartEtymologyQuizResponse);
  rpc SubmitEtymologyBreakdownAnswer(SubmitEtymologyBreakdownAnswerRequest)
    returns (SubmitEtymologyBreakdownAnswerResponse);
  rpc SubmitEtymologyAssemblyAnswer(SubmitEtymologyAssemblyAnswerRequest)
    returns (SubmitEtymologyAssemblyAnswerResponse);
}

message StartEtymologyQuizRequest {
  // Etymology notebooks to source origins from
  repeated string etymology_notebook_ids = 1 [(buf.validate.field).repeated.min_items = 1];
  // Definition notebooks to source words from (book, flashcard, story)
  repeated string definition_notebook_ids = 2 [(buf.validate.field).repeated.min_items = 1];
  EtymologyQuizMode mode = 3;
  bool include_unstudied = 4;
}

message StartEtymologyQuizResponse {
  repeated EtymologyQuizCard cards = 1;
}

message EtymologyQuizCard {
  int64 card_id = 1;
  string expression = 2;
  string meaning = 3;
  repeated EtymologyOriginPart origin_parts = 4;
  string notebook_name = 5;
}

message SubmitEtymologyBreakdownAnswerRequest {
  int64 card_id = 1 [(buf.validate.field).int64.gt = 0];
  repeated EtymologyOriginAnswer answers = 2;
  int64 response_time_ms = 3;
}

message SubmitEtymologyBreakdownAnswerResponse {
  bool correct = 1;
  string reason = 2;
  repeated EtymologyOriginGrade origin_grades = 3;
  repeated EtymologyDefinition related_definitions = 4;
  string next_review_date = 5;
  string learned_at = 6;
  int64 note_id = 7;
}

message SubmitEtymologyAssemblyAnswerRequest {
  int64 card_id = 1 [(buf.validate.field).int64.gt = 0];
  string answer = 2 [(buf.validate.field).string.min_len = 1];
  int64 response_time_ms = 3;
}

message SubmitEtymologyAssemblyAnswerResponse {
  bool correct = 1;
  string reason = 2;
  string correct_expression = 3;
  repeated EtymologyOriginPart origin_parts = 4;
  repeated EtymologyDefinition related_definitions = 5;
  string next_review_date = 6;
  string learned_at = 7;
  int64 note_id = 8;
}
```

### QuizService: Freeform Quiz

```protobuf
service QuizService {
  // ... existing RPCs ...
  rpc StartEtymologyFreeformQuiz(StartEtymologyFreeformQuizRequest)
    returns (StartEtymologyFreeformQuizResponse);
  rpc SubmitEtymologyFreeformAnswer(SubmitEtymologyFreeformAnswerRequest)
    returns (SubmitEtymologyFreeformAnswerResponse);
}

message StartEtymologyFreeformQuizRequest {
  // Etymology notebooks to source origins from
  repeated string etymology_notebook_ids = 1 [(buf.validate.field).repeated.min_items = 1];
  // Definition notebooks to source words from (book, flashcard, story)
  repeated string definition_notebook_ids = 2 [(buf.validate.field).repeated.min_items = 1];
}

message StartEtymologyFreeformQuizResponse {
  repeated string expressions = 1;
  map<string, string> next_review_dates = 2;
}

message SubmitEtymologyFreeformAnswerRequest {
  string expression = 1 [(buf.validate.field).string.min_len = 1];
  repeated EtymologyOriginAnswer answers = 2;
  int64 response_time_ms = 3;
}

message SubmitEtymologyFreeformAnswerResponse {
  bool correct = 1;
  string reason = 2;
  repeated EtymologyOriginGrade origin_grades = 3;
  repeated EtymologyDefinition related_definitions = 4;
  string next_review_date = 5;
  string learned_at = 6;
  string notebook_name = 7;
  int64 note_id = 8;
}
```

## OpenAI Grading

### Why OpenAI is Needed

Simple string matching is insufficient for grading etymology breakdown answers because:

1. **Spelling variations**: User might type "tele" or "tel" or "telos" — all referring to the same Greek origin
2. **Meaning equivalence**: "far" vs. "distant" vs. "remote" are all valid meanings for `tele`
3. **Origin name variations**: "phone" vs. "phon" vs. "phonos" may all be acceptable
4. **Partial credit**: User might get 2 out of 3 origins correct — quality score should reflect this

### Breakdown Mode Grading

A new OpenAI grading method grades the user's origin breakdown of a word. The prompt instructs the model to:

1. Match each user-provided origin to expected origins, allowing spelling variations of the same morpheme
2. Check if the user's meaning is semantically equivalent to the expected meaning (generous with synonyms)
3. Mark an origin as correct only if both the morpheme and meaning match
4. Mark extra or missing origins as incorrect
5. Assess overall correctness (all expected origins matched, no extra incorrect ones) and quality (1-5) based on accuracy and response time

The response includes per-origin grading so the UI can show which origins were correct/incorrect.

### Assembly Mode Grading

Assembly mode (origins → word) reuses the existing `ValidateWordForm` method from the reverse quiz. Spelling variations and word forms are handled the same way.

### Freeform Mode Grading

Freeform mode reuses Breakdown grading. The server looks up the expression across notebooks that have `origin_parts`, finds the expected origins, and grades against them.
