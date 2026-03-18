---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

Add etymology-based word learning pages and quizzes to Langner. Words are organized by their etymological origins — roots, prefixes, suffixes, and combining forms from any language. Users can browse origins, see all words built from a given origin, navigate between related origins, and discover origins with similar meanings across different languages. A new quiz mode tests knowledge of word-to-origin connections.

## Problem

### Current Notebook Format Does Not Fit Etymology Data

The existing flashcard notebook format stores flat word lists. Etymology-based learning has a fundamentally different structure: words are organized around origins, each word is composed of multiple etymological parts, and origins connect to each other through shared words. The current format cannot represent these relationships.

### No Way to Navigate Between Related Etymologies

A key insight of etymology-based learning is that origins connect to each other through words. For example, "telephone" connects `tele` (far) to `phone` (sound/voice). When studying `tele`, you want to quickly jump to `phone` to see its word family — "microphone", "phonetic", "symphony" — and from `phone` discover `micro` (small), leading to "microscope", "microbe". Currently there is no way to follow these connections.

### No Way to Discover Related Meanings Across Languages

Different languages often have origins with similar meanings. For example, both Greek `graphein` and Latin `scribere` mean "to write", producing different word families ("autograph", "graphic" vs. "describe", "script", "prescribe"). Learners benefit from seeing these parallel origins side by side, but there is currently no way to browse origins by meaning.

### No Etymology-Specific Quiz Mode

The existing quiz modes (Standard, Reverse, Freeform) test word-to-meaning recall. They do not test the user's understanding of etymological composition — e.g., given "bicycle", can the user identify that it is built from `bi` (two) + `kyklos` (circle/wheel)?

## Goals

- Define a data model for etymology notebooks that captures origins, their types, languages, and the words built from them
- Provide browsable etymology pages where users can see all words under an origin and navigate to related origins
- Allow browsing origins by meaning to discover related origins across different languages
- Support an etymology quiz mode that tests knowledge of word composition, with spaced repetition (SM-2) to schedule reviews
- Integrate etymology notebooks into the existing notebook list and learning history system

## Data Model

### Core Concepts

An **origin** is an etymological building block — a root, prefix, suffix, or combining form. Each origin has a meaning, language, and type. Origins can come from any language (Greek, Latin, Old English, French, etc.).

An origin is uniquely identified by the combination of its **origin** (the original word/morpheme) and **language**. The same spelling can appear in different languages with different meanings — for example, `pan` (Greek, "all") vs. `pan` (Old English, "a cooking vessel"). The origin + language pair ensures these are treated as distinct origins.

**Origins and definitions are separate.** Origins are defined in etymology notebooks. Definitions (vocabulary entries) live in existing notebooks — books, flashcards, stories — and reference origins via `origin_parts`. This means:

- A word like "telephone" might be a definition in a book notebook, with `origin_parts` linking it to `tele` and `phone` from an etymology notebook
- The same origins can be referenced by definitions across different notebooks
- Etymology quizzes can pull words from any notebook type, as long as the definition has `origin_parts`

A definition can have any number of origin parts — there is no fixed number of slots. For example:

- "telephone" has two origin parts: `tele` (Greek) + `phone` (Greek)
- "bicycle" has two origin parts: `bi` (Latin) + `kyklos` (Greek)
- "international" has three origin parts: `inter` (Latin) + `natio` (Latin) + `al` (Latin)

Related word forms (e.g., noun "bicycle", verb "cycle", adjective "cyclical") are separate definitions that share some of the same origin parts. Each form is browsable and quizzable on its own.

### Data Format

**Etymology notebook** — uses the existing `Index` structure with `kind: Etymology`. Session files contain a flat list of origins.

```yaml
# index.yml
id: greek-latin-roots
kind: Etymology
name: "Greek & Latin Roots"
notebooks:
  - ./session1.yml
  - ./session2.yml
```

```yaml
# session1.yml — flat list of origins
- origin: tele
  type: prefix
  language: Greek
  meaning: far

- origin: phone
  type: root
  language: Greek
  meaning: "sound, voice"

- origin: micro
  type: prefix
  language: Greek
  meaning: small

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

**Definitions in existing notebooks** — definitions in book, flashcard, or story notebooks gain an optional `origin_parts` field that links to origins defined in etymology notebooks.

```yaml
# In a book or flashcard notebook
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

Note: `origin_parts` is distinct from the existing `origin` field on definitions. The `origin` field stores a free-text etymology description (e.g., "from Latin describere"). The `origin_parts` field stores structured references to origins defined in etymology notebooks.

**Key design decisions:**

- **Origins and definitions are separated.** Origins live in etymology notebooks; definitions live in their source notebooks (book, flashcard, story). This avoids duplicating vocabulary data and lets etymology quizzes pull words from any notebook.
- **Etymology notebooks follow the existing `Index` structure.** They use `kind: Etymology` and session files, just like story and book notebooks use their own `kind` values.
- **Origins are identified by `origin` + `language`.** This disambiguates origins that share the same spelling but come from different languages.
- **`origin_parts` is a new optional field on definitions.** Any definition in any notebook type can reference origins. Definitions without `origin_parts` are unaffected.
- **A definition can have any number of origin parts.** The order reflects how the word is composed.

### How Navigation Works

The data model enables navigation between origins through shared definitions across all notebooks:

1. **Origin → Definitions**: Each origin page lists all definitions (from any notebook) whose `origin_parts` reference that origin.
2. **Definition → Origins**: Each definition shows its origin parts in order, and each origin links to the origin's page.
3. **Origin → Related Origins**: From an origin page, you see its definitions, and each definition's other origin parts link to other origins. This creates a navigable graph.
4. **Meaning → Origins**: Users can browse or search origins by meaning. For example, searching "to write" shows both `graphein` (Greek) and `scribere` (Latin), letting the user explore both word families side by side.

