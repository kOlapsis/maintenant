package oauth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// HandleToken handles the token endpoint.
// POST /oauth/token — supports authorization_code and refresh_token grants.
func (s *OAuthServer) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, http.StatusBadRequest, "invalid_request", "malformed form data")
		return
	}

	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		s.handleAuthorizationCode(w, r)
	case "refresh_token":
		s.handleRefreshToken(w, r)
	default:
		tokenError(w, http.StatusBadRequest, "unsupported_grant_type", "")
	}
}

func (s *OAuthServer) handleAuthorizationCode(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")

	// Validate client credentials
	if clientID != s.clientID || !s.VerifyClientSecret(clientSecret) {
		s.logger.Warn("token request with invalid client credentials", "client_id", clientID)
		tokenError(w, http.StatusUnauthorized, "invalid_client", "invalid client credentials")
		return
	}

	// Consume authorization code (marks as used atomically)
	codeHash := HashToken(code)
	authCode, err := s.store.ConsumeCode(r.Context(), codeHash)
	if err != nil {
		s.logger.Warn("token request with invalid code", "error", err)
		tokenError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}

	// Verify client_id matches the code
	if authCode.ClientID != clientID {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}

	// Verify redirect_uri matches
	if redirectURI != authCode.RedirectURI {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}

	// PKCE verification — always enforced
	if codeVerifier == "" {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "code_verifier is required")
		return
	}
	if !VerifyPKCE(codeVerifier, authCode.CodeChallenge) {
		s.logger.Warn("PKCE verification failed", "client_id", clientID)
		tokenError(w, http.StatusBadRequest, "invalid_grant", "code_verifier does not match code_challenge")
		return
	}

	familyID := uuid.New().String()
	s.issueTokenPair(w, r, clientID, authCode.Scope, familyID)
}

func (s *OAuthServer) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	refreshTokenRaw := r.FormValue("refresh_token")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	// Validate client credentials
	if clientID != s.clientID || !s.VerifyClientSecret(clientSecret) {
		tokenError(w, http.StatusUnauthorized, "invalid_client", "invalid client credentials")
		return
	}

	// Look up the refresh token
	tokenHash := HashToken(refreshTokenRaw)
	storedToken, err := s.store.GetToken(r.Context(), tokenHash)
	if err != nil {
		s.logger.Warn("refresh with unknown token", "error", err)
		tokenError(w, http.StatusBadRequest, "invalid_grant", "invalid refresh token")
		return
	}

	// Check token type
	if storedToken.TokenType != "refresh" {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "not a refresh token")
		return
	}

	// Replay detection: if already revoked, revoke the entire family
	if storedToken.Revoked {
		s.logger.Warn("refresh token replay detected, revoking family",
			"family_id", storedToken.FamilyID, "client_id", clientID)
		if err := s.store.RevokeFamily(r.Context(), storedToken.FamilyID); err != nil {
			s.logger.Error("failed to revoke token family", "error", err)
		}
		tokenError(w, http.StatusBadRequest, "invalid_grant", "refresh token already used")
		return
	}

	// Check expiration
	if time.Now().After(storedToken.ExpiresAt) {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "refresh token expired")
		return
	}

	// Revoke old refresh token (rotation)
	if err := s.store.RevokeToken(r.Context(), tokenHash); err != nil {
		s.logger.Error("failed to revoke old refresh token", "error", err)
	}

	s.logger.Info("refresh token rotated", "client_id", clientID, "family_id", storedToken.FamilyID)
	s.issueTokenPair(w, r, clientID, storedToken.Scope, storedToken.FamilyID)
}

func (s *OAuthServer) issueTokenPair(w http.ResponseWriter, r *http.Request, clientID, scope, familyID string) {
	now := time.Now()

	accessToken, accessHash := GenerateToken()
	refreshToken, refreshHash := GenerateToken()

	// Store access token
	if err := s.store.StoreToken(r.Context(), &MCPOAuthToken{
		TokenHash: accessHash,
		TokenType: "access",
		ClientID:  clientID,
		Scope:     scope,
		ExpiresAt: now.Add(s.accessTTL),
		FamilyID:  familyID,
		CreatedAt: now,
	}); err != nil {
		s.logger.Error("failed to store access token", "error", err)
		tokenError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}

	// Store refresh token
	if err := s.store.StoreToken(r.Context(), &MCPOAuthToken{
		TokenHash: refreshHash,
		TokenType: "refresh",
		ClientID:  clientID,
		Scope:     scope,
		ExpiresAt: now.Add(s.refreshTTL),
		FamilyID:  familyID,
		CreatedAt: now,
	}); err != nil {
		s.logger.Error("failed to store refresh token", "error", err)
		tokenError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}

	s.logger.Info("tokens issued", "client_id", clientID, "family_id", familyID)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(s.accessTTL.Seconds()),
		"refresh_token": refreshToken,
	})
}

func tokenError(w http.ResponseWriter, status int, errCode, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]string{"error": errCode}
	if desc != "" {
		resp["error_description"] = desc
	}
	_ = json.NewEncoder(w).Encode(resp)
}
