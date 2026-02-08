---
title: "Technical Design"
weight: 2
---

# Technical Design

## Overview

This design reuses existing types and patterns from story notebooks, extending them minimally for book support.

## Data Model

### Reusing Existing Types

#### StoryScene Extension

The book format reuses `StoryNotebook`, `StoryScene`, and `Note` types from `internal/notebook/story.go`. We extend `StoryScene` with two new fields:

```go
type StoryScene struct {
    Title         string         `yaml:"scene"`
    Conversations []Conversation `yaml:"conversations,omitempty"`
    Statements    []string       `yaml:"statements,omitempty"`  // NEW: for books
    Type          string         `yaml:"type,omitempty"`        // NEW: "blockquote" for quoted passages
    Definitions   []Note         `yaml:"definitions,omitempty"`
}
```

- **Existing story notebooks** use `Conversations` (speaker + quote pairs)
- **Book notebooks** use `Statements` (plain sentences) and optional `Type`

#### Index (No Changes)

Reuse existing `Index` type as-is. Book-specific metadata (repo path, URLs) is stored in `books.yml`, not in `index.yml`.

```go
type Index struct {
    path          string   `yaml:"-"`
    Kind          string   `yaml:"kind"`
    ID            string   `yaml:"id"`
    Name          string   `yaml:"name"`
    NotebookPaths []string `yaml:"notebooks"`

    Notebooks [][]StoryNotebook `yaml:"-"`
}
```

Example book `index.yml`:
```yaml
kind: book
id: frankenstein
name: "Frankenstein"
notebooks:
  - ./vocabulary.yml
```

The `id` links to the entry in `books.yml` for repo metadata.

### Vocabulary File

Uses existing `StoryNotebook` format with `statements` instead of `conversations`:

```yaml
- event: "Frankenstein - Letter 1"
  date: 2025-02-07T00:00:00Z
  scenes:
    - scene: "Paragraph 1"
      statements:
        - "You will rejoice to hear that no disaster has accompanied the commencement of an enterprise."
      definitions:
        - expression: "commencement"
          meaning: "the beginning or start of something"

    - scene: "Chapter 10 - Monster's Speech"
      type: blockquote
      statements:
        - "I am malicious because I am miserable."
        - "Am I not shunned and hated by all mankind?"
      definitions:
        - expression: "malicious"
          meaning: "intending to do harm"
```

### Learning History

Uses existing `LearningHistory` with `type: book`:

```yaml
- metadata:
    id: frankenstein
    title: "Letter 1 - Paragraph 1"
    type: book
  expressions:
    - expression: commencement
      learned_logs:
        - status: understood
          learned_at: "2025-02-01"
```

## Configuration

### Master Config (`config.yml`)

```yaml
notebooks:
  books_directories:
    - notebooks/books

books:
  repo_directory: ebooks
  repositories_file: books.yml
```

| Setting | Description | Default |
|---------|-------------|---------|
| `notebooks.books_directories` | Directories containing book index.yml files | `["notebooks/books"]` |
| `books.repo_directory` | Directory for cloned Standard Ebooks repos | `ebooks` |
| `books.repositories_file` | Path to cloned repositories config file | `books.yml` |

### Repositories Config (`books.yml`)

Separate file to track cloned repositories:

```yaml
repositories:
  - id: frankenstein
    repo_path: ebooks/mary-shelley_frankenstein
    source_url: https://github.com/standardebooks/mary-shelley_frankenstein
    web_url: https://standardebooks.org/ebooks/mary-shelley/frankenstein
```

Each entry:

| Field | Description |
|-------|-------------|
| `id` | Unique identifier (e.g., `frankenstein`) |
| `repo_path` | Path to cloned repository |
| `source_url` | GitHub URL |
| `web_url` | Standard Ebooks reader URL |

## How Commands Use Config

### `langner ebook clone <url>`

1. Clone repo to `books.repo_directory`
2. Parse `content.opf` to get title/author for display
3. Add entry to `books.repositories_file`
4. Create `notebooks/books/{id}/index.yml` with metadata

### `langner ebook list`

1. Read repositories from `books.repositories_file`
2. For each entry, parse `content.opf` to get title/author
3. Display list with metadata

## Directory Structure

```
.
├── ebooks/                              # Cloned repos (books.repo_directory)
│   └── mary-shelley_frankenstein/
│       └── src/epub/
│           ├── content.opf              # Metadata (title, author)
│           └── text/
│               ├── letter-1.xhtml
│               └── chapter-1.xhtml
│
├── notebooks/
│   ├── books/                           # Book notebooks (notebooks.books_directories)
│   │   └── frankenstein/
│   │       ├── index.yml
│   │       └── vocabulary.yml
│   │
│   └── learning_notes/
│       └── frankenstein.yml
│
├── config.yml                           # Master config
├── books.yml                            # Cloned repositories list
```

## Standard Ebooks Source Format

Standard Ebooks uses EPUB 3 format with a standardized structure:

### Repository Structure

```
mary-shelley_frankenstein/
├── src/epub/
│   ├── content.opf          # Package metadata (OPF format)
│   ├── toc.xhtml            # Table of contents
│   └── text/
│       ├── letter-1.xhtml   # Chapter content (XHTML)
│       ├── chapter-1.xhtml
│       └── ...
└── ...
```

### Metadata (`content.opf`)

```xml
<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" ...>
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Frankenstein</dc:title>
    <dc:creator id="author">Mary Wollstonecraft Shelley</dc:creator>
    ...
  </metadata>
  <manifest>...</manifest>
  <spine>...</spine>
</package>
```

### Content Files (XHTML)

```html
<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body epub:type="bodymatter">
  <section id="letter-1" epub:type="chapter">
    <h2>Letter 1</h2>
    <p>You will rejoice to hear...</p>
    <blockquote>
      <p>Quoted text here...</p>
    </blockquote>
  </section>
</body>
</html>
```

## Parsing EPUB/OPF

Since we clone the git repository (not the `.epub` archive), we parse `content.opf` directly as XML.

### Option 1: Standard library `encoding/xml`

```go
import "encoding/xml"

type Package struct {
    Metadata struct {
        Title   string `xml:"title"`
        Creator string `xml:"creator"`
    } `xml:"metadata"`
}

var pkg Package
xml.Unmarshal(opfData, &pkg)
```

### Option 2: EPUB libraries

Some libraries may support parsing OPF directly:
- [pirmd/epub](https://pkg.go.dev/github.com/pirmd/epub)
- [mathieu-keller/epub-parser](https://github.com/mathieu-keller/epub-parser)
- [taylorskalyo/goreader/epub](https://pkg.go.dev/github.com/taylorskalyo/goreader/epub)

Note: Many EPUB libraries expect `.epub` archives, not raw directories. Verify compatibility before use.

## Clone Command URL Derivation

The `langner ebook clone` command accepts either URL format and derives both:

| Input | Derived |
|-------|---------|
| `https://github.com/standardebooks/mary-shelley_frankenstein` | source_url |
| `https://standardebooks.org/ebooks/mary-shelley/frankenstein` | web_url |

Derivation logic:
```
GitHub URL: https://github.com/standardebooks/{repo_name}
  → repo_name = mary-shelley_frankenstein
  → web_url = https://standardebooks.org/ebooks/mary-shelley/frankenstein

Standard Ebooks URL: https://standardebooks.org/ebooks/{author}/{title}
  → repo_name = author_title (replace / with _)
  → source_url = https://github.com/standardebooks/{repo_name}
```
