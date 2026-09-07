package inference

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClient is a stand-in inference.Client for resolver tests. It is never
// called — the resolver only needs to hand it back.
type fakeClient struct{ Client }

func TestStaticResolver(t *testing.T) {
	t.Run("returns the wrapped client", func(t *testing.T) {
		want := fakeClient{}
		got, err := StaticResolver(want).ResolveClient(context.Background(), 0)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("nil client resolves to ErrNoCredential", func(t *testing.T) {
		_, err := StaticResolver(nil).ResolveClient(context.Background(), 42)
		assert.True(t, errors.Is(err, ErrNoCredential))
	})
}

func TestAvailableProviders(t *testing.T) {
	assert.Equal(t, []string{ProviderOpenAI}, AvailableProviders())
	assert.True(t, IsAvailableProvider("openai"))
	assert.False(t, IsAvailableProvider("gemini"))
	assert.False(t, IsAvailableProvider(""))
}
