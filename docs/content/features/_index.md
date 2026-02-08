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

| Command | Description |
|---------|-------------|
| `langner notebooks stories <name>` | Generate study materials from stories |
| `langner quiz notebook` | Take a vocabulary quiz |
| `langner quiz freeform` | Freeform recall quiz |
| `langner dictionary lookup <word>` | Look up word definition |
| `langner validate` | Check notebooks for errors |
