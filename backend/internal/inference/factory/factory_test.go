package factory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/openai"
)

// fakeCreds is a credentialsReader that records how many times it was asked to
// decrypt, so a test can assert the resolver cache avoids repeat DB/decrypt work.
type fakeCreds struct {
	calls int
	creds []auth.LLMCredential
	err   error
}

func (f *fakeCreds) DecryptedCredentials(_ context.Context, _ int64) ([]auth.LLMCredential, error) {
	f.calls++
	return f.creds, f.err
}

// newTestResolver builds a Resolver over a fake credential reader with a
// controllable clock so TTL expiry is deterministic.
func newTestResolver(reader credentialsReader, ttl time.Duration, now *time.Time) *Resolver {
	return &Resolver{
		credentials: reader,
		ttl:         ttl,
		now:         func() time.Time { return *now },
		cache:       make(map[int64]cachedClient),
	}
}

func TestResolverCachesPerUserClient(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	oneKey := []auth.LLMCredential{{Provider: "openai", APIKey: "sk-a", Model: "gpt-4o-mini", IsPrimary: true}}

	t.Run("reuses the cached client within the TTL", func(t *testing.T) {
		clock := now
		reader := &fakeCreds{creds: oneKey}
		r := newTestResolver(reader, time.Minute, &clock)

		c1, err := r.ResolveClient(context.Background(), 7)
		require.NoError(t, err)
		clock = clock.Add(30 * time.Second) // still inside the TTL
		c2, err := r.ResolveClient(context.Background(), 7)
		require.NoError(t, err)

		assert.Equal(t, 1, reader.calls, "second resolve within TTL must not re-decrypt")
		assert.Same(t, c1, c2, "the same cached client instance is returned")
	})

	t.Run("rebuilds after the TTL expires", func(t *testing.T) {
		clock := now
		reader := &fakeCreds{creds: oneKey}
		r := newTestResolver(reader, time.Minute, &clock)

		_, err := r.ResolveClient(context.Background(), 7)
		require.NoError(t, err)
		clock = clock.Add(2 * time.Minute) // past the TTL
		_, err = r.ResolveClient(context.Background(), 7)
		require.NoError(t, err)

		assert.Equal(t, 2, reader.calls, "an expired entry must be rebuilt from fresh credentials")
	})

	t.Run("caches per user independently", func(t *testing.T) {
		clock := now
		reader := &fakeCreds{creds: oneKey}
		r := newTestResolver(reader, time.Minute, &clock)

		_, err := r.ResolveClient(context.Background(), 1)
		require.NoError(t, err)
		_, err = r.ResolveClient(context.Background(), 2)
		require.NoError(t, err)

		assert.Equal(t, 2, reader.calls, "distinct users each resolve once")
	})

	t.Run("Invalidate forces a rebuild before the TTL", func(t *testing.T) {
		clock := now
		reader := &fakeCreds{creds: oneKey}
		r := newTestResolver(reader, time.Minute, &clock)

		_, err := r.ResolveClient(context.Background(), 7)
		require.NoError(t, err)
		r.Invalidate(7)
		_, err = r.ResolveClient(context.Background(), 7)
		require.NoError(t, err)

		assert.Equal(t, 2, reader.calls, "Invalidate must drop the cached client")
	})

	t.Run("does not cache a no-credential result", func(t *testing.T) {
		clock := now
		reader := &fakeCreds{creds: nil} // user has registered no key
		r := newTestResolver(reader, time.Minute, &clock)

		_, err := r.ResolveClient(context.Background(), 7)
		require.ErrorIs(t, err, inference.ErrNoCredential)
		_, err = r.ResolveClient(context.Background(), 7)
		require.ErrorIs(t, err, inference.ErrNoCredential)

		assert.Equal(t, 2, reader.calls, "a user who just added a key must not be told 'no credential' for a whole TTL")
	})

	t.Run("does not cache a build error", func(t *testing.T) {
		clock := now
		reader := &fakeCreds{err: errors.New("db down")}
		r := newTestResolver(reader, time.Minute, &clock)

		_, err := r.ResolveClient(context.Background(), 7)
		require.Error(t, err)
		_, err = r.ResolveClient(context.Background(), 7)
		require.Error(t, err)

		assert.Equal(t, 2, reader.calls, "a transient failure must not be cached")
	})
}

func TestBuild(t *testing.T) {
	t.Run("openai with explicit model", func(t *testing.T) {
		client, err := Build("openai", "sk-test", "gpt-4")
		require.NoError(t, err)
		oc, ok := client.(*openai.Client)
		require.True(t, ok, "openai provider must build an *openai.Client")
		assert.Equal(t, "gpt-4", oc.GetModel())
	})

	t.Run("openai with empty model falls back to default", func(t *testing.T) {
		client, err := Build("openai", "sk-test", "")
		require.NoError(t, err)
		oc, ok := client.(*openai.Client)
		require.True(t, ok)
		assert.Equal(t, defaultOpenAIModel, oc.GetModel())
	})

	t.Run("gemini with empty model falls back to default", func(t *testing.T) {
		client, err := Build("gemini", "key", "")
		require.NoError(t, err)
		oc, ok := client.(*openai.Client)
		require.True(t, ok, "gemini provider builds on the OpenAI-compatible client")
		assert.Equal(t, defaultGeminiModel, oc.GetModel())
	})

	t.Run("unsupported provider errors", func(t *testing.T) {
		_, err := Build("anthropic", "key", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported provider")
	})
}
