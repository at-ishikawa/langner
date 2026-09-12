package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Session is the authenticated identity carried by the langner_session cookie.
// It deliberately carries only the user id and an expiry — NO email or other
// PII is placed in the browser cookie. Email/name are looked up (and decrypted)
// server-side by user id when needed.
type Session struct {
	UserID    int64
	ExpiresAt time.Time
}

// sessionPayload is the compact JSON form signed into the cookie.
type sessionPayload struct {
	UserID    int64 `json:"uid"`
	ExpiresAt int64 `json:"exp"`
}

// SessionSigner signs and verifies stateless session cookies with HMAC-SHA256.
// The cookie value is base64url(payload) + "." + base64url(hmac(payload)); no
// server-side session store is involved.
type SessionSigner struct {
	key []byte
}

// NewSessionSigner builds a SessionSigner from a non-empty signing key.
func NewSessionSigner(key []byte) (*SessionSigner, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("session signing key is empty")
	}
	return &SessionSigner{key: key}, nil
}

// Sign returns the signed cookie value for the session.
func (s *SessionSigner) Sign(sess Session) (string, error) {
	raw, err := json.Marshal(sessionPayload{UserID: sess.UserID, ExpiresAt: sess.ExpiresAt.Unix()})
	if err != nil {
		return "", fmt.Errorf("marshal session payload: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	return body + "." + s.mac(body), nil
}

// Verify checks the signature and expiry of a signed cookie value and returns
// the session. It rejects tampered payloads, tampered signatures, malformed
// tokens, and expired sessions.
func (s *SessionSigner) Verify(token string) (Session, error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return Session{}, fmt.Errorf("malformed session token")
	}
	if !hmac.Equal([]byte(sig), []byte(s.mac(body))) {
		return Session{}, fmt.Errorf("invalid session signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return Session{}, fmt.Errorf("decode session payload: %w", err)
	}
	var payload sessionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Session{}, fmt.Errorf("unmarshal session payload: %w", err)
	}
	sess := Session{UserID: payload.UserID, ExpiresAt: time.Unix(payload.ExpiresAt, 0)}
	if time.Now().After(sess.ExpiresAt) {
		return Session{}, fmt.Errorf("session expired")
	}
	return sess, nil
}

func (s *SessionSigner) mac(body string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
