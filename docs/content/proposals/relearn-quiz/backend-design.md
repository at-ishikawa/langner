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
- Persist **nothing** to learning history: the submit path calls only the `Grade*` methods and **never** `SaveResult` / `SaveReverseResult` / `SaveFreeformResult` / `SaveEtymologyOriginResult` / `learningRepository.Create` / `UpdateLog` / any etymology YAML write / `GetLatestLearnedInfo`.
- Keep already-recovered words out of the next Relearn session via a lightweight, non-SR **"relearn clears"** marker — a new `relearn_clears` table when a DB is present, or an in-memory map otherwise. It is explicitly **not** a learning log.
- Return a rich response carrying the result card **plus** context scenes, `graph_context`, and `example_words`, but deliberately **no** `next_review_date` / `learned_at`.

### Non-Goals

- Any change to SM-2, to the `learning_logs` schema, or to how existing quizzes record history.
- Server-persisted, resumable sessions. The loop is client-driven (see [Frontend Design]({{< relref "frontend-design" >}})); the backend is stateless about Relearn sessions.
- Surfacing Relearn activity anywhere in analytics.

## Relationship to the Learning-History Invariants

The repo's learning-history invariants (L1–L4) govern every path that **writes, reads, or displays** a quiz *learning record*. The Relearn Quiz is deliberately outside that system: it produces no learning record at all. The invariants are respected precisely by **not** participating — the submit handler never reaches `SaveResult` or `GetLatestLearnedInfo`, so there is no canonical-key, symmetric-read, or cross-notebook concern to get wrong. The one thing the Relearn Quiz *does* persist — the relearn-clear marker — is defined below as explicitly not a learning log and is never read by SM-2 or analytics.

## Word Pool Selection

The pool is "every word whose most-recent in-window log status is `misunderstood`, minus words already cleared more recently". This reuses the analytics definition of wrong: in `backend/internal/notebook/types.go`, "IsWrong is true when the attempt was marked misunderstood. Anything else (understood, usable, intuitive) counts as correct" — status constants live in `backend/internal/notebook/notebook.go:33-37` (`LearnedStatusMisunderstood = "misunderstood"`, `LearnedStatusUnderstood = "understood"`, `LearnedStatusCanBeUsed = "usable"`).

Selection runs on the same two backends the rest of the app uses. Per `backend/cmd/langner-server/main.go:93-137`, YAML is the source of truth and the DB is a secondary mirror enabled only when `Database.Host` and `Database.Password` are set.

### Source: the YAML learning histories (both storage modes)

The pool is built from the on-disk YAML learning histories in **every** configuration — with or without a database — implemented as `Service.LoadRelearnPool` in `backend/internal/quiz/relearn.go`.

This mirrors a decision the codebase already made for analytics. `main.go:102-107` documents it verbatim: *"Analytics always reads from YAML: the on-disk learning history files are the only place etymology quiz results are persisted today — `SaveEtymologyOriginResult` writes YAML directly and does not go through the learning repository, so the DB's `learning_logs` has only vocab rows."* A `learning_logs`-only query would therefore silently drop every etymology-origin wrong word. The YAML histories are the complete, canonical source, so the Relearn pool reads them directly and spans vocabulary and etymology uniformly.

`LoadRelearnPool` walks `notebook.NewLearningHistories(dir)` and, for each expression, inspects its four independent log series — `LearnedLogs` (notebook + freeform), `ReverseLogs`, `EtymologyBreakdownLogs`, `EtymologyAssemblyLogs`. For each series it takes the newest log (histories are stored newest-first) and includes the word when that log is within the window **and** its status is `misunderstood` — exactly the "most-recent-in-window wrong" rule analytics uses (`internal/notebook/types.go`: *"IsWrong is true when the attempt was marked misunderstood"*). A word wrong in several series collapses to one card, keeping the most-recent wrong series for the `source_quiz_type` label.

Each surviving wrong word is then resolved to a gradeable card by intersecting against the words the notebook readers already load — `Service.LoadAllWords()` for vocabulary and `Service.LoadEtymologyOriginCards(...)` for origins — indexed by expression. That yields the meaning, contexts, and (for origins) the sense needed to grade and to render the feedback card. Words with no matching card are skipped (nothing to grade or show).

The single selector is the one place the "most-recent-in-window `misunderstood`" rule lives, and both the DB-configured and YAML-only deployments call it — the read/write-symmetry discipline the learning-history invariants require, even though the Relearn pool writes no log.

## "Relearn Clears" Marker

A Relearn attempt records no learning log, so a correctly-answered word's most-recent *real* log stays `misunderstood` and it would reappear in the next Relearn pool that day. The relearn-clear marker prevents that, without being a learning record.

### New table (DB present) — migration `017`

Migrations use golang-migrate with `NNN_description.up.sql` / `.down.sql`, 3-digit sequential; the current head is `016`, so this is `017` (`backend/schemas/migrations/`). The DB runner applies `m.Up()` on startup (`backend/internal/database/db.go:99-109`).

`017_add_relearn_clears.up.sql`:

```sql
CREATE TABLE relearn_clears (
    clear_key VARCHAR(512) NOT NULL,
    cleared_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (clear_key)
);
CREATE INDEX idx_relearn_clears_cleared_at ON relearn_clears (cleared_at);
```

`017_add_relearn_clears.down.sql`:

```sql
DROP TABLE IF EXISTS relearn_clears;
```

