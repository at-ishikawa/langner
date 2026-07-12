---
title: "Backend & Data Model"
weight: 3
---

# Backend & Data Model

## Overview

Backend design for the Relearn Quiz — a practice-only quiz whose defining property is that it **writes nothing to learning history**. It reuses the existing pure OpenAI graders and the existing pool-selection logic that Quiz Analytics already uses to find "wrong words", but it bypasses every `Save*` path, so SM-2 intervals, easiness factors, and the Quiz Analytics view are untouched.

This document is grounded in the current code. All type/function references are to real symbols in `backend/`.

### Goals

- Select the pool as **every word whose most-recent in-window learning log has status `misunderstood`**, across all quiz types, mirroring how analytics selects wrong words — on both the DB (`learning_logs`) and YAML-only paths.
- Present and grade each word **in the format of the quiz it was failed in** — one pooled card per failed log series — using the matching existing pure grader: `GradeNotebookAnswer` (recognition), `GradeReverseAnswer` (reverse), `GradeEtymologyStandardAnswer` / `GradeEtymologyReverseAnswer` (etymology).
- Persist **nothing** — not learning history and not any relearn-local state: the submit path calls only the `Grade*` methods and **never** `SaveResult` / `SaveReverseResult` / `SaveFreeformResult` / `SaveEtymologyOriginResult` / `learningRepository.Create` / `UpdateLog` / any etymology YAML write / `GetLatestLearnedInfo`.
- Be **repeatable**: because a correct relearn answer records nothing, the same in-window words come back every session, so the learner can drill them again and again. A word leaves the pool only by aging out of the window or by being fixed in a real quiz.
- Return a rich response carrying the result card **plus** context scenes, `graph_context`, and `example_words`, but deliberately **no** `next_review_date` / `learned_at`.

### Non-Goals

- Any change to SM-2, to the `learning_logs` schema, or to how existing quizzes record history.
- Server-persisted, resumable sessions. The loop is client-driven (see [Frontend Design]({{< relref "frontend-design" >}})); the backend is stateless about Relearn sessions.
- Surfacing Relearn activity anywhere in analytics.

## Relationship to the Learning-History Invariants

The repo's learning-history invariants (L1–L4) govern every path that **writes, reads, or displays** a quiz *learning record*. The Relearn Quiz is deliberately outside that system: it produces no learning record at all. The invariants are respected precisely by **not** participating — the submit handler never reaches `SaveResult` or `GetLatestLearnedInfo`, so there is no canonical-key, symmetric-read, or cross-notebook concern to get wrong. The Relearn Quiz persists **nothing** — no learning log and no relearn-local marker — so there is no parallel store to keep in sync with anything.

## Word Pool Selection

The pool is "every word whose most-recent in-window log status is `misunderstood`". This reuses the analytics definition of wrong: in `backend/internal/notebook/types.go`, "IsWrong is true when the attempt was marked misunderstood. Anything else (understood, usable, intuitive) counts as correct" — status constants live in `backend/internal/notebook/notebook.go:33-37` (`LearnedStatusMisunderstood = "misunderstood"`, `LearnedStatusUnderstood = "understood"`, `LearnedStatusCanBeUsed = "usable"`). Because Relearn writes nothing, the pool is a pure function of the learning histories and the window — the same word reappears each session until a real quiz fixes it or it ages out.

Selection runs on the same two backends the rest of the app uses. Per `backend/cmd/langner-server/main.go:93-137`, YAML is the source of truth and the DB is a secondary mirror enabled only when `Database.Host` and `Database.Password` are set.

### Source: the YAML learning histories (both storage modes)

The pool is built from the on-disk YAML learning histories in **every** configuration — with or without a database — implemented as `Service.LoadRelearnPool` in `backend/internal/quiz/relearn.go`.

This mirrors a decision the codebase already made for analytics. `main.go:102-107` documents it verbatim: *"Analytics always reads from YAML: the on-disk learning history files are the only place etymology quiz results are persisted today — `SaveEtymologyOriginResult` writes YAML directly and does not go through the learning repository, so the DB's `learning_logs` has only vocab rows."* A `learning_logs`-only query would therefore silently drop every etymology-origin wrong word. The YAML histories are the complete, canonical source, so the Relearn pool reads them directly and spans vocabulary and etymology uniformly.

