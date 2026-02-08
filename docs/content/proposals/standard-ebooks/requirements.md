---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

Add support for learning vocabulary from public domain books published by [Standard Ebooks](https://standardebooks.org/).

## Goals

- Clone Standard Ebooks books directly from the CLI
- Track vocabulary with precise location references (paragraph)
- Reuse existing story notebook format with minor extension
- Use the same spaced repetition system as story notebooks

## User Stories

### Clone a Book

As a user, I want to clone a Standard Ebooks book so I can start learning vocabulary from it.

```bash
langner ebook clone https://standardebooks.org/ebooks/mary-shelley/frankenstein
```

This will:
- Clone the book to `ebooks/` directory
- Create an empty notebook ready for adding vocabulary

### List Cloned Books

As a user, I want to see which books I have cloned.

```bash
langner ebook list
```

Output:
```
frankenstein
  Name: Frankenstein
  Author: Mary Shelley
  URL: https://standardebooks.org/ebooks/mary-shelley/frankenstein
```

### Add Vocabulary

As a user, I want to add vocabulary organized by paragraph or blockquote. The format extends story notebooks with a new `statements` field:

```yaml
- event: "Frankenstein - Letter 1"
  date: 2025-02-07T00:00:00Z
  scenes:
    - scene: "Paragraph 1"
      statements:
        - "You will rejoice to hear that no disaster has accompanied the commencement of an enterprise which you have regarded with such evil forebodings."
      definitions:
        - expression: "commencement"
          meaning: "the beginning or start of something"
        - expression: "forebodings"
          meaning: "a feeling that something bad will happen"

    - scene: "Chapter 10 - Monster's Speech"
      type: blockquote
      statements:
        - "I am malicious because I am miserable."
        - "Am I not shunned and hated by all mankind?"
      definitions:
        - expression: "malicious"
          meaning: "intending to do harm"
        - expression: "shunned"
          meaning: "persistently avoided or rejected"
```

The `statements` field replaces `conversations` for ebook content. Use `type: blockquote` for quoted passages.

### Generate Study Materials

As a user, I want to generate study materials from my ebook vocabulary.

```bash
langner notebooks stories frankenstein --pdf
```

Uses the same command as story notebooks.

### Quiz

As a user, I want to quiz myself on ebook vocabulary.

```bash
langner quiz notebook --notebook frankenstein
```

Uses the same command as story notebooks.

## Configuration

Add ebook settings to `config.yml`:

```yaml
notebooks:
  ebooks_directory: ebooks
```

| Setting | Description | Default |
|---------|-------------|---------|
| `notebooks.ebooks_directory` | Directory where ebooks are cloned | `ebooks` |

## Commands

| Command | Description |
|---------|-------------|
| `langner ebook clone <url>` | Clone a Standard Ebooks book |
| `langner ebook list` | List cloned ebooks |

Other commands (`notebooks`, `quiz`) reuse existing story notebook commands.
