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

Default levels: `[1, 7, 30, 90, 365]` days

| Quality | Level Change |
|---------|-------------|
| 5 (fast correct) | +2 (skip a level) |
| 3-4 (correct) | +1 |
| 1 (wrong) | -1 |

| Review # | Interval |
|----------|----------|
| 1st correct | 7 days |
| 2nd correct | 30 days |
| 3rd correct | 90 days |
| 4th correct | 365 days |

Wrong answers drop one level. For example, at level 3 (90 days), a wrong answer drops to level 2 (30 days). Level never drops below 0 (1 day). A fast correct answer (quality 5) skips a level, reaching 365 days in just 3 reviews.

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

## Choosing Your Schedule

Each person picks their own schedule on the **Settings** page, under
**Review schedule**:

- **Fixed steps** (the Fixed Levels algorithm) — enter the waits in days,
  shortest first, for example `1, 7, 30, 90, 365`. Each wait must be longer
  than the one before it, between 1 and 3650 days, with at most 12 steps.
- **Adaptive** (the Modified SM-2 algorithm).

Until you choose, you follow the server's default schedule, and **Use the
default** switches you back to it. A new schedule applies to your next
answers only: words you have already studied keep the review dates they have
now, and move to the new schedule the next time you answer them.

## Server Default

The default for people who have not chosen a schedule is set in the server's
`config.yml`:

```yaml
quiz:
  algorithm: fixed  # or "modified_sm2"
  fixed_intervals: [1, 7, 30, 90, 365]
```

or with the `QUIZ_ALGORITHM` and `QUIZ_FIXED_INTERVALS` (comma-separated, e.g.
`1,7,30,90`) environment variables. The default algorithm is `modified_sm2`.
The command-line quizzes, which run without a database, always use this
default.

## Recalculating Intervals

Each answer is scheduled when you give it, using the schedule you have at
that moment; changing the schedule does not rewrite earlier answers. The
admin command `langner-admin migrate repair-intervals` replays each person's
latest answer with their own schedule (or the default) and reports, or with
`--apply` fixes, any review date that does not match.
