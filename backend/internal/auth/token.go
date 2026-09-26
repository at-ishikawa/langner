package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// tokenIssuer is the fixed `iss` claim on every access JWT.
	tokenIssuer = "langner"
	// AccessTokenTTL is the fixed lifetime of a minted access JWT (24h). The
	// short lifetime bounds exposure since access JWTs are stateless and not
	// individually revocable before exp.
	AccessTokenTTL = 24 * time.Hour
)

// AccessClaims is the langner access token payload. It carries only the user
// id (as the standard `sub`), the issuer, timestamps, and a random `jti` — NO
// email or name (the no-PII rule migration 024 established). Aud/scope are
// omitted (single audience, single scope).
type AccessClaims struct {
	UserID    int64
	IssuedAt  time.Time
	ExpiresAt time.Time
	JTI       string
}

// TokenSigner signs and verifies langner access JWTs with HS256, pinning the
// algorithm so a token presenting `alg: none` (or any non-HMAC alg) is
// rejected. It is keyed by TOKEN_SIGNING_KEY, decoded via DecodeKey.
type TokenSigner struct {
	key []byte
}

// NewTokenSigner builds a TokenSigner from a non-empty signing key.
func NewTokenSigner(key []byte) (*TokenSigner, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("token signing key is empty")
	}
	return &TokenSigner{key: key}, nil
}

// Sign mints a 24h HS256 access JWT for the user id. The caller passes now so
// tests can pin time; production passes time.Now().
func (s *TokenSigner) Sign(userID int64, now time.Time) (string, error) {
	jti, err := randomJTI()
	if err != nil {
		return "", err
	}
	claims := jwt.RegisteredClaims{
		Issuer:    tokenIssuer,
		Subject:   strconv.FormatInt(userID, 10),
		ID:        jti,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.key)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

// Verify checks the signature, algorithm (HS256 only), issuer, and expiry of an
// access JWT and returns its claims. It rejects tampered/expired tokens and any
// token not signed with HS256 (defeating the `alg: none` attack).
func (s *TokenSigner) Verify(tokenString string) (AccessClaims, error) {
	var registered jwt.RegisteredClaims
	parsed, err := jwt.ParseWithClaims(tokenString, &registered, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(tokenIssuer))
	if err != nil {
		return AccessClaims{}, fmt.Errorf("verify access token: %w", err)
	}
	if !parsed.Valid {
		return AccessClaims{}, fmt.Errorf("invalid access token")
	}
	userID, err := strconv.ParseInt(registered.Subject, 10, 64)
	if err != nil {
		return AccessClaims{}, fmt.Errorf("access token subject not a user id: %w", err)
	}
	claims := AccessClaims{UserID: userID, JTI: registered.ID}
	if registered.IssuedAt != nil {
		claims.IssuedAt = registered.IssuedAt.Time
	}
	if registered.ExpiresAt != nil {
		claims.ExpiresAt = registered.ExpiresAt.Time
	}
	return claims, nil
}

// randomJTI returns 16 random bytes base64url-encoded — a per-token identity
// provisioned for future denylisting/audit.
func randomJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("read jti: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
