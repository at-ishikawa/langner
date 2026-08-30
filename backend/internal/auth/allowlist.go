package auth

// IsAllowed reports whether email is on the allowlist. Comparison is
// case-insensitive and ignores surrounding whitespace on both sides. The email
// passed in MUST be the plaintext address returned by Google at callback time —
// never a stored/decrypted value — so encryption-at-rest never affects
// allowlisting.
func IsAllowed(email string, allowed []string) bool {
	target := NormalizeEmail(email)
	if target == "" {
		return false
	}
	for _, a := range allowed {
		if NormalizeEmail(a) == target {
			return true
		}
	}
	return false
}
