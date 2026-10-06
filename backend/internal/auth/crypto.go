// Package auth provides Google-OAuth sign-in and langner bearer tokens: a
// signed access JWT (the only auth credential for both web and CLI), an email
// allowlist, CSRF state, and request-context helpers. No user PII (email, name)
// is stored — the allowlist is checked against the live email Google returns at
// callback time. The one secret persisted at rest is each user's LLM API key,
// encrypted with the Encryptor below (CREDENTIAL_ENCRYPTION_KEY).
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// Encryptor encrypts a secret (the per-user LLM API key) for storage at rest
// using AES-256-GCM with a fresh random nonce prepended to the ciphertext.
type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor builds an Encryptor from a 32-byte AES-256 key.
func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("credential encryption key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Encryptor{gcm: gcm}, nil
}

// Encrypt returns nonce-prefixed AES-256-GCM ciphertext for the plaintext.
func (e *Encryptor) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	return e.gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt reverses Encrypt. It fails (GCM authentication error) if the
// ciphertext was tampered with or produced under a different key.
func (e *Encryptor) Decrypt(ciphertext []byte) (string, error) {
	ns := e.gcm.NonceSize()
	if len(ciphertext) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	plain, err := e.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

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
