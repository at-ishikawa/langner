package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenSigner_SignVerify(t *testing.T) {
	signer, err := NewTokenSigner([]byte("token-key"))
	require.NoError(t, err)

	now := time.Now()
	tok, err := signer.Sign(42, now)
	require.NoError(t, err)

	claims, err := signer.Verify(tok)
	require.NoError(t, err)
	assert.Equal(t, int64(42), claims.UserID)
	assert.NotEmpty(t, claims.JTI)
	assert.WithinDuration(t, now.Add(AccessTokenTTL), claims.ExpiresAt, time.Second)
}

func TestTokenSigner_RejectsWrongKey(t *testing.T) {
	a, _ := NewTokenSigner([]byte("key-a"))
	b, _ := NewTokenSigner([]byte("key-b"))
	tok, err := a.Sign(1, time.Now())
	require.NoError(t, err)
	_, err = b.Verify(tok)
	assert.Error(t, err, "a token signed by another key must not verify")
}

func TestTokenSigner_RejectsExpired(t *testing.T) {
	signer, _ := NewTokenSigner([]byte("k"))
	tok, err := signer.Sign(1, time.Now().Add(-2*AccessTokenTTL))
	require.NoError(t, err)
	_, err = signer.Verify(tok)
	assert.Error(t, err, "an expired token must not verify")
}

// TestTokenSigner_RejectsAlgNone proves the alg is pinned: a token forged with
// alg=none (unsigned) is rejected even though its claims are well-formed.
func TestTokenSigner_RejectsAlgNone(t *testing.T) {
	signer, _ := NewTokenSigner([]byte("k"))
	claims := jwt.RegisteredClaims{
		Issuer:    tokenIssuer,
		Subject:   "1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	raw, err := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	_, err = signer.Verify(raw)
	assert.Error(t, err, "alg=none must be rejected")
}

func TestTokenSigner_RejectsWrongIssuer(t *testing.T) {
	signer, _ := NewTokenSigner([]byte("k"))
	claims := jwt.RegisteredClaims{
		Issuer:    "someone-else",
		Subject:   "1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("k"))
	require.NoError(t, err)
	_, err = signer.Verify(tok)
	assert.Error(t, err, "a token with the wrong issuer must be rejected")
}

func TestNewTokenSigner_EmptyKey(t *testing.T) {
	_, err := NewTokenSigner(nil)
	assert.Error(t, err)
}

func TestGenerateUserCode_Format(t *testing.T) {
	code, err := GenerateUserCode()
	require.NoError(t, err)
	assert.Len(t, code, 9, "XXXX-XXXX")
	assert.Equal(t, byte('-'), code[4])
	for i, c := range code {
		if i == 4 {
			continue
		}
		assert.True(t, strings.ContainsRune(userCodeAlphabet, c), "char %q not in alphabet", c)
	}
}

func TestGenerateSecrets_Distinct(t *testing.T) {
	a, err := GenerateDeviceCode()
	require.NoError(t, err)
	b, err := GenerateDeviceCode()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
	assert.NotEqual(t, HashSecret(a), HashSecret(b))
	assert.Equal(t, HashSecret(a), HashSecret(a), "hash is deterministic")
}
