---
title: "Technical Design"
weight: 1
---

# Technical Design

## Overview

### Goals

- Store learning data (notes, learning logs, dictionary cache) in MySQL
- Keep notebooks (story, flashcard) in YAML for easy authoring
- Remove `{{ }}` markers from notebook YAML - use `usage` field for text matching
- Keep configuration in YAML files

### Non-Goals

- Migrate notebook content to database
- Change the spaced repetition algorithm
- Modify the CLI interface

## Hybrid Storage Approach

| Data | Storage | Reason |
|------|---------|--------|
| Story/Flashcard/Book notebooks | YAML | Easy to author and edit |
| Book definitions | YAML | Manually authored vocabulary for books |
| Ebook repositories config | YAML | Book catalog (books.yml) |
| Notes (vocabulary) | Database | Query and link to learning logs |
| Learning logs | Database | Transactional data, efficient queries |
| Dictionary cache | Database | API response cache |
| Configuration | YAML | Simple, file-based config |

### Removing `{{ }}` Markers

Currently, notebooks use `{{ }}` markers to indicate words to learn:

```yaml
# Before (current)
quote: "There's {{ nothing }} to tell"
```

With database storage, the `usage` field in the `notes` table identifies the word. The application matches `usage` against the conversation text at runtime for highlighting:

```yaml
# After (no markers)
quote: "There's nothing to tell"
```

## Terminology

| Term | Description | Example |
|------|-------------|---------|
| **usage** | The word/phrase as used in the notebook | "running", "kicked the bucket" |
| **entry** | The dictionary entry to look up | "run", "kick the bucket" |
| **meaning** | What the word/phrase means | "to move fast", "to die (idiom)" |

## Data Model

### Entity Relationship

```
YAML Files                              Database
──────────────────                      ──────────────────

┌─────────────────┐                     ┌────────────────┐
│ Story Notebook  │ ──────────────────> │ NotebookNote   │
│ (YAML)          │                     │ (type="story") │
└─────────────────┘                     └────────────────┘
                                               │
┌─────────────────┐                            │
│Flashcard Notebook│ ────────────────> ┌────────────────┐
│ (YAML)          │                    │ NotebookNote   │
└─────────────────┘                    │(type="flashcard")
                                       └────────────────┘
┌─────────────────┐                            │
│ Book Notebook   │ ─────────────────> ┌────────────────┐
│ (YAML)          │                    │ NotebookNote   │
└─────────────────┘                    │ (type="book")  │
                                       └────────────────┘
                                               │
                                               ▼
                                        ┌─────────────┐
                                        │    Note     │
                                        └─────────────┘
                                               │
                              ┌────────────────┼────────────────┐
                              │                │                │
                              ▼                ▼                ▼
                       ┌─────────────┐  ┌────────────┐  ┌───────────────┐
                       │ LearningLog │  │ NoteImage  │  │ NoteReference │
                       └─────────────┘  └────────────┘  └───────────────┘

                                        ┌─────────────────┐
                                        │ DictionaryEntry │
                                        └─────────────────┘
                                               ▲
                                               │ entry
                                               │
                                        ┌─────────────┐
                                        │    Note     │
                                        └─────────────┘
```

### How Notes Link to YAML Content

Notes are linked to notebook content via the `usage` field:

1. **Story notebooks**: The `usage` value matches text in conversation quotes
2. **Flashcard notebooks**: Each card has a corresponding note by `usage`
3. **Book notebooks**: Definitions files provide notes that match text in book statements

Example flow:
```
YAML: quote: "He was running fast"
           ↓
Database: notes.usage = "running", notes.entry = "run"
           ↓
App matches "running" in quote text for highlighting
```

### Field Ownership

Fields are either **user-provided** (stored in notes) or **dictionary-provided** (fetched from dictionary_entries via `entry`):

| Field | Source | Description |
|-------|--------|-------------|
| usage | User | Word/phrase as used in notebook |
| entry | User | Dictionary entry to look up |
| meaning | User | User-provided meaning (overrides dictionary) |
| level | User | Expression level (new, unusable) |
| dictionary_number | User | Which dictionary result to use (1-indexed) |
| images | User | Links to images for visual learning |
| references | User | External links |
| pronunciation | Dictionary | IPA pronunciation |
| part_of_speech | Dictionary | Grammatical category |
| examples | Dictionary | Usage examples |
| synonyms | Dictionary | Related words |
| antonyms | Dictionary | Opposite words |
| origin | Dictionary | Etymology |

