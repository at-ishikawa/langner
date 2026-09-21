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

	t.Run("valid state verifies and round-trips next + user_code", func(t *testing.T) {
		token, err := signer.Issue(time.Minute, StateClaims{Next: "http://frontend.example/quiz", UserCode: "WDJB-MJHT"})
		require.NoError(t, err)
		claims, err := signer.Verify(token)
		require.NoError(t, err)
		assert.Equal(t, "http://frontend.example/quiz", claims.Next)
		assert.Equal(t, "WDJB-MJHT", claims.UserCode)
	})

	t.Run("two issues differ (fresh nonce)", func(t *testing.T) {
		a, _ := signer.Issue(time.Minute, StateClaims{})
		b, _ := signer.Issue(time.Minute, StateClaims{})
		assert.NotEqual(t, a, b)
	})

	t.Run("tampered signature rejected", func(t *testing.T) {
		token, _ := signer.Issue(time.Minute, StateClaims{})
		_, err := signer.Verify(token + "x")
		assert.Error(t, err)
	})

	t.Run("different key rejected", func(t *testing.T) {
		token, _ := other.Issue(time.Minute, StateClaims{})
		_, err := signer.Verify(token)
		assert.Error(t, err)
	})

	t.Run("expired state rejected", func(t *testing.T) {
		token, _ := signer.Issue(-time.Minute, StateClaims{})
		_, err := signer.Verify(token)
		assert.Error(t, err)
	})

	t.Run("malformed rejected", func(t *testing.T) {
		_, err := signer.Verify("garbage")
		assert.Error(t, err)
	})
}
