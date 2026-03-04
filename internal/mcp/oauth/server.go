package oauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"strings"
	"time"
)

// Config holds the OAuth server configuration.
type Config struct {
	ClientID     string
	ClientSecret string
	IssuerURL    string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
}

// OAuthServer implements OAuth 2.1 with PKCE for MCP authentication.
type OAuthServer struct {
	clientID         string
	clientSecretHash [sha256.Size]byte
	issuerURL        string
	accessTTL        time.Duration
	refreshTTL       time.Duration
	store            MCPOAuthStore
	logger           *slog.Logger
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

	return &OAuthServer{
		clientID:         cfg.ClientID,
		clientSecretHash: sha256.Sum256([]byte(cfg.ClientSecret)),
		issuerURL:        strings.TrimRight(cfg.IssuerURL, "/"),
		accessTTL:        accessTTL,
		refreshTTL:       refreshTTL,
		store:            store,
		logger:           logger,
	}
}

// VerifyClientSecret checks the provided secret against the stored hash
// using constant-time comparison to prevent timing attacks.
func (s *OAuthServer) VerifyClientSecret(provided string) bool {
	h := sha256.Sum256([]byte(provided))
	return subtle.ConstantTimeCompare(h[:], s.clientSecretHash[:]) == 1
}