### Tables

#### notes

Represents a vocabulary word or phrase.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | BIGINT | PRIMARY KEY AUTO_INCREMENT | Internal ID |
| usage | VARCHAR(255) | NOT NULL | Word/phrase as used in notebook (e.g., "running") |
| entry | VARCHAR(255) | NOT NULL | Dictionary entry to look up (e.g., "run") |
| meaning | TEXT | | User-provided meaning (overrides dictionary) |
| level | VARCHAR(50) | | Expression level: "new", "unusable" |
| dictionary_number | INT | | Which dictionary result to use (1-indexed) |
| created_at | TIMESTAMP | NOT NULL | Creation timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |

Note: `entry` defaults to `usage` if not specified. Dictionary data is fetched from `dictionary_entries` using the `entry` field.

#### notebook_notes

Links notes to their source notebooks. A note can appear in multiple notebooks.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | BIGINT | PRIMARY KEY AUTO_INCREMENT | Internal ID |
| note_id | BIGINT | FOREIGN KEY → notes | Reference to notes |
| notebook_type | VARCHAR(50) | NOT NULL | "story", "flashcard", or "book" |
| notebook_id | VARCHAR(255) | NOT NULL | Index ID (e.g., "friends", "vocabulary", "frankenstein") |
| group | VARCHAR(255) | | Episode, chapter, or deck name (e.g., "Friends S01E01", "Letter 1") |
| subgroup | VARCHAR(255) | | Scene or section within group (e.g., "In the CENTRAL PERK", "Paragraph 1") |
| created_at | TIMESTAMP | NOT NULL | Creation timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |

**Examples:**

| notebook_type | notebook_id | group | subgroup |
|---------------|-------------|-------|----------|
| story | friends | Friends S01E01 | In the CENTRAL PERK |
| book | frankenstein | Letter 1 | Paragraph 1 |
| flashcard | vocabulary | English Vocabulary | (null) |

#### note_images

User-provided image links for visual vocabulary learning.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | BIGINT | PRIMARY KEY AUTO_INCREMENT | Internal ID |
| note_id | BIGINT | FOREIGN KEY → notes | Reference to notes |
| url | TEXT | NOT NULL | Link to image |
| sort_order | INT | NOT NULL | Order within note |
| created_at | TIMESTAMP | NOT NULL | Creation timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |

#### note_references

User-provided external references (articles, videos, etc.).

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | BIGINT | PRIMARY KEY AUTO_INCREMENT | Internal ID |
| note_id | BIGINT | FOREIGN KEY → notes | Reference to notes |
| link | TEXT | NOT NULL | Reference URL |
| description | TEXT | | Reference description |
| sort_order | INT | NOT NULL | Order within note |
| created_at | TIMESTAMP | NOT NULL | Creation timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |

#### learning_logs

Tracks learning history for each note. Regular and reverse quizzes are stored in the same table, distinguished by `quiz_type`. Each quiz type has independent SM-2 scheduling.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | BIGINT | PRIMARY KEY AUTO_INCREMENT | Internal ID |
| note_id | BIGINT | FOREIGN KEY → notes | Reference to notes |
| status | VARCHAR(50) | NOT NULL | Learning status |
| learned_at | DATETIME | NOT NULL | Timestamp of learning event (RFC3339) |
| quality | INT | | 0-5 performance grade |
| response_time_ms | INT | | User response latency |
| quiz_type | VARCHAR(50) | NOT NULL | "freeform", "notebook", or "reverse" |
| interval_days | INT | | Days until next review |
| easiness_factor | DECIMAL(3,2) | | SM-2 easiness factor at this point |
| created_at | TIMESTAMP | NOT NULL | Creation timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |

**Quiz types and SM-2 scheduling:**
- `freeform` and `notebook` are recognition quizzes (word → meaning) and share the same SM-2 track
- `reverse` is a recall quiz (meaning → word) with an independent SM-2 track

### Learning Status Values

| Status | Description |
|--------|-------------|
| (empty) | New, never reviewed |
| misunderstood | Answered incorrectly |
| understood | Answered correctly |
| usable | Can use actively |
| intuitive | Mastered |

