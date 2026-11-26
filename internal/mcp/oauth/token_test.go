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

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

type failingGetStore struct {
	fakeStore
	err error
}

func (s *failingGetStore) GetToken(context.Context, string) (*MCPOAuthToken, error) {
	return nil, s.err
}

func TestNewTokenVerifier_AStoreFailureIsNotAnInvalidToken(t *testing.T) {
	_, err := NewTokenVerifier(&failingGetStore{err: errors.New("database exploded")})(context.Background(), "raw", nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, auth.ErrInvalidToken)
	assert.NotContains(t, err.Error(), "exploded")

	_, err = NewTokenVerifier(&failingGetStore{err: ErrTokenNotFound})(context.Background(), "raw", nil)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

type failingCodeStore struct {
	fakeRefreshStore
	err error
}

func (s *failingCodeStore) ConsumeCode(context.Context, string) (*MCPAuthCode, error) {
	return nil, s.err
}

func TestHandleToken_CodeExchangeTellsAStoreFailureFromABadCode(t *testing.T) {
	for _, tc := range []struct {
		err        error
		wantStatus int
		wantError  string
	}{
		{errors.New("database exploded"), http.StatusInternalServerError, "server_error"},
		{ErrCodeNotFound, http.StatusBadRequest, "invalid_grant"},
		{ErrCodeUsed, http.StatusBadRequest, "invalid_grant"},
		{ErrCodeExpired, http.StatusBadRequest, "invalid_grant"},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			rec, body := postToken(t, newTestServerWithStore(&failingCodeStore{err: tc.err}), url.Values{
				"grant_type": {"authorization_code"}, "code": {"c"}, "code_verifier": {"v"}, "redirect_uri": {"http://localhost/cb"},
			})
			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			assert.Equal(t, tc.wantError, body["error"])
			assert.NotContains(t, rec.Body.String(), "exploded")
		})
	}
}

func TestHandleToken_RefusesGetAndQueryStringCredentials(t *testing.T) {
	form := url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {"old"},
		"client_id": {"maintenant-mcp"}, "client_secret": {"secret"},
	}

	store := refreshStore("read")
	rec := httptest.NewRecorder()
	newTestServerWithStore(store).HandleToken(rec, httptest.NewRequest(http.MethodGet, "/oauth/token?"+form.Encode(), nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, rec.Body.String())
	assert.Empty(t, store.storedTokens)

	store = refreshStore("read")
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token?"+form.Encode(), strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	newTestServerWithStore(store).HandleToken(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Empty(t, store.storedTokens, "credentials in the query string are not read")
}
