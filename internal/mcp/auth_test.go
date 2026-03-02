package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildJWT creates a minimal unsigned JWT for testing purposes.
// The signature part is a dummy value since parseJWTClaims does not verify signatures.
func buildJWT(claims jwtClaims) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	sig := base64.RawURLEncoding.EncodeToString([]byte("test-signature"))
	return fmt.Sprintf("%s.%s.%s", header, payloadEnc, sig)
}

// --- parseJWTClaims tests ---

func TestParseJWTClaims_ValidToken(t *testing.T) {
	token := buildJWT(jwtClaims{
		Sub:   "user-123",
		Email: "user@example.com",
		Exp:   time.Now().Add(time.Hour).Unix(),
		Iss:   "https://auth.example.com",
	})

	claims, err := parseJWTClaims(token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.Sub)
	assert.Equal(t, "user@example.com", claims.Email)
	assert.Equal(t, "https://auth.example.com", claims.Iss)
}

func TestParseJWTClaims_MalformedTwoParts(t *testing.T) {
	_, err := parseJWTClaims("header.payload")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed JWT: expected 3 parts")
}

func TestParseJWTClaims_MalformedOnePart(t *testing.T) {
	_, err := parseJWTClaims("just-a-string")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed JWT: expected 3 parts")
}

func TestParseJWTClaims_EmptyString(t *testing.T) {
	_, err := parseJWTClaims("")
	require.Error(t, err)
}

func TestParseJWTClaims_InvalidBase64Payload(t *testing.T) {
	_, err := parseJWTClaims("aaa.!!!invalid-base64!!!.ccc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed JWT payload")
}

func TestParseJWTClaims_InvalidJSONPayload(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	token := fmt.Sprintf("header.%s.sig", payload)
	_, err := parseJWTClaims(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed JWT claims")
}

// --- EmailVerifier tests ---

func TestEmailVerifier_MissingHeader(t *testing.T) {
	v := &EmailVerifier{AllowedEmail: "admin@example.com"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	err := v.VerifyRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing Authorization header")
}

func TestEmailVerifier_InvalidBearerPrefix(t *testing.T) {
	v := &EmailVerifier{AllowedEmail: "admin@example.com"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	err := v.VerifyRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected Bearer token")
}

func TestEmailVerifier_ExpiredToken(t *testing.T) {
	v := &EmailVerifier{AllowedEmail: "admin@example.com"}
	token := buildJWT(jwtClaims{
		Email: "admin@example.com",
		Exp:   time.Now().Add(-time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	err := v.VerifyRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestEmailVerifier_WrongEmail(t *testing.T) {
	v := &EmailVerifier{AllowedEmail: "admin@example.com"}
	token := buildJWT(jwtClaims{
		Email: "hacker@evil.com",
		Exp:   time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	err := v.VerifyRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized: email does not match")
}

func TestEmailVerifier_MatchingEmail(t *testing.T) {
	v := &EmailVerifier{AllowedEmail: "admin@example.com"}
	token := buildJWT(jwtClaims{
		Email: "admin@example.com",
		Exp:   time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	err := v.VerifyRequest(req)
	require.NoError(t, err)
}

func TestEmailVerifier_CaseInsensitiveEmail(t *testing.T) {
	v := &EmailVerifier{AllowedEmail: "Admin@Example.COM"}
	token := buildJWT(jwtClaims{
		Email: "admin@example.com",
		Exp:   time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	err := v.VerifyRequest(req)
	require.NoError(t, err)
}

func TestEmailVerifier_FallsBackToSub(t *testing.T) {
	v := &EmailVerifier{AllowedEmail: "admin@example.com"}
	token := buildJWT(jwtClaims{
		Sub: "admin@example.com",
		Exp: time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	err := v.VerifyRequest(req)
	require.NoError(t, err)
}

func TestEmailVerifier_MissingEmailAndSub(t *testing.T) {
	v := &EmailVerifier{AllowedEmail: "admin@example.com"}
	token := buildJWT(jwtClaims{
		Exp: time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	err := v.VerifyRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token missing email or sub claim")
}

func TestEmailVerifier_NoExpiration(t *testing.T) {
	// Token with Exp=0 should not be treated as expired
	v := &EmailVerifier{AllowedEmail: "admin@example.com"}
	token := buildJWT(jwtClaims{
		Email: "admin@example.com",
		Exp:   0,
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	err := v.VerifyRequest(req)
	require.NoError(t, err)
}

func TestEmailVerifier_MalformedToken(t *testing.T) {
	v := &EmailVerifier{AllowedEmail: "admin@example.com"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")

	err := v.VerifyRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid token")
}

// --- AuthMiddleware tests ---

func TestAuthMiddleware_EmptyEmail_PassesThrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware("", inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.True(t, called, "inner handler should be called when email is empty")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_ValidAuth_PassesThrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	token := buildJWT(jwtClaims{
		Email: "admin@example.com",
		Exp:   time.Now().Add(time.Hour).Unix(),
	})

	handler := AuthMiddleware("admin@example.com", inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.True(t, called, "inner handler should be called with valid auth")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_MissingAuth_Returns401(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	handler := AuthMiddleware("admin@example.com", inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.False(t, called, "inner handler should not be called without auth")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_WrongEmail_Returns401(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	token := buildJWT(jwtClaims{
		Email: "wrong@example.com",
		Exp:   time.Now().Add(time.Hour).Unix(),
	})

	handler := AuthMiddleware("admin@example.com", inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.False(t, called, "inner handler should not be called with wrong email")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_ErrorBodyIsJSON(t *testing.T) {
	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	handler := AuthMiddleware("admin@example.com", inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	assert.Contains(t, string(body), `"error"`)
	assert.Contains(t, string(body), `"unauthorized"`)
}

// --- ProtectedResourceMetadataHandler tests ---

func TestProtectedResourceMetadataHandler_ReturnsCorrectJSON(t *testing.T) {
	handler := ProtectedResourceMetadataHandler("https://maintenant.example.com")
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var metadata map[string]any
	err := json.NewDecoder(rec.Body).Decode(&metadata)
	require.NoError(t, err)
	assert.Equal(t, "https://maintenant.example.com", metadata["resource"])

	methods, ok := metadata["bearer_methods_supported"].([]any)
	require.True(t, ok)
	require.Len(t, methods, 1)
	assert.Equal(t, "header", methods[0])
}

func TestProtectedResourceMetadataHandler_StableResponse(t *testing.T) {
	handler := ProtectedResourceMetadataHandler("https://example.com")

	// Call twice to verify the pre-marshaled body is reused correctly
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)

		var metadata map[string]any
		err := json.NewDecoder(rec.Body).Decode(&metadata)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", metadata["resource"])
	}
}
