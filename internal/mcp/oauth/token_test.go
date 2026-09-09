package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeRefreshStore is a configurable MCPOAuthStore used to drive the refresh
// grant through its store-error and replay branches without a database.
type fakeRefreshStore struct {
	consumeToken *MCPOAuthToken
	consumeErr   error

	revokedFamilies []string
	revokeFamilyErr error

	storedTokens []*MCPOAuthToken
}

func (f *fakeRefreshStore) StoreCode(context.Context, *MCPAuthCode) error { return nil }
func (f *fakeRefreshStore) ConsumeCode(context.Context, string) (*MCPAuthCode, error) {
	return nil, nil
}
func (f *fakeRefreshStore) StoreToken(_ context.Context, token *MCPOAuthToken) error {
	f.storedTokens = append(f.storedTokens, token)
	return nil
}
func (f *fakeRefreshStore) GetToken(context.Context, string) (*MCPOAuthToken, error) {
	return nil, nil
}
func (f *fakeRefreshStore) ConsumeRefreshToken(context.Context, string) (*MCPOAuthToken, error) {
	return f.consumeToken, f.consumeErr
}
func (f *fakeRefreshStore) RevokeToken(context.Context, string) error { return nil }
func (f *fakeRefreshStore) RevokeFamily(_ context.Context, familyID string) error {
	f.revokedFamilies = append(f.revokedFamilies, familyID)
	return f.revokeFamilyErr
}
func (f *fakeRefreshStore) DeleteExpired(context.Context) (int64, error) { return 0, nil }

func newTestServerWithStore(store MCPOAuthStore) *OAuthServer {
	return NewOAuthServer(Config{
		ClientID:            "maintenant-mcp",
		ClientSecret:        "secret",
		IssuerURL:           "https://now.kolapsis.com",
		AllowedRedirectURIs: "https://claude.ai/api/mcp/auth_callback",
	}, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func doRefreshRequest(t *testing.T, s *OAuthServer, refreshToken string) (*httptest.ResponseRecorder, map[string]string) {
	t.Helper()
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {"maintenant-mcp"},
		"client_secret": {"secret"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.HandleToken(rec, req)

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return rec, body
}

func TestHandleRefreshToken_ReplayRevokesFamilyAndRejects(t *testing.T) {
	store := &fakeRefreshStore{
		consumeErr:   ErrTokenRevoked,
		consumeToken: &MCPOAuthToken{FamilyID: "family-1"},
	}
	s := newTestServerWithStore(store)

	rec, body := doRefreshRequest(t, s, "stolen-refresh-token")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != "invalid_grant" {
		t.Errorf("error = %q, want invalid_grant", body["error"])
	}
	if len(store.revokedFamilies) != 1 || store.revokedFamilies[0] != "family-1" {
		t.Errorf("revoked families = %v, want [family-1]", store.revokedFamilies)
	}
	if len(store.storedTokens) != 0 {
		t.Errorf("stored tokens = %d, want 0: replay must not issue a new pair", len(store.storedTokens))
	}
}

func TestHandleRefreshToken_UnknownTokenRejectedWithoutRevoke(t *testing.T) {
	store := &fakeRefreshStore{consumeErr: ErrTokenNotFound}
	s := newTestServerWithStore(store)

	rec, body := doRefreshRequest(t, s, "never-issued")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != "invalid_grant" {
		t.Errorf("error = %q, want invalid_grant", body["error"])
	}
	if len(store.revokedFamilies) != 0 {
		t.Errorf("revoked families = %v, want none", store.revokedFamilies)
	}
	if len(store.storedTokens) != 0 {
		t.Errorf("stored tokens = %d, want 0", len(store.storedTokens))
	}
}

func TestHandleRefreshToken_StoreErrorDuringConsumeIssuesNoPair(t *testing.T) {
	store := &fakeRefreshStore{consumeErr: errors.New("database exploded")}
	s := newTestServerWithStore(store)

	rec, body := doRefreshRequest(t, s, "any-token")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if body["error"] != "server_error" {
		t.Errorf("error = %q, want server_error", body["error"])
	}
	if len(store.revokedFamilies) != 0 {
		t.Errorf("revoked families = %v, want none", store.revokedFamilies)
	}
	if len(store.storedTokens) != 0 {
		t.Errorf("stored tokens = %d, want 0: a failed consume must not issue a pair", len(store.storedTokens))
	}
}
