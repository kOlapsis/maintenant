package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
)

// GenerateToken generates a cryptographically random 32-byte token
// and returns the raw hex-encoded token and its SHA-256 hash.
func GenerateToken() (raw string, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	raw = hex.EncodeToString(b)
	hash = HashToken(raw)
	return
}

// HashToken returns the hex-encoded SHA-256 hash of the given token.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// VerifyPKCE checks that SHA256(codeVerifier) matches the codeChallenge (S256 method)
// using constant-time comparison to prevent timing attacks.
func VerifyPKCE(codeVerifier, codeChallenge string) bool {
	h := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(codeChallenge)) == 1
}
