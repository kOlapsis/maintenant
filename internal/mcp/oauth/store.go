package oauth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrCodeNotFound = errors.New("authorization code not found")
	ErrCodeUsed     = errors.New("authorization code already used")
	ErrCodeExpired  = errors.New("authorization code expired")

	ErrTokenNotFound = errors.New("token not found")
	ErrTokenRevoked  = errors.New("token revoked")
)

// MCPAuthCode represents an OAuth authorization code stored in the database.
type MCPAuthCode struct {
	CodeHash            string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	ExpiresAt           time.Time
	Used                bool
	CreatedAt           time.Time
}

// MCPOAuthToken represents an OAuth token stored in the database.
type MCPOAuthToken struct {
	TokenHash string
	TokenType string // "access" or "refresh"
	ClientID  string
	Scope     string
	ExpiresAt time.Time
	Revoked   bool
	FamilyID  string
	CreatedAt time.Time
}

// MCPOAuthStore is the persistence interface for MCP OAuth codes and tokens.
type MCPOAuthStore interface {
	// Codes
	StoreCode(ctx context.Context, code *MCPAuthCode) error
	ConsumeCode(ctx context.Context, codeHash string) (*MCPAuthCode, error)

	// Tokens
	StoreToken(ctx context.Context, token *MCPOAuthToken) error
	GetToken(ctx context.Context, tokenHash string) (*MCPOAuthToken, error)
	ConsumeRefreshToken(ctx context.Context, tokenHash string) (*MCPOAuthToken, error)
	RevokeToken(ctx context.Context, tokenHash string) error
	RevokeFamily(ctx context.Context, familyID string) error

	// Cleanup
	DeleteExpired(ctx context.Context) (int64, error)
}
