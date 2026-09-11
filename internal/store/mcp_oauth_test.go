package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/mcp/oauth"
	"github.com/stretchr/testify/require"
)

func TestConsumeCode_ConcurrentExchangesYieldExactlyOneWinner(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	now := time.Now()
	code := &oauth.MCPAuthCode{
		CodeHash:            "concurrent-code-hash",
		ClientID:            "client",
		RedirectURI:         "https://example.com/cb",
		CodeChallenge:       "challenge",
		CodeChallengeMethod: "S256",
		Scope:               "mcp",
		ExpiresAt:           now.Add(10 * time.Minute),
		CreatedAt:           now,
	}
	require.NoError(t, s.StoreCode(ctx, code))

	const n = 16
	var ready sync.WaitGroup
	ready.Add(n)
	release := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-release
			_, err := s.ConsumeCode(ctx, code.CodeHash)
			errs[i] = err
		}(i)
	}
	ready.Wait()
	close(release)
	wg.Wait()

	successes, failures := 0, 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		failures++
		require.ErrorIs(t, err, oauth.ErrCodeUsed)
	}
	require.Equal(t, 1, successes, "exactly one caller must win the race")
	require.Equal(t, n-1, failures)
}

func TestConsumeCode_ExpiredIsRefused(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	now := time.Now()
	code := &oauth.MCPAuthCode{
		CodeHash:            "expired-code-hash",
		ClientID:            "client",
		RedirectURI:         "https://example.com/cb",
		CodeChallenge:       "challenge",
		CodeChallengeMethod: "S256",
		ExpiresAt:           now.Add(-time.Minute),
		CreatedAt:           now.Add(-11 * time.Minute),
	}
	require.NoError(t, s.StoreCode(ctx, code))

	_, err := s.ConsumeCode(ctx, code.CodeHash)
	require.ErrorIs(t, err, oauth.ErrCodeExpired)
}

func TestConsumeCode_UnknownIsRefused(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	_, err := s.ConsumeCode(ctx, "never-issued")
	require.ErrorIs(t, err, oauth.ErrCodeNotFound)
}

func TestConsumeCode_UsedIsRefused(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	now := time.Now()
	code := &oauth.MCPAuthCode{
		CodeHash:            "single-use-code-hash",
		ClientID:            "client",
		RedirectURI:         "https://example.com/cb",
		CodeChallenge:       "challenge",
		CodeChallengeMethod: "S256",
		ExpiresAt:           now.Add(10 * time.Minute),
		CreatedAt:           now,
	}
	require.NoError(t, s.StoreCode(ctx, code))

	_, err := s.ConsumeCode(ctx, code.CodeHash)
	require.NoError(t, err)

	_, err = s.ConsumeCode(ctx, code.CodeHash)
	require.ErrorIs(t, err, oauth.ErrCodeUsed)
}

func TestConsumeCode_ReturnsTheStoredFields(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	now := time.Now()
	code := &oauth.MCPAuthCode{
		CodeHash:            "roundtrip-code-hash",
		ClientID:            "maintenant-mcp",
		RedirectURI:         "https://claude.ai/api/mcp/auth_callback",
		CodeChallenge:       "challenge-value",
		CodeChallengeMethod: "S256",
		Scope:               "mcp:read mcp:write",
		ExpiresAt:           now.Add(10 * time.Minute),
		CreatedAt:           now,
	}
	require.NoError(t, s.StoreCode(ctx, code))

	got, err := s.ConsumeCode(ctx, code.CodeHash)
	require.NoError(t, err)
	require.Equal(t, code.CodeHash, got.CodeHash)
	require.Equal(t, code.ClientID, got.ClientID)
	require.Equal(t, code.RedirectURI, got.RedirectURI)
	require.Equal(t, code.CodeChallenge, got.CodeChallenge)
	require.Equal(t, code.CodeChallengeMethod, got.CodeChallengeMethod)
	require.Equal(t, code.Scope, got.Scope)
	require.True(t, got.Used)
}

func TestConsumeRefreshToken_ConcurrentRefreshesYieldExactlyOneWinner(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	now := time.Now()
	token := &oauth.MCPOAuthToken{
		TokenHash: "concurrent-refresh-hash",
		TokenType: "refresh",
		ClientID:  "client",
		Scope:     "mcp",
		ExpiresAt: now.Add(time.Hour),
		FamilyID:  "family-concurrent",
		CreatedAt: now,
	}
	require.NoError(t, s.StoreToken(ctx, token))

	const n = 16
	var ready sync.WaitGroup
	ready.Add(n)
	release := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-release
			_, err := s.ConsumeRefreshToken(ctx, token.TokenHash)
			errs[i] = err
		}(i)
	}
	ready.Wait()
	close(release)
	wg.Wait()

	successes, failures := 0, 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		failures++
		require.ErrorIs(t, err, oauth.ErrTokenRevoked)
	}
	require.Equal(t, 1, successes, "exactly one caller must win the race")
	require.Equal(t, n-1, failures)
}

func TestConsumeRefreshToken_UnknownIsRefused(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	_, err := s.ConsumeRefreshToken(ctx, "never-issued")
	require.ErrorIs(t, err, oauth.ErrTokenNotFound)
}

func TestConsumeRefreshToken_WrongTypeIsRefused(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	now := time.Now()
	access := &oauth.MCPOAuthToken{
		TokenHash: "access-not-refresh-hash",
		TokenType: "access",
		ClientID:  "client",
		ExpiresAt: now.Add(time.Hour),
		FamilyID:  "family-access",
		CreatedAt: now,
	}
	require.NoError(t, s.StoreToken(ctx, access))

	_, err := s.ConsumeRefreshToken(ctx, access.TokenHash)
	require.ErrorIs(t, err, oauth.ErrTokenNotFound)
}

func TestConsumeRefreshToken_RevokedIsRefusedAndCarriesFamilyID(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	now := time.Now()
	token := &oauth.MCPOAuthToken{
		TokenHash: "replayed-refresh-hash",
		TokenType: "refresh",
		ClientID:  "client",
		ExpiresAt: now.Add(time.Hour),
		FamilyID:  "family-replay",
		CreatedAt: now,
	}
	require.NoError(t, s.StoreToken(ctx, token))

	_, err := s.ConsumeRefreshToken(ctx, token.TokenHash)
	require.NoError(t, err)

	got, err := s.ConsumeRefreshToken(ctx, token.TokenHash)
	require.ErrorIs(t, err, oauth.ErrTokenRevoked)
	require.NotNil(t, got, "the caller needs the row back to revoke the token family")
	require.Equal(t, token.FamilyID, got.FamilyID)
}

func TestConsumeRefreshToken_ReturnsTheStoredFields(t *testing.T) {
	db := openTestDB(t)
	s := NewMCPOAuthStore(db)
	ctx := context.Background()

	now := time.Now()
	token := &oauth.MCPOAuthToken{
		TokenHash: "roundtrip-refresh-hash",
		TokenType: "refresh",
		ClientID:  "maintenant-mcp",
		Scope:     "mcp:read mcp:write",
		ExpiresAt: now.Add(time.Hour),
		FamilyID:  "family-roundtrip",
		CreatedAt: now,
	}
	require.NoError(t, s.StoreToken(ctx, token))

	got, err := s.ConsumeRefreshToken(ctx, token.TokenHash)
	require.NoError(t, err)
	require.Equal(t, token.TokenHash, got.TokenHash)
	require.Equal(t, token.ClientID, got.ClientID)
	require.Equal(t, token.Scope, got.Scope)
	require.Equal(t, token.FamilyID, got.FamilyID)
	require.True(t, got.Revoked)
}
