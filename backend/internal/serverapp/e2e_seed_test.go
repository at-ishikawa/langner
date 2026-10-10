package serverapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/at-ishikawa/langner/internal/config"
)

// TestE2ESeedHandler_Gating covers the reject paths, which return BEFORE any
// database access — so a nil *sqlx.DB is safe here. The seed success path is
// Postgres-only and is exercised by the preview e2e run, not this unit test.
func TestE2ESeedHandler_Gating(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.E2ESeedSecret = "s3cret"
	h := e2eSeedHandler(cfg, nil)

	do := func(method, secret string) int {
		req := httptest.NewRequest(method, "/internal/e2e/seed", nil)
		if secret != "" {
			req.Header.Set("X-E2E-Seed-Secret", secret)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec.Code
	}

	if got := do(http.MethodGet, "s3cret"); got != http.StatusMethodNotAllowed {
		t.Errorf("GET with correct secret: got %d, want 405", got)
	}
	if got := do(http.MethodPost, ""); got != http.StatusForbidden {
		t.Errorf("POST with no secret: got %d, want 403", got)
	}
	if got := do(http.MethodPost, "wrong"); got != http.StatusForbidden {
		t.Errorf("POST with wrong secret: got %d, want 403", got)
	}
}
