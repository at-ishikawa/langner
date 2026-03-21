---
title: "Spaced Repetition"
weight: 3
---

# Spaced Repetition (Modified SM-2 Algorithm)

Langner uses a modified SM-2 algorithm to optimize vocabulary retention. The algorithm adapts review intervals based on your performance and provides gentler penalties for well-learned words.

## How It Works

Each word has two key metrics:
- **Easiness Factor (EF)**: Starts at 2.5, adjusts based on performance (range: 1.3 to 3.0+)
- **Interval**: Days until next review, grows with consecutive correct answers

## Quality Grades

Your response quality (1-5) is automatically determined by OpenAI based on:
- Whether your answer was correct
- Your response time relative to the meaning's complexity

| Quality | Meaning |
|---------|---------|
| 1 | Wrong answer |
| 3 | Correct but slow (struggled) |
| 4 | Correct at normal speed |
| 5 | Correct and fast (instant recall) |

## Interval Growth (Correct Answers)

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

## Gentler Penalties (Wrong Answers)

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

## Exponential Algorithm (Alternative)

Langner also supports an **exponential** algorithm as an alternative to Modified SM-2. This algorithm uses a simpler scoring model:

- **Score** = sum of `(quality - 3)` for all review history
- Score is clamped to a minimum of 1
- **Interval** = `base ^ max(score - 1, 0)` where base defaults to 4

Example progression with base 4 and quality 4 each time:
- Review 1: score 1, interval = 4^0 = 1 day
- Review 2: score 2, interval = 4^1 = 4 days
- Review 3: score 3, interval = 4^2 = 16 days
- Review 4: score 4, interval = 4^3 = 64 days
- Review 5: score 5, interval = 4^4 = 256 days

Wrong answers (quality < 3) subtract from the score, naturally reducing the interval without a separate penalty formula. Quality 5 answers add 2 to the score, accelerating growth.

## Configuration

Choose the algorithm and base in your `config.yaml`:

```yaml
quiz:
  algorithm: modified_sm2  # or "exponential"
  exponential_base: 4      # base for exponential algorithm (default: 4)
```

The default algorithm is `modified_sm2` for backward compatibility.

## Recalculating Intervals

If you change the algorithm or base, run the recalculate command to update all existing learning history:

```bash
langner migrate recalculate-intervals
```

This replays all review history through the configured algorithm and updates stored intervals.
