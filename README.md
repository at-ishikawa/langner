# Langner

[![CI Status](https://github.com/at-ishikawa/langner/actions/workflows/pr.yml/badge.svg)](https://github.com/at-ishikawa/langner/actions/workflows/pr.yml)

A CLI tool to help you learn English by creating interactive vocabulary notebooks from your favorite content (videos, books, articles, etc.).

## What is Langner?

Langner helps you learn English vocabulary in context by:

- Creating study notebooks from your favorite content
- Generating formatted study materials (Markdown and PDF)
- Testing your knowledge with interactive quizzes
- Tracking your learning progress over time

## Quick Start

### 1. Installation

Install using Go:
```bash
go install github.com/at-ishikawa/langner/cmd/langner@latest
```

Make sure `$GOPATH/bin` is in your `PATH` to run the `langner` command.

### 2. Set Up Your Configuration

Copy the example configuration:
```bash
cp config.example.yml config.yml
```

Edit `config.yml` to customize your directories:
- `notebooks.stories_directory`: Where you store your story notebooks
- `notebooks.learning_notes_directory`: Where learning progress is tracked
- `outputs.story_directory`: Where generated study materials are saved

### 3. Create Your First Story Notebook

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

### 4. Set Up Environment Variables (Optional for Advanced Features)

Different commands require different environment variables:

**For Quiz Commands** (`langner quiz notebook`, `langner quiz freeform`):
```bash
export OPENAI_API_KEY="your-openai-api-key"
export OPENAI_MODEL="gpt-4o-mini"  # Optional, defaults to gpt-4o-mini
```

**For Dictionary Lookup** (`langner dictionary lookup`):
```bash
export RAPID_API_HOST="wordsapiv1.p.rapidapi.com"
export RAPID_API_KEY="your-rapidapi-key"
```
Get your RapidAPI key at: https://rapidapi.com/dpventures/api/wordsapi

**Note:** The `notebooks stories` command uses cached dictionary data, so it doesn't require API keys if you already have cached definitions.

## Main Features

### Generate Study Materials

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

### Take Vocabulary Quizzes

Test your knowledge with interactive quizzes using spaced repetition:

```bash
# Quiz from a specific notebook (shows word, you provide meaning)
langner quiz notebook

# Freeform quiz (recall both word and meaning)
langner quiz freeform
```

**Spaced Repetition System:**
Langner uses spaced repetition to optimize your learning. Words are reviewed at increasing intervals:
- After 1st correct answer: review in 3 days
- After 2nd correct answer: review in 7 days
- After 3rd correct answer: review in 14 days
- After 4th correct answer: review in 30 days
- After 5th correct answer: review in 60 days
- After 6th correct answer: review in 90 days
- And so on up to 1,095 days (3 years)

The `quiz notebook` command automatically shows you only words that are due for review based on these intervals. Words you answered incorrectly (marked as "misunderstood") will always appear in quizzes until you answer them correctly.

### Validate Your Notebooks

Check your notebooks for errors and inconsistencies:

```bash
# Check for errors
langner validate

# Automatically fix errors
langner validate --fix
```

## Workflow Example

1. Watch your favorite English show or read an article
2. Create a story notebook with interesting vocabulary
3. Mark words you want to learn with `{{ }}`
4. Generate study materials: `langner notebooks stories <name> --pdf`
5. Review the PDF with definitions and examples
6. Test yourself: `langner quiz notebook`
7. Track your progress in learning notes

## Configuration Options

Your `config.yml` file controls:

- **notebooks**: Directory paths for stories and learning notes
- **dictionaries**: Cache directory for dictionary lookups
- **templates**: Directory for custom markdown templates
- **outputs**: Where to save generated study materials

## Command Reference

| Command | Description | Required Environment Variables |
|---------|-------------|-------------------------------|
| `langner notebooks stories <name>` | Generate study materials from a notebook | None (uses cached dictionary data) |
| `langner quiz notebook <name>` | Take a vocabulary quiz from a specific notebook | `OPENAI_API_KEY` |
| `langner quiz freeform` | Freeform recall quiz | `OPENAI_API_KEY` |
| `langner dictionary lookup <word>` | Look up word definition | `RAPID_API_HOST`, `RAPID_API_KEY` |
| `langner validate` | Check notebooks for errors | None |
| `langner validate --fix` | Auto-fix validation errors | None |

## Tips for Success

1. **Create notebooks regularly**: The more you practice, the better you learn
2. **Use real content**: Learn from content you enjoy (TV shows, books, podcasts)
3. **Mark words in context**: Understanding how words are used helps retention
4. **Review with quizzes**: Regular testing reinforces memory
5. **Track progress**: Use learning notes to see how far you've come

## Getting Help

For detailed help on any command:
```bash
langner [command] --help
```

Enable debug mode for troubleshooting:
```bash
langner --debug [command]
```

## License

This project is licensed under the MIT License. See the LICENSE file for details.
