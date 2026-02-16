---
title: "Backend Design"
weight: 3
---

# Backend Design

## Overview

This document describes the Go backend changes to support MySQL storage for notes, learning logs, and dictionary entries while keeping notebooks in YAML.

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/jmoiron/sqlx` | Lightweight ORM - extends `database/sql` with struct scanning and named queries |
| `github.com/go-sql-driver/mysql` | MySQL driver |
| `github.com/golang-migrate/migrate/v4` | Schema migration management |

## Package Structure

```
schemas/
├── migrations/                      # SQL migration files (outside internal/)
│   ├── 001_create_tables.up.sql
│   ├── 001_create_tables.down.sql
│   └── ...
internal/
├── database/
│   └── db.go                        # Connection setup + migration runner
├── story/
│   └── repository.go                # Story models + StoryRepository interface (YAML only)
├── flashcard/
│   └── repository.go                # Flashcard models + FlashcardRepository interface (YAML only)
├── book/
│   └── repository.go                # Book models + BookRepository interface (YAML only)
├── note/
│   └── repository.go                # Note models + NoteRepository interface + YAML/DB implementations
├── learning/
│   └── repository.go                # LearningLog model + LearningRepository interface + YAML/DB implementations
├── dictionary/
│   └── repository.go                # DictionaryEntry model + DictionaryRepository interface + YAML/DB implementations
├── datasync/
│   └── datasync.go                  # Import/export orchestration (reads YAML repos ↔ DB repos)
├── cli/                             # Modified to use repository interfaces
├── config/                          # Add DatabaseConfig
└── ...
```

## Models

Each domain package defines its own models for internal business logic. Models don't need to map 1:1 to tables or YAML files — they should be shaped for how consumers use them. The YAML and DB implementations handle mapping between storage format and domain models.

### story/

- `StoryNotebook` — event, metadata (series, season, episode), scenes
- `StoryScene` — title, conversations, notes
- `Conversation` — character, quote

### flashcard/

- `FlashcardNotebook` — title, description, cards
- `FlashcardCard` — expression, definition, meaning, pronunciation, part_of_speech

### book/

- `BookNotebook` — event, scenes
- `BookScene` — scene, type, statements

### note/

- `Note` — usage, entry, meaning, level, dictionary_number, images, references, notebook_notes

### learning/

- `LearningLog` — note reference, status, learned_at, quality, response_time_ms, quiz_type, interval_days, easiness_factor

### dictionary/

- `DictionaryEntry` — word, source_type, source_url, response (JSON)

## Repository Interfaces

Each domain package defines a repository interface. Notebook repositories are YAML-only since notebooks stay in YAML files. Repositories for database-backed data have two implementations (YAML and DB).

### StoryRepository

YAML only — reads from `notebooks/stories/` directories.

### FlashcardRepository

YAML only — reads from `notebooks/flashcards/` directories.

### BookRepository

YAML only — reads from `notebooks/books/` directories.

### NoteRepository

- `YAMLNoteRepository` — reads notes from story/flashcard YAML notebooks and book definition files. Maps YAML fields (`expression` → `Usage`, `definition` → `Entry`) to the domain model.
- `DBNoteRepository` — reads/writes from MySQL using `sqlx`. Manages notes, note_images, note_references, and notebook_notes tables.

### LearningRepository

- `YAMLLearningRepository` — reads from `learning_notes/*.yml` files
- `DBLearningRepository` — reads/writes from MySQL using `sqlx`

### DictionaryRepository

- `YAMLDictionaryRepository` — reads from `dictionaries/rapidapi/*.json` files
- `DBDictionaryRepository` — reads/writes from MySQL using `sqlx`

## How Commands Use Repositories

### Import Command (`langner db import`)

Orchestrated by `internal/datasync/`. Reads from YAML implementations, writes to DB implementations:

```
YAMLNoteRepository.FindAll()        ──→  DBNoteRepository.Create()
YAMLLearningRepository.FindAll()    ──→  DBLearningRepository.Create()
YAMLDictionaryRepository.FindAll()  ──→  DBDictionaryRepository.Upsert()
```

### Export Command (`langner db export`)

Reads from DB repositories, writes to YAML files.

### Quiz Commands

Currently:
```
Reader (YAML notebooks) → LearningHistories (YAML) → Quiz Session → WriteYamlFile()
```

After:
```
StoryRepository/FlashcardRepository (YAML) → LearningRepository (DB) → Quiz Session → LearningRepository.Create()
```

`StoryRepository`, `FlashcardRepository`, and `BookRepository` replace the existing `Reader` for loading notebook content. Only learning data and notes move to the database.

### Dictionary Lookup

Currently:
```
rapidapi.FileCache (JSON files) → Reader
```

After:
```
DictionaryRepository (DB) → fallback to API call → DictionaryRepository.Upsert()
```

## Configuration

Add `DatabaseConfig` to `internal/config/config.go` with host, port, database, username, and password fields.

## Schema Migrations

SQL migration files live at `schemas/migrations/`. Use `golang-migrate/migrate` CLI to run them.

Migrations are run manually via the `migrate` CLI or `make db-migrate`.

## Testing

- Repository interfaces enable mocking with `go.uber.org/mock` for CLI and importer tests
- Integration tests for DB repository implementations use a test MySQL database