Example navigation path:
- Start at `phone` (sound) → see "telephone", "microphone", "phonetic" (from various notebooks)
- Click `tele` in "telephone" → see `tele` (far) → see "telephone", "telescope", "television"
- Click `scope` in "telescope" → see `scope` (to look at) → see "telescope", "microscope", "stethoscope"
- Click `micro` in "microscope" → see `micro` (small) → see "microscope", "microphone", "microbe"
- Back to `phone` through "microphone"

### Discovering Related Meanings Across Languages

Origins from different languages often share similar meanings. The origin list page supports browsing by meaning, which groups origins with the same or similar meanings together:

| Meaning | Origins |
|---------|---------|
| to write | `graphein` (Greek), `scribere` (Latin) |
| small | `micro` (Greek), `minus` (Latin) |
| star | `aster` (Greek), `stella` (Latin) |
| water | `hydor` (Greek), `aqua` (Latin) |

Clicking any origin in this view navigates to its origin detail page with all associated definitions. This lets learners compare how different languages contributed to English vocabulary for the same concept.

## User Stories

### Browse Etymologies

#### View Etymology List

As a user, I want to see all origins from an etymology notebook.

- Open an etymology notebook
- See a list of all origins with their type (root/prefix/suffix), language, and meaning
- Each origin shows the count of words that use it
- Origins can be searched by origin or meaning text

#### Browse Origins by Meaning

As a user, I want to find origins that share similar meanings across different languages.

- View origins grouped by meaning (e.g., "to write" shows `graphein` from Greek and `scribere` from Latin)
- Click any origin to see its word family
- Compare word families from different language origins with the same meaning

#### View Origin Detail

As a user, I want to see all words built from a specific origin.

- Click an origin (e.g., `phone` — Greek, "sound, voice") to see its detail page
- See the origin's type, language, and meaning
- See all words that include this origin, with:
  - The full origin breakdown shown visually (e.g., `tele` + `phone` → telephone)
  - Word meaning
  - Part of speech if available
  - Notes if available

#### Navigate Between Origins

As a user, when viewing words under one origin, I want to jump to another origin that appears in those words.

- Each origin shown in a word's breakdown is a clickable link to that origin's page
- Example: viewing `phone` (sound), clicking `tele` in "telephone" navigates to the `tele` page showing "far" and all words using `tele`
- A breadcrumb or back navigation lets users retrace their path

### Etymology Quiz

#### Start Etymology Quiz

As a user, I want to quiz myself on word composition.

- Select notebooks in two categories:
  - **Etymology notebooks** — which origins to quiz on (e.g., "Greek & Latin Roots")
  - **Definition notebooks** — which books/flashcards to pull words from (e.g., "Frankenstein", "English Vocabulary")
- Choose quiz mode:
  - **Breakdown**: Given a word, type its etymological origins and their meanings
  - **Assembly**: Given etymological origins with their meanings, type the word
  - **Freeform**: Open-ended — type any word and identify its origins
- The quiz includes definitions from the selected definition notebooks that have `origin_parts` matching origins in the selected etymology notebooks
- For Breakdown and Assembly modes: see how many words are due for review (based on SM-2 spaced repetition schedule) and optionally include unstudied words
- For Freeform mode: no word count — the quiz continues until the user chooses to stop

#### Answer an Etymology Question (Breakdown Mode)

As a user, I want to identify the etymological origins of a word.

- A word is shown (e.g., "telescope") along with its meaning
- The user types each origin and its meaning (e.g., "tele = far, scope = to look at")
- Submit and see feedback

#### Answer an Etymology Question (Assembly Mode)

As a user, I want to identify a word from its etymological origins.

- The origins are shown with their meanings (e.g., `tele` = far + `scope` = to look at)
- The user types the word
- Submit and see feedback

#### Get Feedback

As a user, I want to see if my etymology breakdown/assembly was correct.

- See correct/incorrect indicator
- See the full correct breakdown with each origin, its meaning, and language
- See related words that share the same origins (encourages exploration)
- See the next review date (calculated by SM-2 based on the answer quality)
- Proceed to the next word

#### Freeform Etymology Quiz

As a user, I want to type any word and test my knowledge of its etymology in an open-ended format.

- The user selects etymology and definition notebooks (same as structured quiz) but there is no fixed word count — the quiz is open-ended (same pattern as the existing Freeform quiz)
- The user types a word; the system checks whether it exists in the selected etymology notebooks
- If found, the user types the origins and their meanings (same input as Breakdown mode)
- Submit and see feedback with the correct breakdown, related words, and next review date
- The user can continue with another word or see results at any time

#### Complete an Etymology Quiz

As a user, I want to see a summary of my etymology quiz results.

- See number of words practiced
- See correct and incorrect counts
- Each result shows the word with its full etymology breakdown
- Each result card supports Override, Skip/Resume, and Change Review Date actions (same as existing quiz result cards)

### Notebook Management

#### View Etymology Notebooks in Notebook List

As a user, I want to see etymology notebooks alongside story and flashcard notebooks.

- The `/notebooks` page shows etymology notebooks as a separate category
- Each etymology notebook shows its title and number of origins
- Each etymology notebook also shows the count of definitions (from all other notebooks) that reference its origins via `origin_parts`

## Out of Scope

- Automatic etymology lookup from external dictionaries
- Creating or editing etymology notebooks through the UI (YAML files are edited manually)
- Merging etymologies across different notebooks (each notebook is self-contained)
- Audio pronunciation of etymological origins
