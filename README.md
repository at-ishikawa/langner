# Langner

[![CI Status](https://github.com/at-ishikawa/langner/actions/workflows/pr.yml/badge.svg)](https://github.com/at-ishikawa/langner/actions/workflows/pr.yml)

A vocabulary learning app that helps you learn English words and phrases from stories you enjoy.

## Why Langner?

Most vocabulary apps give you random word lists with no context. Langner takes a different approach:

1. **Learn words from stories you care about** - Create notebooks from books, TV shows, or articles you're actually reading. Words stick better when you learn them in context rather than from isolated flashcards.
2. **Don't waste time on words you already know** - A spaced repetition system tracks what you know and what you don't. Words you've mastered stop showing up, so you spend time only on what needs practice.
3. **Export PDFs to study anywhere** - Generate printable study materials from your notebooks so you can review on any device, even offline.

## Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Go](https://golang.org/doc/install) (1.25+)
- [Node.js](https://nodejs.org/) and [pnpm](https://pnpm.io/installation)

### Setup

```bash
cp config.example.yml config.yml
make setup
```

This starts the database, installs dependencies, and runs migrations.

### Running

```bash
export OPENAI_API_KEY="your-api-key-here"
make dev
```

- **Frontend**: http://localhost:3000
- **Backend**: http://localhost:8080

## Features

### Books

Read books directly in the app. Select any word or phrase in the text to look it up in the dictionary, then save definitions to your notebook for later study.

- Browse your book library
- Read chapters with an interactive reader
- Tap any word to see its definition, pronunciation, examples, synonyms, and antonyms
- Save words to your notebook with one click

### Learn

Browse your vocabulary and etymology notebooks to review what you've been studying.

**Vocabulary** - View all saved words organized by the stories and scenes where you found them. Each word shows its definition, pronunciation, examples, learning status, and next review date.

**Etymology** - Explore word origins (roots, prefixes, suffixes). Browse by origin or by meaning to see how words are related.

### Quiz

Test yourself with several quiz modes, all powered by spaced repetition:

**Vocabulary Quizzes**
- **Standard** - See a word, type its meaning
- **Reverse** - See a meaning and context, type the word
- **Freeform** - Recall any word and its meaning from memory

**Etymology Quizzes**
- **Breakdown** - See a word, identify its origins and their meanings
- **Assembly** - See the origins, type the complete word
- **Freeform** - Recall a word and break down its etymology

After each answer, you get feedback with the correct answer, examples, and pronunciation. You can override results if you think you were marked incorrectly, or skip words you want to exclude from future quizzes.

At the end of a session, a results page shows your score and lets you review incorrect answers.

### Export PDF

From any notebook page, export a formatted PDF with all your words, definitions, examples, and pronunciations. Useful for offline review or printing.

## Configuration

Edit `config.yml` to set your directories for notebooks, dictionaries, templates, and outputs. See `config.example.yml` for all available options.

### Environment Variables

| Variable | Required For | Description |
|----------|-------------|-------------|
| `OPENAI_API_KEY` | Quizzes | OpenAI API key for quiz answer evaluation |
| `OPENAI_MODEL` | Quizzes (optional) | Model to use, defaults to `gpt-4o-mini` |
| `RAPID_API_HOST` | Dictionary lookup | Set to `wordsapiv1.p.rapidapi.com` |
| `RAPID_API_KEY` | Dictionary lookup | Get at [RapidAPI](https://rapidapi.com/dpventures/api/wordsapi) |

## License

This project is licensed under the MIT License. See the LICENSE file for details.
