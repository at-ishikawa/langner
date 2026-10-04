// Command server is the Vercel Go-framework entrypoint for the langner backend.
// Vercel's "go" framework detects cmd/server/main.go, builds it, and runs the
// binary — routing $PORT traffic to it. Config comes from the environment (no
// config.yml on Vercel; see config-from-env), and the server listens on $PORT
// because config binds server.port to the PORT env var. It is a thin wrapper
// over serverapp.Serve, the same entrypoint cmd/langner-server uses.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/at-ishikawa/langner/internal/serverapp"
)

func main() {
	if err := serverapp.Serve(context.Background(), ""); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}
