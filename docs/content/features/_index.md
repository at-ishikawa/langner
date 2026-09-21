---
title: "Features"
weight: 2
bookCollapseSection: true
---

# Features

## Study Materials Generation

Generate formatted study guides (markdown/PDF) from your notebooks with word definitions, pronunciations, examples, and synonyms.

## Vocabulary Quizzes

Interactive quizzes to test your knowledge. Two modes available:
- **Notebook quiz** - Shows word, you provide the meaning
- **Freeform quiz** - Recall both word and meaning from context

## Spaced Repetition

Langner uses [spaced repetition]({{< relref "spaced-repetition" >}}) to optimize learning. Words are reviewed at increasing intervals based on how well you know them.

## Dictionary Integration

Look up word definitions using WordsAPI. Results are cached locally and automatically merged into your study materials.

## Notebook Validation

Check notebooks for errors and inconsistencies. Auto-fix available for common issues.

## Command Reference

Quizzes and study materials live in the web app (`langner-server`). The CLI is
split into two binaries: `langner` for end users and `langner-admin` for
schema/data and account administration.

| Command | Description |
|---------|-------------|
| `langner validate` | Check notebooks for errors (auto-fix with `--fix`) |
| `langner ebook clone/list/remove` | Manage cloned Standard Ebooks repositories |
| `langner-admin migrate import-db` | Import notebook data into the database |
| `langner-admin migrate schema/rollback` | Apply or roll back schema migrations |
| `langner-admin auth provision` | Upsert allowlist/admin accounts and notebook ownership |
| `langner-admin notebooks set-owner` | Set a notebook's visibility and owner |