### Dictionary Entries Table

Stores dictionary data from API responses or manual input.

#### dictionary_entries

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| word | VARCHAR(255) | PRIMARY KEY | Lookup word |
| source_type | VARCHAR(50) | NOT NULL | "rapidapi", "manual", etc. |
| source_url | TEXT | | URL to dictionary page for reference |
| response | JSON | | Dictionary data in unified format (see below) |
| created_at | TIMESTAMP | NOT NULL | Creation timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |

### Dictionary Response Format

The `response` JSON column uses a unified format for both API and manual entries:

```json
{
  "frequency": 5.36,
  "pronunciation": "rʌn",
  "syllables": ["run"],
  "results": [
    {
      "definition": "move fast by using one's feet",
      "partOfSpeech": "verb",
      "synonyms": ["race", "rush"],
      "antonyms": ["walk", "stop"],
      "similarTo": ["hurry", "speed"],
      "typeOf": ["locomote", "move"],
      "derivation": ["runner", "running"],
      "examples": ["He ran to catch the bus"]
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| frequency | number | No | Word frequency score |
| pronunciation | string | No | IPA pronunciation |
| syllables | string[] | No | Syllable list |
| results | object[] | Yes | Array of definitions |
| results[].definition | string | Yes | The meaning |
| results[].partOfSpeech | string | No | noun, verb, adjective, etc. |
| results[].synonyms | string[] | No | Related words |
| results[].antonyms | string[] | No | Opposite words |
| results[].similarTo | string[] | No | Similar words |
| results[].typeOf | string[] | No | Hypernyms |
| results[].derivation | string[] | No | Derived words |
| results[].examples | string[] | No | Usage examples |

**Manual entry example** (minimal required fields):

```json
{
  "results": [
    {
      "definition": "to move quickly on foot"
    }
  ]
}
```

**RapidAPI response** is stored as-is since it follows the same format.

## Indexes

```sql
CREATE INDEX idx_notes_usage ON notes(usage);
CREATE INDEX idx_notes_entry ON notes(entry);
CREATE INDEX idx_notebook_notes_note ON notebook_notes(note_id);
CREATE INDEX idx_notebook_notes_notebook ON notebook_notes(notebook_type, notebook_id);
CREATE INDEX idx_learning_logs_note ON learning_logs(note_id, quiz_type);
CREATE INDEX idx_learning_logs_status ON learning_logs(note_id, quiz_type, status);
CREATE INDEX idx_learning_logs_date ON learning_logs(learned_at);
```

## Configuration

Add database configuration to `config.yml`:

```yaml
database:
  driver: mysql
  host: localhost
  port: 3306
  database: langner
  username: langner
  password: ${DB_PASSWORD}  # Use environment variable
```

Environment variable for password:

```bash
export DB_PASSWORD="your-database-password"
```

## Migration Strategy

### Phase 1: Add Database Support

1. Add database connection layer
2. Create tables for notes, learning_logs, dictionary_entries
3. Keep YAML notebooks unchanged

### Phase 2: Migration Command

```bash
# Migrate learning history and notes from YAML to database
langner migrate to-database

# Export database to YAML (for backup)
langner migrate to-yaml
```

### Phase 3: Remove `{{ }}` Markers

1. Update notebooks to remove `{{ }}` markers
2. Application uses `notes.usage` to match text for highlighting

## Implementation Notes

### Text Matching for Highlighting

When rendering notebooks, the application:

1. Loads notes from database
2. For each conversation quote, searches for `notes.usage` in the text
3. Highlights matched words/phrases

```go
func HighlightUsage(quote string, notes []Note) string {
    for _, note := range notes {
        // Case-insensitive match and highlight
        quote = highlightWord(quote, note.Usage)
    }
    return quote
}
```

### Query Optimization

Database enables efficient queries:
- Filter notes by learning status
- Find notes due for review (spaced repetition)
- Search notes by usage or entry
- Aggregate learning statistics

### Field Mapping (YAML to Database)

| YAML Field | Database Field | Notes |
|------------|----------------|-------|
| `expression` | `usage` | Word/phrase as used in notebook |
| `definition` | `entry` | Dictionary entry to look up |
| `meaning` | `meaning` | User-provided meaning |
| `images` | `note_images` | Separate table (links to images) |
| `references` | `note_references` | Separate table |
