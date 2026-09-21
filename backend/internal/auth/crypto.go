// Package auth provides Google-OAuth sign-in and langner bearer tokens: a
// signed access JWT (the only auth credential for both web and CLI), an email
// allowlist, CSRF state, and request-context helpers. No user PII (email, name)
// is stored — the allowlist is checked against the live email Google returns at
// callback time — so there is no encryption-at-rest here.
package auth

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// NormalizeEmail trims surrounding whitespace and lowercases an email so that
// allowlist comparisons are case-insensitive.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// DecodeKey decodes a key string that may be hex- or base64-encoded, falling
// back to the raw bytes of the string. Used to turn the configured
// TOKEN_SIGNING_KEY string into key bytes.
func DecodeKey(s string) []byte {
	if b, err := hex.DecodeString(s); err == nil && len(b) > 0 {
		return b
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) > 0 {
		return b
	}
	return []byte(s)
}
