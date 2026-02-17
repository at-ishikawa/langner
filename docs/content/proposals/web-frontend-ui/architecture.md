---
title: "Architecture"
weight: 3
---

# Architecture

## Overview

The web frontend UI consists of a Next.js frontend and a Go backend API, both running locally. The frontend communicates with the backend using Connect RPC with protobuf for type-safe API contracts.

```
Next.js Frontend (localhost:3000)  ──Connect RPC──>  Go Backend API (localhost:8080)
                                                          │
                                                          ├── Notebooks (YAML on disk)
                                                          ├── Learning Notes (YAML on disk)
                                                          ├── Dictionary Cache (JSON on disk)
                                                          └── OpenAI API
```

## Why Connect RPC

- **Type-safe API contracts** — Single `.proto` files generate both Go server code and TypeScript client code
- **HTTP/1.1 compatible** — Works with simple POST requests, no HTTP/2 required
- **No proxy required** — Unlike gRPC-Web, Connect protocol works directly from the browser
- **Official Buf tooling** — First-class support for both Go and TypeScript code generation

## Stack

| Component | Technology |
|-----------|-----------|
| Frontend | Next.js (App Router) |
| API protocol | Connect RPC (protobuf) |
| Backend | Go (existing codebase) |
| AI grading | OpenAI GPT-4o-mini (existing) |
| Data storage | YAML files on disk (existing) |

## Protobuf Definitions

Naming follows the [database support proposal](https://github.com/at-ishikawa/langner/pull/34) table naming: `notes` (with `usage`, `entry`, `meaning`), `notebook_notes`, `learning_logs`.

### Service

```protobuf
service QuizService {
  rpc GetQuizOptions(GetQuizOptionsRequest) returns (GetQuizOptionsResponse);
  rpc StartQuiz(StartQuizRequest) returns (StartQuizResponse);
  rpc SubmitAnswer(SubmitAnswerRequest) returns (SubmitAnswerResponse);
}
```

### GetQuizOptions

BFF method for the quiz start screen. Returns everything needed to render the screen.

```protobuf
message GetQuizOptionsRequest {}

message GetQuizOptionsResponse {
  repeated NotebookSummary notebooks = 1;
}

message NotebookSummary {
  string notebook_id = 1;
  string name = 2;
  int32 review_count = 3;
}
```

### StartQuiz

Starts a quiz session. Returns all notes to quiz on.

```protobuf
message StartQuizRequest {
  repeated string notebook_ids = 1;
  bool include_unstudied = 2;
}

message StartQuizResponse {
  repeated Flashcard flashcards = 1;
}

message Flashcard {
  int64 note_id = 1;
  string entry = 2;
  repeated Example examples = 3;
}

message Example {
  string text = 1;
  string speaker = 2;
}
```

`Example.speaker` is set for story notebooks (e.g., "Rachel") and empty for book notebooks or flashcards where the context is a plain statement.

### SubmitAnswer

Submits a user's answer for grading. Updates learning history on the backend.

```protobuf
message SubmitAnswerRequest {
  int64 note_id = 1;
  string answer = 2;
  int64 response_time_ms = 3;
}

message SubmitAnswerResponse {
  bool correct = 1;
  string meaning = 2;
  string reason = 3;
}
```

## Backend Architecture

The Go backend reuses existing code from the CLI. No new data storage is needed — it reads/writes the same YAML files.

```
cmd/
  langner/          # Existing CLI (unchanged)
  langner-server/   # New HTTP server entry point

internal/
  notebook/         # Existing domain logic (reused as-is)
  inference/        # Existing OpenAI client (reused as-is)
  config/           # Existing config (reused as-is)
  server/           # New: Connect RPC handlers
    quiz_handler.go
```

## Code Generation

Protobuf definitions live in a `proto/` directory at the project root. Code generation produces:
- **Go server code** — `protoc-gen-go` + `protoc-gen-connect-go`
- **TypeScript client code** — `@bufbuild/protoc-gen-es` + `@connectrpc/protoc-gen-connect-es`

## CORS

Since the frontend and backend run on different ports locally, the Go backend must enable CORS.

## Out of Scope

- Authentication / user management
- Database migration (stays on YAML files)
- WebSocket / real-time features
- Offline support
- Cloud hosting / deployment
