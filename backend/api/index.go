// Package handler is the Vercel serverless entrypoint for the langner backend.
// Vercel's Go runtime builds every *.go in this directory into a function and
// invokes the exported Handler; backend/vercel.json rewrites ALL paths to it, so
// the connect RPC services and the /auth/* routes all reach this one function.
//
// The full HTTP stack (and its DB pool) is built ONCE per cold start via
// sync.Once and reused across warm invocations — never per request. Config is
// loaded from the environment (no config.yml on Vercel; see config-from-env),
// so the Vercel project's env vars fully configure the server.
package handler

import (
	"net/http"
	"sync"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/serverapp"
)

var (
	once        sync.Once
	rootHandler http.Handler
	initErr     error
)

func initHandler() {
	loader, err := config.NewConfigLoader("")
	if err != nil {
		initErr = err
		return
	}
	cfg, err := loader.Load()
	if err != nil {
		initErr = err
		return
	}
	// The cleanup func and auth components are intentionally dropped: a serverless
	// instance lives for its own lifetime (the OS reclaims the DB pool when the
	// instance is recycled), and there is no long-running process to run the
	// device-code sweeper.
	h, _, _, err := serverapp.BuildHandler(cfg)
	if err != nil {
		initErr = err
		return
	}
	rootHandler = h
}

// Handler is the Vercel function entrypoint. It lazily builds the langner HTTP
// stack on the first request of a cold start, then delegates every request to
// it. A build failure (e.g. missing TOKEN_SIGNING_KEY or an unreachable DB)
// surfaces as a 500 until the instance is recycled.
func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(initHandler)
	if initErr != nil {
		http.Error(w, "server initialization failed: "+initErr.Error(), http.StatusInternalServerError)
		return
	}
	rootHandler.ServeHTTP(w, r)
}
