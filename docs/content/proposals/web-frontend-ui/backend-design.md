---
title: "Backend Design"
weight: 5
---

# Backend Design

## Overview

The Go backend is an HTTP server that exposes Connect RPC handlers. It reuses existing CLI domain logic — no new data storage or business logic is needed.

## Connect RPC

Connect RPC is built on Go's standard `net/http`. Handlers are registered as `http.Handler` on a standard `http.ServeMux`. No gRPC framework dependency is required.

## File Structure

```
proto/                # Protobuf definitions (shared)
  quiz/
    v1/
      quiz.proto

backend/
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

Protobuf definitions live in the `proto/` directory. Code generation produces:

- **Go server code** — `protoc-gen-go` + `protoc-gen-connect-go`
- **TypeScript client code** — `@bufbuild/protoc-gen-es` + `@connectrpc/protoc-gen-connect-es`

## Concurrency

The CLI processes one request at a time. The HTTP server handles concurrent requests, which could cause race conditions when writing learning history YAML files (e.g., two simultaneous `SubmitAnswer` calls).

A mutex protects file writes. This is sufficient for a single-user local application.

## Configuration

The server reuses the same `config.yaml` and environment variables as the CLI (`OPENAI_API_KEY`, `RAPID_API_KEY`). No separate server configuration is needed.

## Error Handling

Connect RPC errors map to standard codes:

| Scenario | Connect Code |
|----------|-------------|
| Notebook not found | `NOT_FOUND` |
| Invalid request (missing fields) | `INVALID_ARGUMENT` |
| OpenAI API failure | `INTERNAL` |

For `INVALID_ARGUMENT`, Connect RPC supports the same error details as gRPC using `google.golang.org/genproto/googleapis/rpc/errdetails`. Field-level validation errors use `BadRequest` with `FieldViolations` to identify which field failed and why. The API (`connect.NewErrorDetail`, `AddDetail`, `Details()`) is identical to gRPC — only the wire format differs (JSON in response body for Connect protocol, trailers for gRPC).

## CORS

Since the frontend (`:3000`) and backend (`:8080`) run on different ports, the server enables CORS for local development.

## Testing

Handler tests mock the domain layer and OpenAI client using the existing `mock_inference.Client`. Tests follow the same patterns as the CLI tests (table-driven, testify assertions).
