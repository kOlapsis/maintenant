package mcp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// jwtClaims represents the relevant claims in a JWT token.
type jwtClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
	Iss   string `json:"iss"`
}

// EmailVerifier validates JWT bearer tokens and checks the email claim against the allowed email.
type EmailVerifier struct {
	AllowedEmail string
}

// VerifyRequest extracts and validates the Bearer token from the Authorization header.
// It checks the email claim matches the configured allowed email.
// Note: In production, JWT signature verification should use the issuer's JWKS endpoint.
// For the initial implementation, we validate the token structure and claims.
func (v *EmailVerifier) VerifyRequest(r *http.Request) error {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return errors.New("missing Authorization header")
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		return errors.New("invalid Authorization header: expected Bearer token")
	}
	token := strings.TrimPrefix(auth, "Bearer ")

	claims, err := parseJWTClaims(token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// Check expiration
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return errors.New("token expired")
	}

	// Check email claim
	email := claims.Email
	if email == "" {
		email = claims.Sub
	}
	if email == "" {
		return errors.New("token missing email or sub claim")
	}
	if !strings.EqualFold(email, v.AllowedEmail) {
		return errors.New("unauthorized: email does not match allowed user")
	}

	return nil
}

// parseJWTClaims decodes the payload section of a JWT without verifying the signature.
// This is intentionally minimal — full signature verification requires fetching JWKS
// from the authorization server, which will be added when testing with real providers.
func parseJWTClaims(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed JWT: expected 3 parts")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("malformed JWT payload: %w", err)
	}

	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("malformed JWT claims: %w", err)
	}

	return &claims, nil
}

// AuthMiddleware wraps an http.Handler with JWT email verification.
// If allowedEmail is empty, no authentication is enforced.
func AuthMiddleware(allowedEmail string, next http.Handler) http.Handler {
	if allowedEmail == "" {
		return next
	}
	verifier := &EmailVerifier{AllowedEmail: allowedEmail}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := verifier.VerifyRequest(r); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"code":"unauthorized","message":"%s"}}`, err.Error()), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ProtectedResourceMetadataHandler returns an http.Handler that serves
// RFC 9728 OAuth 2.0 Protected Resource Metadata at /.well-known/oauth-protected-resource.
func ProtectedResourceMetadataHandler(resourceURL string) http.Handler {
	metadata := map[string]any{
		"resource":                 resourceURL,
		"bearer_methods_supported": []string{"header"},
	}
	body, _ := json.Marshal(metadata)

	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}
