package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAllowed(t *testing.T) {
	allowed := []string{"admin@example.com", "  Second.User@Example.com "}

	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{name: "exact match", email: "admin@example.com", want: true},
		{name: "case-insensitive match", email: "ADMIN@example.com", want: true},
		{name: "whitespace trimmed on both sides", email: "  second.user@example.com  ", want: true},
		{name: "not on the list", email: "intruder@example.com", want: false},
		{name: "empty email", email: "", want: false},
		{name: "empty against empty allowlist entry not matched", email: "  ", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsAllowed(tt.email, allowed))
		})
	}
}
