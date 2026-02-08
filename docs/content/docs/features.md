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

## Spaced Repetition (Modified SM-2 Algorithm)

Langner uses a modified SM-2 algorithm to optimize vocabulary retention. The algorithm adapts review intervals based on your performance and provides gentler penalties for well-learned words.

### How It Works

Each word has two key metrics:
- **Easiness Factor (EF)**: Starts at 2.5, adjusts based on performance (range: 1.3 to 3.0+)
- **Interval**: Days until next review, grows with consecutive correct answers

### Quality Grades

Your response quality (1-5) is automatically determined by OpenAI based on:
- Whether your answer was correct
- Your response time relative to the meaning's complexity

| Quality | Meaning |
|---------|---------|
| 1 | Wrong answer |
| 3 | Correct but slow (struggled) |
| 4 | Correct at normal speed |
| 5 | Correct and fast (instant recall) |

### Interval Growth (Correct Answers)

For correct answers (quality ≥ 3):

| Review # | Interval Calculation |
|----------|---------------------|
| 1st correct | 1 day |
| 2nd correct | 6 days |
| 3rd+ correct | Previous interval × EF |

Example progression with EF = 2.5:
- Review 1: 1 day
- Review 2: 6 days
- Review 3: 6 × 2.5 = 15 days
- Review 4: 15 × 2.5 = 38 days
- Review 5: 38 × 2.5 = 95 days

### Gentler Penalties (Wrong Answers)

Unlike standard SM-2 which resets to day 1 on any mistake, Langner uses **proportional reduction** based on your learning progress:

| Previous Correct Streak | Interval Reduction | EF Penalty |
|-------------------------|-------------------|------------|
| 1-2 reviews | Reset to 1 day | -0.54 (full) |
| 3-5 reviews | × 0.5 | -0.40 |
| 6-9 reviews | × 0.6 | -0.30 |
| 10+ reviews | × 0.7 | -0.20 |

**Example**: If you have a 90-day interval after 8 correct reviews and get one wrong:
- Standard SM-2: Reset to 1 day (harsh!)
- Langner: 90 × 0.6 = 54 days, EF drops by only 0.30

This preserves your learning progress while still ensuring you review the word sooner.

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
