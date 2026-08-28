package oauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"
)

// defaultAllowedRedirectURIs is intentionally empty.
// Loopback (localhost / 127.0.0.1 / ::1) is always accepted for local clients.
// Remote redirect URIs must be added explicitly via MAINTENANT_MCP_ALLOWED_REDIRECT_URIS.
var defaultAllowedRedirectURIs []string

// Config holds the OAuth server configuration.
type Config struct {
	ClientID     string
	ClientSecret string
	IssuerURL    string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	// AllowedRedirectURIs is a comma-separated list of full redirect URIs that
	// are matched exactly (RFC 6749 §3.1.2.3). Loopback is always allowed.
	// Set via MAINTENANT_MCP_ALLOWED_REDIRECT_URIS.
	AllowedRedirectURIs string
}

// OAuthServer implements OAuth 2.1 with PKCE for MCP authentication.
type OAuthServer struct {
	clientID               string
	clientSecretHash       [sha256.Size]byte
	issuerURL              string
	accessTTL              time.Duration
	refreshTTL             time.Duration
	allowedRedirectURIs    []string
	store                  MCPOAuthStore
	logger                 *slog.Logger
}

// NewOAuthServer creates an OAuth 2.1 server from config.
func NewOAuthServer(cfg Config, store MCPOAuthStore, logger *slog.Logger) *OAuthServer {
	accessTTL := cfg.AccessTTL
	if accessTTL == 0 {
		accessTTL = time.Hour
	}
	refreshTTL := cfg.RefreshTTL
	if refreshTTL == 0 {
		refreshTTL = 30 * 24 * time.Hour
	}

	allowed := make([]string, len(defaultAllowedRedirectURIs))
	copy(allowed, defaultAllowedRedirectURIs)
	for raw := range strings.SplitSeq(cfg.AllowedRedirectURIs, ",") {
		if uri := strings.TrimSpace(raw); uri != "" {
			allowed = append(allowed, uri)
		}
	}

	return &OAuthServer{
		clientID:               cfg.ClientID,
		clientSecretHash:       sha256.Sum256([]byte(cfg.ClientSecret)),
		issuerURL:              strings.TrimRight(cfg.IssuerURL, "/"),
		accessTTL:              accessTTL,
		refreshTTL:             refreshTTL,
		allowedRedirectURIs:    allowed,
		store:                  store,
		logger:                 logger,
	}
}

// isRedirectURIAllowed returns true when uri is a loopback address or matches
// an allowed redirect URI exactly (simple string comparison, RFC 6749 §3.1.2.3).
func (s *OAuthServer) isRedirectURIAllowed(rawURI string) bool {
	u, err := url.Parse(rawURI)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return u.Scheme == "http" || u.Scheme == "https"
	}
	return slices.Contains(s.allowedRedirectURIs, rawURI)
}

// VerifyClientSecret checks the provided secret against the stored hash
// using constant-time comparison to prevent timing attacks.
func (s *OAuthServer) VerifyClientSecret(provided string) bool {
	h := sha256.Sum256([]byte(provided))
	return subtle.ConstantTimeCompare(h[:], s.clientSecretHash[:]) == 1
}
