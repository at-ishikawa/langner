// Package auth provides Google-OAuth sign-in: a stateless signed session
// cookie, an email allowlist, CSRF state, request-context helpers, and
// encryption-at-rest for user PII.
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// Encryptor encrypts user PII (email, display name) for storage and computes a
// keyed blind index over the email so equality lookups and the "one account
// per email" UNIQUE constraint work without persisting the plaintext address.
//
// Encrypt uses AES-256-GCM with a fresh random nonce prepended to the
// ciphertext. BlindIndex is HMAC-SHA256 over the normalised email, keyed with
// the same credential key.
type Encryptor struct {
	gcm cipher.AEAD
	key []byte
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
	return &Encryptor{gcm: gcm, key: key}, nil
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

// BlindIndex returns a deterministic keyed HMAC-SHA256 hex digest over the
// normalised (trimmed, lowercased) email. Equal emails map to equal digests;
// different emails map to different digests.
func (e *Encryptor) BlindIndex(email string) string {
	mac := hmac.New(sha256.New, e.key)
	mac.Write([]byte(NormalizeEmail(email)))
	return hex.EncodeToString(mac.Sum(nil))
}

// NormalizeEmail trims surrounding whitespace and lowercases an email so that
// blind-index and allowlist comparisons are case-insensitive.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// DecodeKey decodes a key string that may be hex- or base64-encoded, falling
// back to the raw bytes of the string. Used to turn the configured
// SESSION_SIGNING_KEY / CREDENTIAL_ENCRYPTION_KEY strings into key bytes.
func DecodeKey(s string) []byte {
	if b, err := hex.DecodeString(s); err == nil && len(b) > 0 {
		return b
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) > 0 {
		return b
	}
	return []byte(s)
}
