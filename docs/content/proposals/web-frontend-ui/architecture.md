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

## Why Connect RPC over gRPC or Twirp

| | Connect RPC | gRPC | Twirp |
|---|---|---|---|
| Go dependency | `net/http` only | `google.golang.org/grpc` | `net/http` only |
| Browser support | Direct (HTTP/1.1 POST) | Requires gRPC-Web proxy | Direct (HTTP/1.1 POST) |
| TypeScript codegen | Official Buf tooling (`@connectrpc/protoc-gen-connect-es`) | Third-party | Third-party |
| Protocols | Connect + gRPC + gRPC-Web | gRPC only | Twirp only |
| HTTP/2 required | No | Yes | No |

Connect RPC was chosen for official Buf tooling support on both Go and TypeScript, and direct browser compatibility without a proxy.

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

## Out of Scope

- Authentication / user management
- Database migration (stays on YAML files)
- WebSocket / real-time features
- Offline support
- Cloud hosting / deployment