`LoadRelearnPool` walks `notebook.NewLearningHistories(dir)` and, for each expression, inspects its four independent log series — `LearnedLogs` (notebook + freeform), `ReverseLogs`, `EtymologyBreakdownLogs`, `EtymologyAssemblyLogs`. For each series it takes the newest log (histories are stored newest-first) and includes the word when that log is within the window **and** its status is `misunderstood` — exactly the "most-recent-in-window wrong" rule analytics uses (`internal/notebook/types.go`: *"IsWrong is true when the attempt was marked misunderstood"*). A word wrong in several series collapses to one card, keeping the most-recent wrong series for the `source_quiz_type` label.

Each surviving wrong word is then resolved to a gradeable card by intersecting against the words the notebook readers already load — `Service.LoadAllWords()` for vocabulary and `Service.LoadEtymologyOriginCards(...)` for origins — indexed by expression. That yields the meaning, contexts, and (for origins) the sense needed to grade and to render the feedback card. Words with no matching card are skipped (nothing to grade or show).

The single selector is the one place the "most-recent-in-window `misunderstood`" rule lives, and both the DB-configured and YAML-only deployments call it — the read/write-symmetry discipline the learning-history invariants require, even though the Relearn pool writes no log.

`LoadRelearnPool(windowStart time.Time)` takes only the window boundary; it has no other inputs and no side effects.

## No Persisted State (Relearn is Repeatable)

The Relearn Quiz stores **nothing** — not a learning log, and not any relearn-local marker. This is a deliberate product decision: the point of Relearn is to let the learner drill the same recently-missed words as many times as they want. If a correct relearn answer suppressed the word, a second run the same day would surface almost nothing — the opposite of what "relearn" should do.

So there is no `relearn_clears` table and no in-memory clear map. An earlier revision of this design added a non-SR `relearn_clears` marker (migration `017`) to keep recovered words out of the next session; migration `018_drop_relearn_clears` removes it, and the store interface and its DB/memory implementations are deleted. The pool builder now has a single input — the look-back window — and is a pure read over the YAML learning histories.

A word leaves the pool through the two mechanisms that already existed and require no new state:

- It **ages out** of the rolling look-back window.
- It is **fixed in a real quiz**, whose fresh `understood` log becomes the most-recent in-window log so the most-recent-in-window `misunderstood` check no longer selects it.

Correct/incorrect within a Relearn session only reshapes that session's client-side working queue (see [Frontend Design]({{< relref "frontend-design" >}})); the backend never learns of it.

## Proto API

Add to `proto/api/v1/quiz.proto`. The `QuizService` currently has the RPCs listed in `quiz.proto:25-46`; add three:

```proto
service QuizService {
  // ... existing RPCs ...
  rpc StartRelearnQuiz(StartRelearnQuizRequest) returns (StartRelearnQuizResponse);
  rpc SubmitRelearnAnswer(SubmitRelearnAnswerRequest) returns (SubmitRelearnAnswerResponse);
  rpc BatchSubmitRelearnAnswers(BatchSubmitRelearnAnswersRequest) returns (BatchSubmitRelearnAnswersResponse);
}
```

There is no `OverrideRelearnCard` RPC: the Mark-as-Correct/Incorrect override is purely client-side, because Relearn persists nothing for it to reconcile.

### Label-only enum value

The existing `QuizType` enum (`quiz.proto:9-17`) ends at `QUIZ_TYPE_ETYMOLOGY_FREEFORM = 6`. Add:

```proto
enum QuizType {
  // ... existing values 0..6 ...
  QUIZ_TYPE_RELEARN = 7;  // UI label only — NEVER written to learning_logs or YAML
}
```

`QUIZ_TYPE_RELEARN` exists so the frontend can label the mode. It is never assigned to a `learning_logs.quiz_type` value and never written to a YAML `quiz_type` field — the submit handler never constructs a `learning.LearningLog` at all.

### Start

```proto
message StartRelearnQuizRequest {
  // Look-back window in hours. Default 24 when unset; clamped to [1, 168].
  int32 window_hours = 1 [(buf.validate.field).int32 = {gte: 1, lte: 168}];
}

message StartRelearnQuizResponse {
  repeated RelearnCard cards = 1;
}

message RelearnCard {
  int64 note_id = 1;
  string entry = 2;               // the expression shown; the meaning-grading key
  QuizType source_quiz_type = 3;  // where the most-recent wrong answer came from (label only)
  repeated Example examples = 4;  // reuses the existing Example message
}
```

