package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/mcp/oauth"
)

// MCPOAuthStoreImpl implements oauth.MCPOAuthStore using SQLite.
type MCPOAuthStoreImpl struct {
	db     *Reader
	writer *Writer
}

// NewMCPOAuthStore creates a new SQLite-backed MCP OAuth store.
func NewMCPOAuthStore(d *DB) *MCPOAuthStoreImpl {
	return &MCPOAuthStoreImpl{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

func (s *MCPOAuthStoreImpl) StoreCode(ctx context.Context, code *oauth.MCPAuthCode) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO mcp_oauth_codes (code_hash, client_id, redirect_uri, code_challenge, code_challenge_method, scope, expires_at, used, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		code.CodeHash, code.ClientID, code.RedirectURI,
		code.CodeChallenge, code.CodeChallengeMethod,
		code.Scope, code.ExpiresAt.Unix(), code.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("store mcp oauth code: %w", err)
	}
	return nil
}

// ConsumeCode atomically marks an authorization code used, so that only one
// caller among concurrent exchanges of the same code can ever succeed.
func (s *MCPOAuthStoreImpl) ConsumeCode(ctx context.Context, codeHash string) (*oauth.MCPAuthCode, error) {
	now := time.Now().Unix()
	result, err := s.writer.Exec(ctx,
		`UPDATE mcp_oauth_codes SET used = 1 WHERE code_hash = ? AND used = 0 AND expires_at > ?`,
		codeHash, now)
	if err != nil {
		return nil, fmt.Errorf("consume mcp oauth code: %w", err)
	}
	if result.RowsAffected != 1 {
		return nil, s.diagnoseCodeFailure(ctx, codeHash, now)
	}
	return s.readCode(ctx, codeHash)
}

func (s *MCPOAuthStoreImpl) diagnoseCodeFailure(ctx context.Context, codeHash string, now int64) error {
	code, err := s.readCode(ctx, codeHash)
	if err != nil {
		return err
	}
	if code.Used {
		return oauth.ErrCodeUsed
	}
	if code.ExpiresAt.Unix() <= now {
		return oauth.ErrCodeExpired
	}
	// Row exists, unused and unexpired, yet the conditional UPDATE affected no
	// row: a concurrent caller won the race between it and this diagnosis.
	return oauth.ErrCodeUsed
}

func (s *MCPOAuthStoreImpl) readCode(ctx context.Context, codeHash string) (*oauth.MCPAuthCode, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT code_hash, client_id, redirect_uri, code_challenge, code_challenge_method, scope, expires_at, used, created_at
		 FROM mcp_oauth_codes WHERE code_hash = ?`, codeHash)

	var code oauth.MCPAuthCode
	var expiresAt, createdAt int64
	var used int
	err := row.Scan(&code.CodeHash, &code.ClientID, &code.RedirectURI,
		&code.CodeChallenge, &code.CodeChallengeMethod,
		&code.Scope, &expiresAt, &used, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, oauth.ErrCodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query mcp oauth code: %w", err)
	}

	code.ExpiresAt = time.Unix(expiresAt, 0)
	code.CreatedAt = time.Unix(createdAt, 0)
	code.Used = used != 0
	return &code, nil
}

func (s *MCPOAuthStoreImpl) StoreToken(ctx context.Context, token *oauth.MCPOAuthToken) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO mcp_oauth_tokens (token_hash, token_type, client_id, scope, expires_at, revoked, family_id, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		token.TokenHash, token.TokenType, token.ClientID,
		token.Scope, token.ExpiresAt.Unix(),
		token.FamilyID, token.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("store mcp oauth token: %w", err)
	}
	return nil
}

func (s *MCPOAuthStoreImpl) GetToken(ctx context.Context, tokenHash string) (*oauth.MCPOAuthToken, error) {
	return s.readToken(ctx, tokenHash)
}

// ConsumeRefreshToken atomically revokes a refresh token, so that only one
// caller among concurrent refreshes of the same token can ever rotate it.
func (s *MCPOAuthStoreImpl) ConsumeRefreshToken(ctx context.Context, tokenHash string) (*oauth.MCPOAuthToken, error) {
	result, err := s.writer.Exec(ctx,
		`UPDATE mcp_oauth_tokens SET revoked = 1 WHERE token_hash = ? AND token_type = 'refresh' AND revoked = 0`,
		tokenHash)
	if err != nil {
		return nil, fmt.Errorf("consume mcp oauth refresh token: %w", err)
	}
	if result.RowsAffected != 1 {
		return s.diagnoseTokenFailure(ctx, tokenHash)
	}
	return s.readToken(ctx, tokenHash)
}

// diagnoseTokenFailure explains why the conditional UPDATE affected no row.
// For ErrTokenRevoked it also returns the current row, since the caller needs
// its FamilyID to revoke the whole family on a replay.
func (s *MCPOAuthStoreImpl) diagnoseTokenFailure(ctx context.Context, tokenHash string) (*oauth.MCPOAuthToken, error) {
	token, err := s.readToken(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if token.TokenType != "refresh" {
		return nil, oauth.ErrTokenNotFound
	}
	// The row exists as a refresh token yet wasn't revoked by us: it is
	// either already revoked, or a concurrent caller just won the race
	// between it and this diagnosis. Either way, it is spent.
	return token, oauth.ErrTokenRevoked
}

func (s *MCPOAuthStoreImpl) readToken(ctx context.Context, tokenHash string) (*oauth.MCPOAuthToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT token_hash, token_type, client_id, scope, expires_at, revoked, family_id, created_at
		 FROM mcp_oauth_tokens WHERE token_hash = ?`, tokenHash)

	var token oauth.MCPOAuthToken
	var expiresAt, createdAt int64
	var revoked int
	err := row.Scan(&token.TokenHash, &token.TokenType, &token.ClientID,
		&token.Scope, &expiresAt, &revoked, &token.FamilyID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, oauth.ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query mcp oauth token: %w", err)
	}

	token.ExpiresAt = time.Unix(expiresAt, 0)
	token.CreatedAt = time.Unix(createdAt, 0)
	token.Revoked = revoked != 0

	return &token, nil
}

func (s *MCPOAuthStoreImpl) RevokeToken(ctx context.Context, tokenHash string) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE mcp_oauth_tokens SET revoked = 1 WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("revoke mcp oauth token: %w", err)
	}
	return nil
}

func (s *MCPOAuthStoreImpl) RevokeFamily(ctx context.Context, familyID string) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE mcp_oauth_tokens SET revoked = 1 WHERE family_id = ?`, familyID)
	if err != nil {
		return fmt.Errorf("revoke mcp oauth token family: %w", err)
	}
	return nil
}

func (s *MCPOAuthStoreImpl) DeleteExpired(ctx context.Context) (int64, error) {
	now := time.Now().Unix()

	res1, err := s.writer.Exec(ctx,
		`DELETE FROM mcp_oauth_codes WHERE expires_at < ?`, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired mcp oauth codes: %w", err)
	}

	res2, err := s.writer.Exec(ctx,
		`DELETE FROM mcp_oauth_tokens WHERE expires_at < ?`, now)
	if err != nil {
		return res1.RowsAffected, fmt.Errorf("delete expired mcp oauth tokens: %w", err)
	}

	return res1.RowsAffected + res2.RowsAffected, nil
}
