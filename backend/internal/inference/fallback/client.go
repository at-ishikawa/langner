// Package fallback provides an inference.Client decorator that tries a primary
// provider and, only when the primary fails with a rate-limit / quota error,
// retries the SAME call once on a secondary provider. It is wired once in
// main.go, so every inference call site (quiz grading, relearn, grammar, word
// lookup) transparently gains provider failover without any per-call changes.
package fallback

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/at-ishikawa/langner/internal/inference"
)

// Client wraps a primary and secondary inference.Client. Every method calls the
// primary first; only a rate-limit/quota error (see shouldFallback) triggers a
// single retry of the same call on the secondary. Any other error — including a
// malformed batch response or an ordinary grading verdict — is returned from the
// primary unchanged.
type Client struct {
	primary   inference.Client
	secondary inference.Client
}

// NewClient wraps primary with a rate-limit fallback to secondary. Both must be
// non-nil (main.go only builds this when two providers are configured).
func NewClient(primary, secondary inference.Client) *Client {
	return &Client{primary: primary, secondary: secondary}
}

// shouldFallback reports whether err from the primary is a rate-limit / quota
// exhaustion that a DIFFERENT provider might not be hitting — the only class of
// failure worth re-issuing against a second provider. It matches the signals the
// OpenAI/Gemini clients surface (HTTP 429, insufficient_quota, RESOURCE_EXHAUSTED,
// "rate limit"/"too many requests") by substring, since these providers return
// plain (untyped) errors here. A malformed-response error is explicitly NOT a
// fallback trigger: it is not a quota problem, and batch callers have their own
// per-item fallback for it.
func shouldFallback(err error) bool {
	if err == nil || errors.Is(err, inference.ErrMalformedResponse) {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, sig := range []string{
		"response error 429",
		"insufficient_quota",
		"resource_exhausted",
		"rate limit",
		"rate_limit",
		"too many requests",
	} {
		if strings.Contains(msg, sig) {
			return true
		}
	}
	return false
}

func (c *Client) AnswerMeanings(ctx context.Context, params inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
	r, err := c.primary.AnswerMeanings(ctx, params)
	if shouldFallback(err) {
		logFallback("AnswerMeanings", err)
		return c.secondary.AnswerMeanings(ctx, params)
	}
	return r, err
}

func (c *Client) ValidateWordForm(ctx context.Context, params inference.ValidateWordFormRequest) (inference.ValidateWordFormResponse, error) {
	r, err := c.primary.ValidateWordForm(ctx, params)
	if shouldFallback(err) {
		logFallback("ValidateWordForm", err)
		return c.secondary.ValidateWordForm(ctx, params)
	}
	return r, err
}

// ValidateWordFormBatch retries the WHOLE batch on the secondary (a single
// call), never fanning out into N per-item calls — preserving the batch's
// quota-protecting contract (see inference.Client).
func (c *Client) ValidateWordFormBatch(ctx context.Context, params []inference.ValidateWordFormRequest) ([]inference.ValidateWordFormResponse, error) {
	r, err := c.primary.ValidateWordFormBatch(ctx, params)
	if shouldFallback(err) {
		logFallback("ValidateWordFormBatch", err)
		return c.secondary.ValidateWordFormBatch(ctx, params)
	}
	return r, err
}

func (c *Client) LookupWord(ctx context.Context, params inference.LookupWordRequest) (inference.LookupWordResponse, error) {
	r, err := c.primary.LookupWord(ctx, params)
	if shouldFallback(err) {
		logFallback("LookupWord", err)
		return c.secondary.LookupWord(ctx, params)
	}
	return r, err
}

func (c *Client) GradeCorrection(ctx context.Context, params inference.GradeCorrectionRequest) (inference.GradeCorrectionResponse, error) {
	r, err := c.primary.GradeCorrection(ctx, params)
	if shouldFallback(err) {
		logFallback("GradeCorrection", err)
		return c.secondary.GradeCorrection(ctx, params)
	}
	return r, err
}

func logFallback(method string, err error) {
	slog.Warn("inference: primary provider rate-limited; retrying on the fallback provider",
		"method", method, "error", err)
}