Note on validation: the proto set has no existing two-sided ranged-int precedent (the only ranged int today is one-sided `int32.gte = 0` at `analytics.proto:40`). The `{gte: 1, lte: 168}` form above is the natural expression of a clamp; because unset/`0` must map to the default 24, the handler also normalizes in code:

```go
window := req.Msg.GetWindowHours()
if window == 0 { window = 24 }
if window < 1 { window = 1 } else if window > 168 { window = 168 }
```

### Submit

The response mirrors `SubmitAnswerResponse` (`quiz.proto:165-176`) for the result card, adds the context payload, and **omits `next_review_date` and `learned_at` entirely** — they are not "left blank", they are not fields on this message, so there is nothing for the client to render or gate an override on.

```proto
message SubmitRelearnAnswerRequest {
  int64 note_id = 1 [(buf.validate.field).int64.gt = 0];
  string answer = 2;
  int64 response_time_ms = 3;
  bool is_skipped = 4;
}

message SubmitRelearnAnswerResponse {
  bool correct = 1;
  string meaning = 2;
  string reason = 3;
  WordDetail word_detail = 4;
  repeated string images = 5;

  // Rich Learn-page context (see Frontend Design):
  repeated RelearnContextScene context_scenes = 6;  // conversations/statements the word appears in
  GraphPrompt graph_context = 7;                     // reuse of the etymology relation graph
  repeated string example_words = 8;                 // related words sharing the origin

  // Deliberately NO next_review_date and NO learned_at.
}

message RelearnContextScene {
  string notebook_name = 1;
  string scene_title = 2;
  repeated string statements = 3;   // prose lines containing the expression
  repeated ConversationLine conversations = 4;
}

message ConversationLine {
  string speaker = 1;
  string quote = 2;
}

message BatchSubmitRelearnAnswersRequest {
  repeated SubmitRelearnAnswerRequest answers = 1 [(buf.validate.field).repeated.min_items = 1];
}
message BatchSubmitRelearnAnswersResponse {
  repeated SubmitRelearnAnswerResponse responses = 1;
}
```

`GraphPrompt`, `WordDetail`, and `Example` are existing messages reused as-is. The batch variant exists for parity with the other quiz modes, though the client-driven loop primarily uses the single-shot `SubmitRelearnAnswer` (one card resolved at a time — see [Frontend Design]({{< relref "frontend-design" >}})).

## Handler Flow

The `QuizHandler` (`backend/internal/server/quiz_handler.go:25-39`) already holds per-session in-memory card stores (`noteStore`, `reverseStore`, `etymologyOriginStore`, …) plus `nextID` and a mutex. Add a single `relearnStore map[int64]quiz.RelearnCard` — no other dependency, since Relearn persists nothing.

### StartRelearnQuiz

1. Normalize/clamp `window_hours`.
2. Ask the pool selector for candidate `(note_id, source_quiz_type, entry)` rows within the window whose most-recent log is `misunderstood`.
3. Build a `RelearnCard` per row (loading examples/meaning/etymology metadata for grading and context), store it in `relearnStore` keyed by `note_id`, and return the cards.

### SubmitRelearnAnswer — the critical path

The existing standard handler pattern is **validate → look up card → Grade → Save → GetLatestLearnedInfo → respond** (`quiz_handler.go:117-139`). The Relearn handler uses the **same first three steps and then stops before any Save/read-back**:

```go
func (h *QuizHandler) SubmitRelearnAnswer(ctx, req) (*SubmitRelearnAnswerResponse, error) {
    validateRequest(req.Msg)
    card := h.relearnStore[req.Msg.GetNoteId()]   // under h.mu; NotFound if missing

    var grade quiz.GradeResult
    // gradeRelearn dispatches to the pure grader matching the card's Format, so
    // the answer is graded in the direction the word was failed in:
    //   skip                        -> skippedGradeResult()   (quiz_handler_batch.go:22-29)
    //   QuizTypeReverse             -> GradeReverseAnswer(card.ReverseCard(), ...)
    //   QuizTypeEtymologyStandard   -> GradeEtymologyStandardAnswer(card.EtymologyCard(), ...)
    //   QuizTypeEtymologyReverse    -> GradeEtymologyReverseAnswer(card.EtymologyCard(), ...)
    //   default (recognition)       -> GradeNotebookAnswer(card.VocabCard(), ...)
    grade, err := h.gradeRelearn(ctx, card, req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs(), req.Msg.GetIsSkipped())

    // Record NOTHING — not learning history and not any relearn-local state.
    // Relearn is repeatable, so the word stays in the pool until it ages out of
    // the window or is fixed in a real quiz.

    return connect.NewResponse(&apiv1.SubmitRelearnAnswerResponse{
        Correct:       grade.Correct,
        Meaning:       card.Meaning,
        Reason:        grade.Reason,
        WordDetail:    toProtoWordDetail(card.WordDetail),
        Images:        card.Images,
        ContextScenes: toProtoRelearnScenes(card.ContextScenes),  // conversations/statements
        GraphContext:  graphContext,                              // etymology graph (nil if none)
        ExampleWords:  exampleWords,                              // related words (empty if none)
        // no NextReviewDate, no LearnedAt — not fields on this message
    }), nil
}
```

