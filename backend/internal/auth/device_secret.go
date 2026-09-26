package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// userCodeAlphabet is an unambiguous Crockford-style alphabet (RFC 8628 §6.1
// recommends excluding easily-confused characters). It drops 0/1 and the
// letters they resemble.
const userCodeAlphabet = "BCDFGHJKLMNPQRSTVWXZ23456789"

// GenerateDeviceCode returns an opaque 32-byte base64url device secret (~43
// chars). It is a bearer secret during polling and is stored HASHED via
// HashSecret — never in plaintext.
func GenerateDeviceCode() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("read device code: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateRefreshToken returns an opaque 32-byte base64url refresh token,
// stored HASHED server-side and returned to the client once.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("read refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateUserCode returns an 8-character human code rendered as XXXX-XXXX from
// the unambiguous alphabet. ~28^8 space; combined with a 15-min expiry and rate
// limiting it is impractical to brute-force on the approval page.
func GenerateUserCode() (string, error) {
	const n = 8
	out := make([]byte, 0, n+1)
	buf := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("read user code: %w", err)
	}
	for i, b := range buf {
		if i == n/2 {
			out = append(out, '-')
		}
		out = append(out, userCodeAlphabet[int(b)%len(userCodeAlphabet)])
	}
	return string(out), nil
}

// HashSecret returns the lowercase hex SHA-256 of an opaque secret (device code
// or refresh token). Lookups are by this hash so plaintext is never at rest.
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// NormalizeUserCode upper-cases and trims a user_code so approval-page lookups
// are case-insensitive (the DB stores the canonical upper-case form).
func NormalizeUserCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
