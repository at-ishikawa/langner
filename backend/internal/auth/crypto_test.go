package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeKey(t *testing.T) {
	// A 64-char hex string decodes to 32 bytes.
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	assert.Len(t, DecodeKey(hexKey), 32)

	// A non-hex, non-base64 string falls back to raw bytes.
	assert.Equal(t, []byte("plain-string"), DecodeKey("plain-string"))
}

func TestNormalizeEmail(t *testing.T) {
	assert.Equal(t, "user@example.com", NormalizeEmail("  USER@Example.com "))
	assert.Equal(t, "", NormalizeEmail("   "))
}
