---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

Add a **Relearn Quiz** — a standalone quiz type that collects every word the learner recently got wrong, across *all* quiz types (notebook, freeform, reverse, and the etymology modes), into a single mixed session. The session presents each word in one unified recognition format (show the expression, ask for its meaning) and loops until every word has been answered correctly at least once.

The Relearn Quiz is a **practice-only** tool. It deliberately writes **nothing** to learning history: it never records a log, never advances or resets a spaced-repetition (SM-2) interval, and never appears in Quiz Analytics. It exists so the learner can drill down on their recent mistakes as many times as they like without polluting the data that drives scheduling and analytics.

## Problem

### Recent mistakes are scattered and only reviewed once

When a learner gets a word wrong in a quiz, the immediate feedback is the only chance to see it again in that session. After the session ends, the word is scheduled by SM-2 for some future date — often days away — and it is mixed back in among many other words. There is no way to say "show me everything I just got wrong, right now, and keep drilling me until I have them."

The learner's wrong answers are also spread across different quiz types and notebooks. A word failed in the reverse quiz, another failed in an etymology breakdown, and a third failed in a standard notebook quiz all represent "things I struggled with today", but there is no single surface that brings them together.

### Re-drilling a word through a normal quiz distorts the schedule

A learner *could* re-run a normal quiz to hit a word again, but every attempt in a normal quiz writes a learning log and moves the SM-2 schedule. Drilling a word five times in a row to cement it would flood the history with five records and inflate (or, on a slip, deflate) its easiness factor and next-review date. That corrupts both the spaced-repetition schedule and the Quiz Analytics view, which reads the same logs. There is no "off the record" way to practice.

### No way to focus a session purely on weak words

The existing quizzes select words by SM-2 due-date and notebook membership. None of them offers "the set of words I got wrong in the last N hours" as a pool, regardless of which quiz produced the mistake.

## Goals

- Provide a one-tap session built from **every word answered incorrectly across all quiz types within a recent time window** (default: last 24 hours, configurable on the start screen).
- Present all words in a **single unified recognition format** (expression → meaning), regardless of the quiz type that originally produced the wrong answer.
- **Loop** the session until every word has been answered correctly at least once — a word answered correctly leaves the queue; a word answered wrong or skipped goes to the back of the queue.
- Grade answers with the **same OpenAI meaning graders** the existing quizzes use, so "correct" means the same thing here as everywhere else.
- Write **nothing** to learning history — no logs, no SM-2 changes, no analytics impact.
- Keep words the learner has already recovered from **out of the next Relearn session that day**, without recording anything that SM-2 or analytics would read.
- Offer a **rich feedback screen** that shows not just the result but the full Learn-page context for the word — the conversations/statements it appears in, plus its etymology origin and related words — so a re-drill is also a re-learn.

## Non-Goals

- Changing how any existing quiz selects, grades, or records words.
- Any change to the SM-2 algorithm, easiness factors, intervals, or next-review dates.
- Surfacing Relearn activity in Quiz Analytics or any history/streak view.
- Producing new vocabulary content — the Relearn Quiz only re-presents words the learner already has.
- Persisting a server-side session. The loop is driven entirely by the client; the backend is stateless about Relearn sessions.

## How the Word Pool Is Selected

The pool for a Relearn session is **every word whose most-recent in-window learning log has status `misunderstood`**, evaluated within the configured time window (default 24 hours), across all quiz types.

This mirrors exactly how Quiz Analytics already decides a word was "gotten wrong": it looks at the learner's logs, takes the most recent one in range, and treats `misunderstood` as wrong (`understood` / `usable` are correct). The Relearn Quiz reuses that same most-recent-log rule so that "words I got wrong" means the same thing in both places.

Selection details:

- **Window.** The learner picks a look-back window on the start screen. The default is 24 hours; the value is clamped to a sane range (see the [Backend Design]({{< relref "backend-design" >}})). Only logs written within the window are considered.
- **Most-recent-in-window wins.** For each `(word, quiz type)` series, only the most recent log inside the window decides inclusion. A word that was wrong at 9am but answered correctly at 11am (in a real quiz) is **not** in the pool — its most recent in-window log is `understood`, so it drops out naturally.
- **All quiz types contribute.** A word wrong in `reverse`, a word wrong in `etymology_breakdown`, and a word wrong in `notebook` all land in the same pool. They are de-duplicated to a single card per underlying word so the learner is not asked the same expression twice in one loop.
- **Both storage backends.** Selection works on the database path (the `learning_logs` table) and on the YAML-only path (learning-history files), the same two paths the rest of the app already supports.

