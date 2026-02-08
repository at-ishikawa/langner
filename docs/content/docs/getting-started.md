---
title: "Getting Started"
weight: 1
---

# Getting Started

## Installation

Install using Go:

```bash
go install github.com/at-ishikawa/langner/cmd/langner@latest
```

Make sure `$GOPATH/bin` is in your `PATH` to run the `langner` command.

## Configuration

Copy the example configuration:

```bash
cp config.example.yml config.yml
```

Edit `config.yml` to customize your directories:

- `notebooks.stories_directory`: Where you store your story notebooks
- `notebooks.learning_notes_directory`: Where learning progress is tracked
- `outputs.story_directory`: Where generated study materials are saved

## Create Your First Story Notebook

Create a YAML file in your stories directory (e.g., `stories/daily-conversation/meeting.yml`):

```yaml
- event: 'Meeting a friend at a coffee shop'
  date: 2025-11-08T00:00:00Z
  scenes:
    - scene: At the coffee shop
      conversations:
        - speaker: Alice
          quote: I'm {{ excited }} about the new project
        - speaker: Bob
          quote: Me too! We should {{ discuss }} the details
      definitions:
        - expression: excited
          meaning: Feeling enthusiastic and eager
        - expression: discuss
          meaning: To talk about something with someone
```

Mark words you want to learn with `{{ }}` in the quotes.

## Environment Variables

Different commands require different environment variables:

### For Quiz Commands

```bash
export OPENAI_API_KEY="your-openai-api-key"
export OPENAI_MODEL="gpt-4o-mini"  # Optional, defaults to gpt-4o-mini
```

### For Dictionary Lookup

```bash
export RAPID_API_HOST="wordsapiv1.p.rapidapi.com"
export RAPID_API_KEY="your-rapidapi-key"
```

Get your RapidAPI key at: https://rapidapi.com/dpventures/api/wordsapi

## Next Steps

- Generate study materials: `langner notebooks stories <name>`
- Take a quiz: `langner quiz notebook`
- Look up words: `langner dictionary lookup <word>`
