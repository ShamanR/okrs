// Package auth holds what the OAuth entry points share: validating the post-login
// redirect target and minting the CSRF state parameter. The URI packages beneath it
// (auth/start, auth/callback) hold one endpoint each.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// safeRedirectPath returns true only for relative paths on this host,
// preventing open-redirect attacks via a crafted next parameter.
func SafeRedirectPath(next string) bool {
	if next == "" {
		return false
	}
	// Must start with / but not // (protocol-relative URL)
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return false
	}
	return true
}
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
