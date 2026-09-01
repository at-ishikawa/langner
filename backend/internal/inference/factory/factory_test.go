package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/inference/openai"
)

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

	t.Run("unsupported provider errors", func(t *testing.T) {
		_, err := Build("gemini", "key", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported provider")
	})
}