**Why a text `clear_key` and not a `note_id` foreign key.** The pool is built from the YAML histories, which include etymology origins — and origins have no row in `notes` (they live in `etymology_origins`, and `SaveEtymologyOriginResult` never creates a note). A `note_id` FK could not represent a cleared origin at all. The key is therefore an opaque string the pool builder owns: `"<v|o>\x00<notebook>\x00<lowercased expression>"` (a kind prefix keeps a vocab word and an origin that share a spelling in the same notebook distinct — the same `(name, type)` disambiguation the learning history already uses). `RelearnClearKey` in `backend/internal/quiz/relearn.go` is the single function that builds it, and both the pool read and the clear write call it.

Design choices that keep this from being a learning log:

- **One row per key**; a new clear upserts `cleared_at` to `now()`. There is no history of clears — only the latest matters for pool suppression.
- It has **no `quiz_type`, no `status`, no `quality`, no `interval_days`, no `easiness_factor`**. It is structurally incapable of feeding SM-2.
- Nothing reads this table except the Relearn pool builder. Neither the SM-2 write path (`learning.LearningRepository`) nor the analytics repos reference it.

The upsert:

```sql
INSERT INTO relearn_clears (clear_key, cleared_at) VALUES ($1, $2)
ON CONFLICT (clear_key) DO UPDATE SET cleared_at = EXCLUDED.cleared_at;
```

### In-memory map (no DB)

When no database is configured, relearn clears live in a process-local `map[string]time.Time` (clear_key → latest cleared_at), guarded by a mutex. It is **never** written to any YAML learning-history file — writing it there would make it a learning record and violate the whole point. It is naturally ephemeral: a process restart clears it, at which point words age out of the pool on their own via the rolling window.

One interface, defined in `backend/internal/learning/relearn_clears.go`, lets the handler treat both the same way:

```go
type RelearnClearStore interface {
    // AllClears returns every recorded clear keyed by clear_key. The pool
    // builder reads the whole set once per session start and filters in Go.
    AllClears(ctx context.Context) (map[string]time.Time, error)
    // MarkCleared upserts the latest clear time for a key.
    MarkCleared(ctx context.Context, clearKey string, at time.Time) error
}
```

`DBRelearnClearStore` runs the SQL above; `MemoryRelearnClearStore` reads and writes the map. `main.go` selects the DB implementation when `Database.Host`+`Password` are set (`handler.SetRelearnClearStore(...)`) and the memory implementation otherwise — the same wiring pattern as `learningRepo`. The pool builder takes the `AllClears` map and excludes a word when `clears[key]` is not before its most-recent wrong log; the handler calls `MarkCleared(card.ClearKey, now)` only on a correct answer.

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

The `QuizHandler` (`backend/internal/server/quiz_handler.go:25-39`) already holds per-session in-memory card stores (`noteStore`, `reverseStore`, `etymologyOriginStore`, …) plus `nextID` and a mutex. Add a `relearnStore map[int64]quiz.RelearnCard` and a `RelearnClears` dependency.

### StartRelearnQuiz

1. Normalize/clamp `window_hours`.
2. Ask the pool selector (DB or YAML) for candidate `(note_id, source_quiz_type, entry)` rows within the window whose most-recent log is `misunderstood` and that are not more-recently cleared.
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

    // Record NOTHING to learning history. The ONLY persistence is the non-SR
    // clear marker, and only on a correct answer. A marker-write failure is
    // logged, never fatal (the word simply reappears next session).
    if grade.Correct {
        _ = h.relearnClears.MarkCleared(ctx, card.ClearKey, time.Now())
    }

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
- The only write is `relearn_clears`, which no SM-2 or analytics code reads.

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
backend/schemas/migrations/017_add_relearn_clears.up.sql  # NEW
backend/schemas/migrations/017_add_relearn_clears.down.sql# NEW
backend/internal/quiz/relearn.go                          # NEW: RelearnCard, LoadRelearnPool, context builders
backend/internal/learning/relearn_clears.go               # NEW: RelearnClearStore + DB & memory impls
backend/internal/server/quiz_handler_relearn.go           # NEW: Start/Submit/Batch handlers
backend/internal/server/quiz_handler.go                   # MODIFY: add relearnStore + relearnClears + setter
backend/cmd/langner-server/main.go                        # MODIFY: wire RelearnClearStore (DB vs memory), like learningRepo
backend/cmd/langner/datasync.go                           # MODIFY: list relearn_clears in the clear/validate-db table set
```

No changes to `learning_logs`, to `learning.LearningRepository`, to the SM-2 `IntervalCalculator`, or to any analytics code — the Relearn Quiz sits entirely alongside them.

## Testing

- **Pool selection (DB)**: seed `learning_logs` with a word wrong then later correct → excluded; wrong only → included; wrong in two quiz types → one card; a `relearn_clears` row newer than the last wrong log → excluded; older → included. Uses the live-DB integration test harness the repo already has for the Postgres path.
- **Pool selection (YAML)**: same cases over on-disk history + the in-memory clears map.
- **No-write guarantee**: after a full Relearn session (mix of correct/wrong/skip), assert `learning_logs` row count is unchanged, no YAML learning-history file mtime changed, and analytics `DayDetail`/`DailySummaries` for the day are identical to before. This is the load-bearing test for the feature's core promise.
- **Clear marker is not a log**: assert a cleared word has a `relearn_clears` row but **no** new `learning_logs` row, and that its `next_review_date` from a subsequent real quiz is unaffected.
- **Grading dispatch / direction**: a reverse card grades by the word (`GradeReverseAnswer` — typing the meaning is wrong); a recognition card grades by the meaning; etymology cards route to the standard/reverse etymology graders; a skip yields `skippedGradeResult()` and re-queues client-side. A word failed in two types yields two independent cards.
