---
title: "CLI Commands"
weight: 2
---

# CLI Commands

This document describes the CLI commands for database operations.

## Import Command

Import data from YAML/JSON files into the database.

### Usage

```bash
langner db import [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would be imported without making changes |
| `--update-existing` | Update existing records instead of skipping |

### Examples

```bash
# Import all data (skip duplicates)
langner db import

# Preview what would be imported
langner db import --dry-run

# Update existing records
langner db import --update-existing
```

### Import Sources

The import command scans and imports from all sources:

| Source | Target Table |
|--------|--------------|
| Story notebooks (`notebooks/stories/**/*.yml`) | notes, notebook_notes |
| Flashcard notebooks (`notebooks/flashcards/**/*.yml`) | notes, notebook_notes |
| Book definitions (`definitions_directories/**/*.yml`) | notes, notebook_notes |
| Learning history (`notebooks/learning_notes/*.yml`) | learning_logs |
| Dictionary cache (`dictionaries/rapidapi/*.json`) | dictionary_entries |

### Notes Import

Extracts vocabulary entries from story/flashcard YAML notebooks and book definition files.

**Story/Flashcard notebook mapping:**

| YAML Field | Database Column |
|------------|-----------------|
| expression | usage |
| definition | entry |
| meaning | meaning |
| level | level |
| dictionary_number | dictionary_number |
| images | note_images (separate table) |
| references | note_references (separate table) |

**Book definition mapping:**

Book definitions (`definitions_directories`) provide vocabulary for Standard Ebooks notebooks:

| YAML Field | Database Column |
|------------|-----------------|
| scenes[].expressions[].expression | usage |
| scenes[].expressions[].definition | entry |
| scenes[].expressions[].meaning | meaning |
| scenes[].expressions[].dictionary_number | dictionary_number |

**Duplicate detection:**
- Unique constraint: `(usage, entry)` enforced at the database level
- If exists: skip (or update if `--update-existing`)

**notebook_notes mapping:**

Each note import also creates a `notebook_notes` record linking the note to its source notebook.

For story notebooks:

| Derived From | Database Column |
|---|---|
| - | notebook_type = "story" |
| Index ID (e.g., "friends") | notebook_id |
| Episode name (e.g., "Friends S01E01") | group |
| Scene title (e.g., "In the CENTRAL PERK") | subgroup |

For flashcard notebooks:

| Derived From | Database Column |
|---|---|
| - | notebook_type = "flashcard" |
| Index ID (e.g., "vocabulary") | notebook_id |
| Notebook name (e.g., "English Vocabulary") | group |
| - | subgroup = null |

For book definitions:

| Derived From | Database Column |
|---|---|
| - | notebook_type = "book" |
| Book ID (e.g., "frankenstein") | notebook_id |
| Chapter name (e.g., "Letter 1") | group |
| Scene name (e.g., "Paragraph 1") | subgroup |

**Duplicate detection for notebook_notes:**
- Unique key: `(note_id, notebook_type, notebook_id, group)`
- If exists: skip

### Learning Logs Import

Imports learning history from YAML files. Both regular (`learned_logs`) and reverse (`reverse_logs`) are imported into the same `learning_logs` table, distinguished by `quiz_type`.

**Mapping for regular logs (`learned_logs`):**

| YAML Field | Database Column |
|------------|-----------------|
| expression | → note_id (lookup by usage) |
| learned_logs[].status | status |
| learned_logs[].learned_at | learned_at |
| learned_logs[].quality | quality |
| learned_logs[].response_time_ms | response_time_ms |
| learned_logs[].quiz_type | quiz_type |
| learned_logs[].interval_days | interval_days |
| easiness_factor | easiness_factor |

**Mapping for reverse logs (`reverse_logs`):**

| YAML Field | Database Column |
|------------|-----------------|
| expression | → note_id (lookup by usage) |
| reverse_logs[].status | status |
| reverse_logs[].learned_at | learned_at |
| reverse_logs[].quality | quality |
| reverse_logs[].response_time_ms | response_time_ms |
| - | quiz_type = "reverse" |
| reverse_logs[].interval_days | interval_days |
| reverse_easiness_factor | easiness_factor |

**Note linking:**
1. Find note by matching `usage` field
2. If note not found: skip with warning

**Duplicate detection:**
- Unique key: `(note_id, quiz_type, learned_at)`
- If exists: skip

### Dictionary Import

Imports cached dictionary responses from JSON files.

**Mapping:**

| JSON Field | Database Column |
|------------|-----------------|
| (filename without .json) | word |
| (full content) | response |
| - | source_type = "rapidapi" |
| - | source_url = RapidAPI URL |

**Duplicate detection:**
- Unique key: `word`
- If exists: skip (or update if `--update-existing`)

### Example Output

```
$ langner db import --dry-run

Scanning sources...
  Story notebooks:    150 notes found
  Flashcard notebooks: 50 notes found
  Notebook notes:     200 links found
  Learning history:   500 logs found
  Dictionary cache:   200 entries found

Notes:
  [SKIP] "running" (run) - already exists
  [NEW]  "kicked the bucket" (kick the bucket)
  [NEW]  "nothing" (nothing)

Learning logs:
  [SKIP] "running" 2025-01-15 - already exists
  [NEW]  "running" 2025-02-01
  [WARN] "unknown_word" - note not found, skipping

Dictionary entries:
  [SKIP] "run" - already exists
  [NEW]  "serendipity"

Summary (dry-run):
  Notes:              45 new, 155 skip
  Notebook notes:    200 new, 0 skip
  Learning logs:     100 new, 395 skip, 5 warnings
  Dictionary entries: 50 new, 150 skip
```

## Export Command

Export database to YAML files for backup.

### Usage

```bash
langner db export [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--output` | Output directory (default: `./export`) |

### Examples

```bash
# Export all data to default directory
langner db export

# Export to specific directory
langner db export --output ./backup
```

### Export Output

```
export/
├── notes.yml                 # All notes
├── notebook_notes.yml        # Note-notebook links
├── learning_logs.yml         # All learning logs
└── dictionary_entries.yml    # All dictionary entries
```

