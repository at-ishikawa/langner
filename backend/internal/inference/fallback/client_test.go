package fallback

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/inference"
)

// fakeClient is a spy inference.Client: it returns a preset error from every
// method and records how many times each was called, tagging its results with
// `name` so a test can tell which provider answered.
type fakeClient struct {
	name          string
	err           error
	answerCalls   int
	validateCalls int
	batchCalls    int
	lookupCalls   int
	gradeCalls    int
}

func (f *fakeClient) AnswerMeanings(context.Context, inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
	f.answerCalls++
	return inference.AnswerMeaningsResponse{Answers: []inference.AnswerMeaning{{Expression: f.name}}}, f.err
}
func (f *fakeClient) ValidateWordForm(context.Context, inference.ValidateWordFormRequest) (inference.ValidateWordFormResponse, error) {
	f.validateCalls++
	return inference.ValidateWordFormResponse{Reason: f.name}, f.err
}
func (f *fakeClient) ValidateWordFormBatch(_ context.Context, params []inference.ValidateWordFormRequest) ([]inference.ValidateWordFormResponse, error) {
	f.batchCalls++
	out := make([]inference.ValidateWordFormResponse, len(params))
	for i := range params {
		out[i] = inference.ValidateWordFormResponse{Index: i, Reason: f.name}
	}
	return out, f.err
}
func (f *fakeClient) LookupWord(context.Context, inference.LookupWordRequest) (inference.LookupWordResponse, error) {
	f.lookupCalls++
	return inference.LookupWordResponse{}, f.err
}
func (f *fakeClient) GradeCorrection(context.Context, inference.GradeCorrectionRequest) (inference.GradeCorrectionResponse, error) {
	f.gradeCalls++
	return inference.GradeCorrectionResponse{Reason: f.name}, f.err
}

func TestFallback_PrimarySucceeds_SecondaryUntouched(t *testing.T) {
	primary := &fakeClient{name: "primary", err: nil}
	secondary := &fakeClient{name: "secondary"}
	c := NewClient(primary, secondary)

	res, err := c.AnswerMeanings(context.Background(), inference.AnswerMeaningsRequest{})
	require.NoError(t, err)
	assert.Equal(t, "primary", res.Answers[0].Expression)
	assert.Equal(t, 1, primary.answerCalls)
	assert.Equal(t, 0, secondary.answerCalls, "secondary must not be called when primary succeeds")
}

func TestFallback_RateLimit_FailsOverToSecondary(t *testing.T) {
	rateLimit := fmt.Errorf("grade answer: %w", errors.New("response error 429: Too Many Requests"))
	primary := &fakeClient{name: "primary", err: rateLimit}
	secondary := &fakeClient{name: "secondary", err: nil}
	c := NewClient(primary, secondary)

	res, err := c.GradeCorrection(context.Background(), inference.GradeCorrectionRequest{})
	require.NoError(t, err, "the secondary answered, so the call succeeds")
	assert.Equal(t, "secondary", res.Reason)
	assert.Equal(t, 1, primary.gradeCalls)
	assert.Equal(t, 1, secondary.gradeCalls)
}

func TestFallback_QuotaSignals_FailOver(t *testing.T) {
	for _, sig := range []string{
		"insufficient_quota",
		"RESOURCE_EXHAUSTED: quota exceeded",
		"429 rate limit reached",
		"too many requests",
	} {
		primary := &fakeClient{name: "primary", err: errors.New(sig)}
		secondary := &fakeClient{name: "secondary"}
		c := NewClient(primary, secondary)
		_, err := c.ValidateWordForm(context.Background(), inference.ValidateWordFormRequest{})
		require.NoError(t, err, "signal %q should fail over", sig)
		assert.Equal(t, 1, secondary.validateCalls, "signal %q should reach the secondary", sig)
	}
}

func TestFallback_NonRateLimitError_NoFailover(t *testing.T) {
	boom := errors.New("failed to grade answer: connection reset by peer")
	primary := &fakeClient{name: "primary", err: boom}
	secondary := &fakeClient{name: "secondary"}
	c := NewClient(primary, secondary)

	_, err := c.AnswerMeanings(context.Background(), inference.AnswerMeaningsRequest{})
	require.ErrorIs(t, err, boom, "a non-rate-limit error is returned from the primary as-is")
	assert.Equal(t, 0, secondary.answerCalls, "no fallback on a non-rate-limit error")
}

func TestFallback_MalformedBatch_NoFailover(t *testing.T) {
	// A malformed batch is NOT a quota problem: it must NOT fail over (the caller
	// has its own per-item fallback). It stays a single primary batch call.
	primary := &fakeClient{name: "primary", err: fmt.Errorf("batch parse: %w", inference.ErrMalformedResponse)}
	secondary := &fakeClient{name: "secondary"}
	c := NewClient(primary, secondary)

	_, err := c.ValidateWordFormBatch(context.Background(), []inference.ValidateWordFormRequest{{}, {}})
	require.ErrorIs(t, err, inference.ErrMalformedResponse)
	assert.Equal(t, 1, primary.batchCalls)
	assert.Equal(t, 0, secondary.batchCalls, "malformed batch must not fan out to the secondary")
}

func TestFallback_RateLimitedBatch_RetriesWholeBatchOnce(t *testing.T) {
	// A rate-limited batch retries as ONE batch call on the secondary — never N
	// per-item calls (which would re-burst the quota).
	primary := &fakeClient{name: "primary", err: errors.New("response error 429")}
	secondary := &fakeClient{name: "secondary", err: nil}
	c := NewClient(primary, secondary)

	res, err := c.ValidateWordFormBatch(context.Background(), []inference.ValidateWordFormRequest{{}, {}, {}})
	require.NoError(t, err)
	require.Len(t, res, 3)
	assert.Equal(t, "secondary", res[0].Reason)
	assert.Equal(t, 1, primary.batchCalls)
	assert.Equal(t, 1, secondary.batchCalls, "exactly one secondary batch call, not per-item")
}
