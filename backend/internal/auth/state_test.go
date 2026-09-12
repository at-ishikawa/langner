package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateSigner_IssueVerify(t *testing.T) {
	signer, err := NewStateSigner([]byte("state-key"))
	require.NoError(t, err)
	other, err := NewStateSigner([]byte("other-key"))
	require.NoError(t, err)

	t.Run("valid state verifies", func(t *testing.T) {
		token, err := signer.Issue(time.Minute)
		require.NoError(t, err)
		assert.NoError(t, signer.Verify(token))
	})

	t.Run("two issues differ (fresh nonce)", func(t *testing.T) {
		a, _ := signer.Issue(time.Minute)
		b, _ := signer.Issue(time.Minute)
		assert.NotEqual(t, a, b)
	})

	t.Run("tampered signature rejected", func(t *testing.T) {
		token, _ := signer.Issue(time.Minute)
		assert.Error(t, signer.Verify(token+"x"))
	})

	t.Run("different key rejected", func(t *testing.T) {
		token, _ := other.Issue(time.Minute)
		assert.Error(t, signer.Verify(token))
	})

	t.Run("expired state rejected", func(t *testing.T) {
		token, _ := signer.Issue(-time.Minute)
		assert.Error(t, signer.Verify(token))
	})

	t.Run("malformed rejected", func(t *testing.T) {
		assert.Error(t, signer.Verify("garbage"))
	})
}
