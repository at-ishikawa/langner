package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// StateSigner mints and verifies the short-lived, signed CSRF state that
// protects the OAuth redirect round-trip. The same token is both passed to
// Google as the `state` parameter and stored in a temporary cookie; the
// callback verifies the signature, the expiry, and that the two match.
type StateSigner struct {
	key []byte
}

type statePayload struct {
	Nonce     string `json:"n"`
	ExpiresAt int64  `json:"exp"`
}

// NewStateSigner builds a StateSigner from a non-empty signing key.
func NewStateSigner(key []byte) (*StateSigner, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("state signing key is empty")
	}
	return &StateSigner{key: key}, nil
}

// Issue returns a fresh signed state token valid for ttl.
func (s *StateSigner) Issue(ttl time.Duration) (string, error) {
	nonce := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read state nonce: %w", err)
	}
	raw, err := json.Marshal(statePayload{
		Nonce:     base64.RawURLEncoding.EncodeToString(nonce),
		ExpiresAt: time.Now().Add(ttl).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal state payload: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	return body + "." + s.mac(body), nil
}

// Verify checks a state token's signature and expiry.
func (s *StateSigner) Verify(token string) error {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return fmt.Errorf("malformed state token")
	}
	if !hmac.Equal([]byte(sig), []byte(s.mac(body))) {
		return fmt.Errorf("invalid state signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return fmt.Errorf("decode state payload: %w", err)
	}
	var payload statePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("unmarshal state payload: %w", err)
	}
	if time.Now().After(time.Unix(payload.ExpiresAt, 0)) {
		return fmt.Errorf("state expired")
	}
	return nil
}

func (s *StateSigner) mac(body string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
