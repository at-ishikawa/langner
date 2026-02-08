---
title: "Features"
weight: 2
---

# Features

## Study Materials Generation

Create formatted study materials from your notebooks:

```bash
# Generate markdown output
langner notebooks stories <name>

# Generate both markdown and PDF
langner notebooks stories <name> --pdf
```

This creates a study guide with:

- Original conversations
- Word definitions and pronunciations
- Example sentences
- Synonyms

## Vocabulary Quizzes

Test your knowledge with interactive quizzes:

```bash
# Quiz from a specific notebook (shows word, you provide meaning)
langner quiz notebook

# Freeform quiz (recall both word and meaning)
langner quiz freeform
```

## Spaced Repetition

Langner uses spaced repetition to optimize your learning. Words are reviewed at increasing intervals:

| Correct Answers | Next Review |
|-----------------|-------------|
| 1 | 3 days |
| 2 | 7 days |
| 3 | 14 days |
| 4 | 30 days |
| 5 | 60 days |
| 6 | 90 days |
| 7 | 180 days |
| 8 | 270 days |
| 9 | 365 days |
| 10 | 540 days |
| 11 | 730 days |
| 12+ | 1095 days |

Words you answered incorrectly will always appear in quizzes until you answer them correctly.

## Dictionary Lookup

Look up word definitions using WordsAPI:

```bash
langner dictionary lookup <word>
```

## Notebook Validation

Check your notebooks for errors and inconsistencies:

```bash
# Check for errors
langner validate

# Automatically fix errors
langner validate --fix
```

## Command Reference

| Command | Description | Required Environment Variables |
|---------|-------------|-------------------------------|
| `langner notebooks stories <name>` | Generate study materials | None (uses cached dictionary data) |
| `langner quiz notebook` | Take a vocabulary quiz | `OPENAI_API_KEY` |
| `langner quiz freeform` | Freeform recall quiz | `OPENAI_API_KEY` |
| `langner dictionary lookup <word>` | Look up word definition | `RAPID_API_HOST`, `RAPID_API_KEY` |
| `langner validate` | Check notebooks for errors | None |
