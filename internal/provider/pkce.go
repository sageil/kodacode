package provider

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// PKCE holds a code_verifier / code_challenge pair for OAuth PKCE.
type PKCE struct {
	Verifier  string
	Challenge string
}

// GeneratePKCE creates a PKCE code_verifier (43-128 chars, base64url-encoded
// random bytes) and the corresponding S256 code_challenge.
func GeneratePKCE() (PKCE, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return PKCE{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])
	return PKCE{Verifier: verifier, Challenge: challenge}, nil
}
