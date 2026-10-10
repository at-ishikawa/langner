package serverapp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchGlobOrigin(t *testing.T) {
	const pat = "https://langner-git-*-at-ishikawa.vercel.app"
	cases := []struct {
		origin string
		want   bool
	}{
		{"https://langner-git-fix-stateless-at-ishikawa.vercel.app", true},
		{"https://langner-git-feat-x-at-ishikawa.vercel.app", true},
		// wrong account scope — the anchored suffix is what keeps the wildcard safe.
		{"https://langner-git-x-attacker.vercel.app", false},
		// different project prefix.
		{"https://evil-git-x-at-ishikawa.vercel.app", false},
		{"https://evil.com", false},
		// suffix must be anchored at the end, not just contained.
		{"https://langner-git-x-at-ishikawa.vercel.app.evil.com", false},
		{"", false},
	}
	for _, c := range cases {
		if got := matchGlobOrigin(pat, c.origin); got != c.want {
			t.Errorf("matchGlobOrigin(%q, %q) = %v, want %v", pat, c.origin, got, c.want)
		}
	}
	// A pattern with no '*' is an exact match.
	if !matchGlobOrigin("https://a.com", "https://a.com") || matchGlobOrigin("https://a.com", "https://b.com") {
		t.Error("no-star pattern must be exact")
	}
}

func TestCorsMiddleware_WildcardAndExact(t *testing.T) {
	h := corsMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
		[]string{
			"https://langner.vercel.app",                   // exact (prod)
			"https://langner-git-*-at-ishikawa.vercel.app", // preview glob
		},
	)
	reflected := func(origin string) string {
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Header().Get("Access-Control-Allow-Origin")
	}
	if got := reflected("https://langner.vercel.app"); got != "https://langner.vercel.app" {
		t.Errorf("exact prod origin not reflected: %q", got)
	}
	if got := reflected("https://langner-git-my-branch-at-ishikawa.vercel.app"); got != "https://langner-git-my-branch-at-ishikawa.vercel.app" {
		t.Errorf("preview origin not reflected: %q", got)
	}
	if got := reflected("https://evil.com"); got != "" {
		t.Errorf("disallowed origin was reflected: %q", got)
	}
	if got := reflected("https://langner-git-x-attacker.vercel.app"); got != "" {
		t.Errorf("wrong-scope origin was reflected: %q", got)
	}
}
