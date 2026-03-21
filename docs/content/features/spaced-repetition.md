---
title: "Spaced Repetition"
weight: 3
---

# Spaced Repetition

Langner supports two spaced repetition algorithms: **Modified SM-2** (default) and **Fixed Levels**.

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

## Fixed Levels Algorithm

A simple level-based system with configurable interval steps. Each correct answer advances one level, each wrong answer drops one level.

Default levels: `[1, 3, 7, 14, 30, 60, 120, 365]` days

| Review # | Interval |
|----------|----------|
| 1st correct | 3 days |
| 2nd correct | 7 days |
| 3rd correct | 14 days |
| 4th correct | 30 days |
| 5th correct | 60 days |
| 6th correct | 120 days |
| 7th correct | 365 days |

Wrong answers drop one level. For example, at level 5 (60 days), a wrong answer drops to level 4 (30 days). Level never drops below 0 (1 day).

## Modified SM-2 Algorithm (Default)

A modified version of the SuperMemo 2 algorithm with gentler penalties for well-learned words.

Each word has two key metrics:
- **Easiness Factor (EF)**: Starts at 2.5, adjusts based on performance (range: 1.3 to 3.0+)
- **Interval**: Days until next review, grows with consecutive correct answers

### Interval Growth (Correct Answers)

| Review # | Interval Calculation |
|----------|---------------------|
| 1st correct | 1 day |
| 2nd correct | 6 days |
| 3rd+ correct | Previous interval × EF |

### Gentler Penalties (Wrong Answers)

Unlike standard SM-2 which resets to day 1 on any mistake, Langner uses **proportional reduction** based on your learning progress:

| Previous Correct Streak | Interval Reduction | EF Penalty |
|-------------------------|-------------------|------------|
| 1-2 reviews | Reset to 1 day | -0.54 (full) |
| 3-5 reviews | × 0.5 | -0.40 |
| 6-9 reviews | × 0.6 | -0.30 |
| 10+ reviews | × 0.7 | -0.20 |

## Configuration

Choose the algorithm in your `config.yaml`:

```yaml
quiz:
  algorithm: fixed  # or "modified_sm2"
  fixed_intervals: [1, 3, 7, 14, 30, 60, 120, 365]
```

The default algorithm is `modified_sm2` for backward compatibility.

## Recalculating Intervals

If you change the algorithm or intervals, run the recalculate command to update all existing learning history:

```bash
langner migrate recalculate-intervals
```

This replays all review history through the configured algorithm and updates stored intervals.
