package oauth

import (
	"encoding/json"
	"net/http"
)

// authServerMetadata represents RFC 8414 Authorization Server Metadata.
// We use a local struct because the SDK's oauthex.AuthServerMeta requires
// a client-only build tag (mcp_go_client_oauth).
type authServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported,omitempty"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported,omitempty"`
}

// HandleAuthServerMetadata serves the OAuth 2.0 Authorization Server Metadata (RFC 8414)
// at /.well-known/oauth-authorization-server.
func (s *OAuthServer) HandleAuthServerMetadata(w http.ResponseWriter, _ *http.Request) {
	meta := authServerMetadata{
		Issuer:                            s.issuerURL,
		AuthorizationEndpoint:             s.issuerURL + "/oauth/authorize",
		TokenEndpoint:                     s.issuerURL + "/oauth/token",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(meta)
}