### "Relearn clears" — keeping recovered words out of the next session

Because a Relearn attempt records nothing, answering a word correctly in Relearn would normally leave its most-recent *real* log unchanged — so the same word would reappear in the very next Relearn session that day, even though the learner just recovered it.

To avoid that, a Relearn session records a lightweight, **non-spaced-repetition** marker when a word is cleared: a "relearn clear" of `(note_id, cleared_at)`. When the next Relearn pool is built, a word is excluded if it has a relearn-clear marker more recent than its most-recent in-window wrong log.

This marker is explicitly **not a learning log**:

- It is never read by SM-2 and never affects intervals, easiness factors, or next-review dates.
- It is never read by Quiz Analytics.
- It only gates the Relearn pool.

A word that the learner later gets wrong again in a *real* quiz reappears in the pool naturally, because that new wrong log is more recent than the relearn-clear marker. Conversely, a word cleared by a later real quiz also drops out naturally via the most-recent-log check — the marker is only needed to cover words that Relearn itself recovered.

## User Stories

### Start a Relearn Session

As a learner, I want to open the Relearn Quiz and immediately practice the words I recently got wrong, so I can shore them up while they are still fresh.

- Open the Relearn Quiz start screen.
- See how many words are currently in the pool for the default window (last 24 hours).
- Optionally change the look-back window (e.g. last 6 hours, last 48 hours).
- Tap **Start** to begin the session.
- If the pool is empty, see a positive empty state ("Nothing to relearn — you're all caught up") instead of an empty session.

### Answer a Word in the Unified Recognition Format

As a learner, I want every word presented the same way — see the word, type its meaning — regardless of which quiz I originally got it wrong in, so the session has one simple rhythm.

- The card shows the expression and asks for its meaning.
- A small label notes where the word originally came from (e.g. "missed in Reverse"), for context only.
- The answer is graded by the same OpenAI meaning grader the notebook/etymology quizzes use, so "correct" is judged consistently.

### Loop Until Everything Is Correct

As a learner, I want the session to keep going until I have answered every word correctly at least once, so I actually finish having relearned all of them.

- A correct answer removes the word from the working queue.
- A wrong answer, or a skip, sends the word to the **back** of the queue to come around again later in the same session.
- The session ends only when the queue is empty.
- A progress indicator shows how many distinct words remain (not a fixed "1 of N"), since the queue can grow shorter only as words are cleared.

### See Rich Context on Every Answer

As a learner, I want each answer's feedback to show me *why* the word means what it means — where it shows up and where it comes from — so re-drilling doubles as re-learning.

- Beyond the usual result card (correct/incorrect, the meaning, the reason), the feedback screen shows the **Learn-page context** for the word:
  - The conversations or statements the expression appears in.
  - Its etymology origin and related words that share the same origin, with the relation graph.
- This reuses the same context the Learn page already assembles, so the information matches what the learner sees when browsing the word normally.

### Practice Without Consequences

As a learner, I want to know that drilling these words changes nothing about my schedule or stats, so I can practice as aggressively as I want.

- Nothing I do in a Relearn session appears in Quiz Analytics.
- No next-review date moves; no easiness factor changes; no log is added to any word's history.
- I can run Relearn twice in a row; words I cleared the first time do not come back the second time (that day), but words I am still missing do.

### Return the Next Day

As a learner, I want yesterday's cleared words gone, but anything I *newly* get wrong tomorrow to show up.

- The relearn-clear markers only suppress words relative to their most-recent wrong log. A fresh wrong answer in a real quiz always re-qualifies a word for the pool.
- The window is a rolling look-back, so old mistakes age out on their own.

## Out of Scope

- Editing or deleting learning history from the Relearn Quiz.
- Selecting the pool by notebook, difficulty, or quiz type (the pool is always "all recent wrong words"; only the time window is configurable).
- A server-persisted, resumable session (the loop is client-side and lives only for the current session).
- Any leaderboard, streak, or trend visualization of Relearn activity.
