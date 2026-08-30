package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionSigner_SignVerify(t *testing.T) {
	signer, err := NewSessionSigner([]byte("test-session-signing-key"))
	require.NoError(t, err)
	other, err := NewSessionSigner([]byte("a-completely-different-key"))
	require.NoError(t, err)

	future := time.Now().Add(time.Hour).Truncate(time.Second)
	past := time.Now().Add(-time.Hour).Truncate(time.Second)

	tests := []struct {
		name    string
		build   func() string
		wantErr bool
		wantID  int64
	}{
		{
			name: "happy path round-trips user id",
			build: func() string {
				v, _ := signer.Sign(Session{UserID: 42, ExpiresAt: future})
				return v
			},
			wantID: 42,
		},
		{
			name: "tampered payload fails signature check",
			build: func() string {
				v, _ := signer.Sign(Session{UserID: 42, ExpiresAt: future})
				// Flip a character in the payload body (before the dot).
				b := []byte(v)
				b[0] ^= 0xFF
				return string(b)
			},
			wantErr: true,
		},
		{
			name: "tampered signature fails",
			build: func() string {
				v, _ := signer.Sign(Session{UserID: 42, ExpiresAt: future})
				return v + "x"
			},
			wantErr: true,
		},
		{
			name: "signed with a different key fails",
			build: func() string {
				v, _ := other.Sign(Session{UserID: 42, ExpiresAt: future})
				return v
			},
			wantErr: true,
		},
		{
			name: "expired session rejected",
			build: func() string {
				v, _ := signer.Sign(Session{UserID: 42, ExpiresAt: past})
				return v
			},
			wantErr: true,
		},
		{
			name:    "malformed token rejected",
			build:   func() string { return "not-a-valid-token" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess, err := signer.Verify(tt.build())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, sess.UserID)
		})
	}
}
