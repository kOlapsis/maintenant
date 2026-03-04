package oauth

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// HandleAuthorize handles the authorization endpoint.
// GET /oauth/authorize — validates client credentials, PKCE, auto-approves, redirects with code.
func (s *OAuthServer) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	responseType := q.Get("response_type")
	clientID := q.Get("client_id")
	clientSecret := q.Get("client_secret")
	redirectURI := q.Get("redirect_uri")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	state := q.Get("state")
	scope := q.Get("scope")

	// Validate redirect_uri before any redirect to prevent open redirect.
	if redirectURI == "" {
		s.logger.Warn("authorization request missing redirect_uri", "client_id", clientID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_request",
			"error_description": "invalid redirect_uri",
		})
		return
	}

	if _, err := url.ParseRequestURI(redirectURI); err != nil {
		s.logger.Warn("authorization request with invalid redirect_uri", "redirect_uri", redirectURI, "client_id", clientID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_request",
			"error_description": "invalid redirect_uri",
		})
		return
	}

	if responseType != "code" {
		oauthRedirectError(w, r, redirectURI, state, "unsupported_response_type", "only 'code' is supported")
		return
	}

	// Validate client credentials
	if clientID != s.clientID || !s.VerifyClientSecret(clientSecret) {
		s.logger.Warn("authorization request with invalid client credentials", "client_id", clientID)
		oauthRedirectError(w, r, redirectURI, state, "unauthorized_client", "invalid client credentials")
		return
	}

	// PKCE is mandatory per OAuth 2.1
	if codeChallenge == "" {
		oauthRedirectError(w, r, redirectURI, state, "invalid_request", "missing code_challenge")
		return
	}
	if codeChallengeMethod != "S256" {
		oauthRedirectError(w, r, redirectURI, state, "invalid_request", "code_challenge_method must be S256")
		return
	}

	// Auto-approve: generate authorization code and redirect
	code, codeHash := GenerateToken()
	now := time.Now()

	err := s.store.StoreCode(r.Context(), &MCPAuthCode{
		CodeHash:            codeHash,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Scope:               scope,
		ExpiresAt:           now.Add(10 * time.Minute),
		CreatedAt:           now,
	})
	if err != nil {
		s.logger.Error("failed to store authorization code", "error", err)
		oauthRedirectError(w, r, redirectURI, state, "server_error", "internal error")
		return
	}

	s.logger.Info("authorization code issued", "client_id", clientID)

	redirectURL := buildRedirectURL(redirectURI, map[string]string{
		"code":  code,
		"state": state,
	})
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func oauthRedirectError(w http.ResponseWriter, r *http.Request, redirectURI, state, errCode, desc string) {
	u := buildRedirectURL(redirectURI, map[string]string{
		"error":             errCode,
		"error_description": desc,
		"state":             state,
	})
	http.Redirect(w, r, u, http.StatusFound)
}

func buildRedirectURL(baseURI string, params map[string]string) string {
	u, err := url.Parse(baseURI)
	if err != nil {
		return baseURI
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
