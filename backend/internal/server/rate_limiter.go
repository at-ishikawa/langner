package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter is a minimal fixed-window per-key limiter used to throttle the
// device-authorization endpoints (RFC 8628 §5.4 / design §9). It is
// intentionally simple: an in-process map of key → (window start, count).
type rateLimiter struct {
	mu     sync.Mutex
	counts map[string]*windowCount
	limit  int
	window time.Duration
	now    func() time.Time
}

type windowCount struct {
	start time.Time
	count int
}

// newRateLimiter allows up to limit requests per key per window.
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		counts: make(map[string]*windowCount),
		limit:  limit,
		window: window,
		now:    time.Now,
	}
}

// allow reports whether a request for key is within the limit, counting it.
func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	wc, ok := l.counts[key]
	if !ok || now.Sub(wc.start) >= l.window {
		l.counts[key] = &windowCount{start: now, count: 1}
		return true
	}
	if wc.count >= l.limit {
		return false
	}
	wc.count++
	return true
}

// clientIP extracts a best-effort client IP for rate-limiting. It honours a
// single X-Forwarded-For hop (Vercel/edge) then falls back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
