package server

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	"github.com/at-ishikawa/langner/internal/inference"
)

// TestMapGradeError pins the mapping from a grading failure to an actionable
// connect code, replacing the blanket CodeInternal that turned every provider
// failure into an opaque "internal" error.
func TestMapGradeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode connect.Code
	}{
		{
			name:     "no credential -> FailedPrecondition",
			err:      inference.ErrNoCredential,
			wantCode: connect.CodeFailedPrecondition,
		},
		{
			name:     "wrapped no credential -> FailedPrecondition",
			err:      fmt.Errorf("failed to grade answer: %w", inference.ErrNoCredential),
			wantCode: connect.CodeFailedPrecondition,
		},
		{
			name:     "quota provider error -> ResourceExhausted",
			err:      &inference.ProviderError{Provider: "openai", StatusCode: 429, Code: "insufficient_quota"},
			wantCode: connect.CodeResourceExhausted,
		},
		{
			name:     "wrapped quota provider error -> ResourceExhausted",
			err:      fmt.Errorf("item 0: %w", fmt.Errorf("failed to grade answer: %w", &inference.ProviderError{Provider: "openai", StatusCode: 429, Code: "insufficient_quota"})),
			wantCode: connect.CodeResourceExhausted,
		},
		{
			name:     "invalid key provider error -> Unauthenticated",
			err:      &inference.ProviderError{Provider: "openai", StatusCode: 401, Code: "invalid_api_key"},
			wantCode: connect.CodeUnauthenticated,
		},
		{
			name:     "generic provider 500 -> Internal",
			err:      &inference.ProviderError{Provider: "openai", StatusCode: 500},
			wantCode: connect.CodeInternal,
		},
		{
			name:     "plain error -> Internal",
			err:      errors.New("something went wrong"),
			wantCode: connect.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapGradeError(tt.err)
			assert.Equal(t, tt.wantCode, got.Code())
		})
	}
}
