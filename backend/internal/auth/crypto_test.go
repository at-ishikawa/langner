package auth

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef") // 32 bytes
}

func TestEncryptor_EncryptDecrypt(t *testing.T) {
	enc, err := NewEncryptor(testKey())
	require.NoError(t, err)

	tests := []struct {
		name      string
		plaintext string
	}{
		{name: "email", plaintext: "user@example.com"},
		{name: "name with unicode", plaintext: "Renée Müller"},
		{name: "empty", plaintext: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, err := enc.Encrypt(tt.plaintext)
			require.NoError(t, err)
			// Ciphertext must never equal the plaintext bytes (proves no leak).
			assert.NotEqual(t, []byte(tt.plaintext), ct)
			assert.False(t, bytes.Contains(ct, []byte(tt.plaintext)) && tt.plaintext != "",
				"ciphertext must not contain the plaintext")

			got, err := enc.Decrypt(ct)
			require.NoError(t, err)
			assert.Equal(t, tt.plaintext, got)
		})
	}
}

func TestEncryptor_EncryptUsesFreshNonce(t *testing.T) {
	enc, err := NewEncryptor(testKey())
	require.NoError(t, err)

	a, err := enc.Encrypt("user@example.com")
	require.NoError(t, err)
	b, err := enc.Encrypt("user@example.com")
	require.NoError(t, err)
	// Same plaintext, different ciphertext because of the random nonce.
	assert.NotEqual(t, a, b)
}

func TestEncryptor_DecryptTamperedFails(t *testing.T) {
	enc, err := NewEncryptor(testKey())
	require.NoError(t, err)

	ct, err := enc.Encrypt("user@example.com")
	require.NoError(t, err)

	// Flip the last byte of the ciphertext — GCM authentication must reject it.
	tampered := append([]byte{}, ct...)
	tampered[len(tampered)-1] ^= 0xFF
	_, err = enc.Decrypt(tampered)
	assert.Error(t, err)

	// A ciphertext produced under a different key must also fail to decrypt.
	other, err := NewEncryptor([]byte("ffffffffffffffffffffffffffffffff"))
	require.NoError(t, err)
	otherCT, err := other.Encrypt("user@example.com")
	require.NoError(t, err)
	_, err = enc.Decrypt(otherCT)
	assert.Error(t, err)
}

func TestNewEncryptor_KeyLength(t *testing.T) {
	_, err := NewEncryptor([]byte("too-short"))
	assert.Error(t, err)
	_, err = NewEncryptor(testKey())
	assert.NoError(t, err)
}

func TestEncryptor_BlindIndex(t *testing.T) {
	enc, err := NewEncryptor(testKey())
	require.NoError(t, err)

	// Deterministic and case/whitespace-insensitive.
	base := enc.BlindIndex("user@example.com")
	assert.Equal(t, base, enc.BlindIndex("user@example.com"))
	assert.Equal(t, base, enc.BlindIndex("  USER@Example.com "))

	// Different emails produce different digests.
	assert.NotEqual(t, base, enc.BlindIndex("other@example.com"))

	// Keyed: a different key yields a different digest for the same email.
	other, err := NewEncryptor([]byte("ffffffffffffffffffffffffffffffff"))
	require.NoError(t, err)
	assert.NotEqual(t, base, other.BlindIndex("user@example.com"))
}

func TestDecodeKey(t *testing.T) {
	// A 64-char hex string decodes to 32 bytes.
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	assert.Len(t, DecodeKey(hexKey), 32)

	// A non-hex, non-base64 string falls back to raw bytes.
	assert.Equal(t, []byte("plain-string"), DecodeKey("plain-string"))
}
