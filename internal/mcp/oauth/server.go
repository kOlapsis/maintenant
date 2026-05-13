package oauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

// defaultAllowedRedirectOrigins is intentionally empty.
// Loopback (localhost / 127.0.0.1 / ::1) is always accepted for local clients.
// Remote origins must be added explicitly via MAINTENANT_MCP_ALLOWED_REDIRECT_URIS.
var defaultAllowedRedirectOrigins []string

// Config holds the OAuth server configuration.
type Config struct {
	ClientID     string
	ClientSecret string
	IssuerURL    string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	// AllowedRedirectURIs is a comma-separated list of additional allowed redirect
	// URI origins (scheme://host) appended to the built-in defaults.
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
	allowedRedirectOrigins []string
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

	allowed := make([]string, len(defaultAllowedRedirectOrigins))
	copy(allowed, defaultAllowedRedirectOrigins)
	for _, raw := range strings.Split(cfg.AllowedRedirectURIs, ",") {
		if origin := strings.TrimSpace(raw); origin != "" {
			allowed = append(allowed, origin)
		}
	}

	return &OAuthServer{
		clientID:               cfg.ClientID,
		clientSecretHash:       sha256.Sum256([]byte(cfg.ClientSecret)),
		issuerURL:              strings.TrimRight(cfg.IssuerURL, "/"),
		accessTTL:              accessTTL,
		refreshTTL:             refreshTTL,
		allowedRedirectOrigins: allowed,
		store:                  store,
		logger:                 logger,
	}
}

// isRedirectURIAllowed returns true when uri is a loopback address or its
// origin (scheme://host) is in the server's allowed list.
func (s *OAuthServer) isRedirectURIAllowed(rawURI string) bool {
	u, err := url.Parse(rawURI)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return u.Scheme == "http" || u.Scheme == "https"
	}
	origin := u.Scheme + "://" + u.Host
	for _, allowed := range s.allowedRedirectOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

// VerifyClientSecret checks the provided secret against the stored hash
// using constant-time comparison to prevent timing attacks.
func (s *OAuthServer) VerifyClientSecret(provided string) bool {
	h := sha256.Sum256([]byte(provided))
	return subtle.ConstantTimeCompare(h[:], s.clientSecretHash[:]) == 1
}