Guarantees this path upholds:

- It calls **only** the pure graders `GradeNotebookAnswer` / `GradeReverseAnswer` / `GradeEtymologyStandardAnswer` / `GradeEtymologyReverseAnswer` (all pure — they call `AnswerMeanings` / `ValidateWordForm` and return `GradeResult{Correct, Reason, Quality, Classification}` with no repository access).
- It **never** calls `SaveResult` / `SaveReverseResult` / `SaveFreeformResult` / `SaveEtymologyOriginResult`, so `learningRepository.Create` and the etymology YAML write are never reached.
- It **never** calls `GetLatestLearnedInfo`, so no `learned_at` / `next_review_date` is computed or returned.
- It performs **no write of any kind** — no learning log and no relearn-local marker.

`grade.Quality` is computed by the graders but simply **discarded** here — it exists only so we could reuse the grader unchanged; nothing consumes it.

### Grading source selection

Whether a word is graded by the etymology grader or the notebook grader is decided by the card metadata captured at `StartRelearnQuiz` time (`RelearnCard.IsEtymology`, derived from which log series the wrong answer came from), not by anything the client sends — keeping the grading rule server-side and in one place.

### Context assembly

The context scenes and the `graph_context` / `example_words` fields are populated from the same notebook and etymology data the app already loads:

- **Conversations/statements** come from the word's context sentences (`card.Contexts`, the same data the reverse quiz surfaces), shaped into `RelearnContextScene` so the frontend can highlight the expression.
- **`example_words` and `graph_context`** reuse the exact helpers the etymology submit handler already uses — `loadCardExampleWords(card)` and `buildGraphContextForCard(...)` (`quiz_handler.go`); for non-etymology words they are empty/absent.

Because this context is assembled from read-only notebook data and returned in the response, it involves no learning-history access and cannot affect SM-2 or analytics.

## Files to Modify / Add

```
proto/api/v1/quiz.proto                                   # ADD RPCs, RelearnCard, Submit/Start messages, QUIZ_TYPE_RELEARN
backend/internal/quiz/relearn.go                          # NEW: RelearnCard, LoadRelearnPool, context builders
backend/internal/server/quiz_handler_relearn.go           # NEW: Start/Submit/Batch handlers
backend/internal/server/quiz_handler.go                   # MODIFY: add relearnStore
```

No changes to `learning_logs`, to `learning.LearningRepository`, to the SM-2 `IntervalCalculator`, or to any analytics code — the Relearn Quiz sits entirely alongside them. It adds no table of its own: migration `018_drop_relearn_clears` removes the `relearn_clears` table an earlier revision introduced, and `datasync.go` no longer lists it.

## Testing

- **Pool selection (DB)**: seed `learning_logs` with a word wrong then later correct → excluded; wrong only → included; wrong in two quiz types → one card. Uses the live-DB integration test harness the repo already has for the Postgres path.
- **Pool selection (YAML)**: same cases over on-disk history.
- **No-write guarantee**: after a full Relearn session (mix of correct/wrong/skip), assert `learning_logs` row count is unchanged, no YAML learning-history file mtime changed, and analytics `DayDetail`/`DailySummaries` for the day are identical to before. This is the load-bearing test for the feature's core promise.
- **Repeatability**: a correctly-answered word reappears in the very next `StartRelearnQuiz` for the same window — the DB integration test asserts the word is still in the pool after a correct answer, because Relearn persists no clear state.
- **Grading dispatch / direction**: a reverse card grades by the word (`GradeReverseAnswer` — typing the meaning is wrong); a recognition card grades by the meaning; etymology cards route to the standard/reverse etymology graders; a skip yields `skippedGradeResult()` and re-queues client-side. A word failed in two types yields two independent cards.
