package server

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"github.com/at-ishikawa/langner/internal/inference"
)

// mapGradeError converts a grading failure into an actionable connect error,
// replacing the blanket CodeInternal that previously turned every provider
// failure into an opaque "internal" error. It is the ONE place every grade
// handler (standard / reverse / freeform / grammar / relearn) maps a grade
// error, so the mapping stays consistent across quiz modes:
//
//   - no credential      -> FailedPrecondition ("add a key in Settings")
//   - quota / no credits -> ResourceExhausted  ("key has no credits")
//   - invalid API key    -> Unauthenticated    ("key is invalid")
//   - anything else      -> Internal (unchanged)
func mapGradeError(err error) *connect.Error {
	if errors.Is(err, inference.ErrNoCredential) {
		return connect.NewError(connect.CodeFailedPrecondition,
			errors.New("No API key configured. Add one in Settings to grade answers."))
	}
	var pe *inference.ProviderError
	if errors.As(err, &pe) {
		if pe.IsQuota() {
			return connect.NewError(connect.CodeResourceExhausted,
				fmt.Errorf("Your %s API key has no remaining credits — update it in Settings.", pe.Provider))
		}
		if pe.IsInvalidKey() {
			return connect.NewError(connect.CodeUnauthenticated,
				fmt.Errorf("Your %s API key is invalid — update it in Settings.", pe.Provider))
		}
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
}
