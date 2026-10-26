package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 045-mcp-hardening on the OAuth layer: scope parsing,
// the authorize endpoint, the token endpoint and the verifier.

// FR-010, US2 scenarios 1 and 2, edge case "scope répété".
func TestParseScope(t *testing.T) {
	for raw, want := range map[string]string{
		"":                "read",
		"read":            "read",
		"read write":      "read write",
		"write read":      "read write",
		"read read write": "read write",
		"write write":     "",
		"write":           "",
		"admin":           "",
		"read foo":        "",
		"READ":            "",
		"read,write":      "",
	} {
		t.Run("["+raw+"]", func(t *testing.T) {
			got, err := ParseScope(raw)
			if want == "" {
				assert.Error(t, err, "%q is not a scope this server knows", raw)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func authorizeQuery(scope string) url.Values {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {"maintenant-mcp"},
		"redirect_uri":          {"http://localhost:33418/oauth/callback"},
		"code_challenge":        {"abc"},
		"code_challenge_method": {"S256"},
		"state":                 {"xyz"},
	}
	if scope != "" {
		q.Set("scope", scope)
	}
	return q
}

func authorize(t *testing.T, s *OAuthServer, q url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	s.HandleAuthorize(rec, req)
	return rec
}

func location(t *testing.T, rec *httptest.ResponseRecorder) *url.URL {
	t.Helper()
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	return loc
}

// US2 scenario 1: no scope means read.
func TestHandleAuthorize_NoScopeMeansRead(t *testing.T) {
	store := &fakeStore{}
	s := newTestServerWithStore(store)

	for name, q := range map[string]url.Values{
		"absent": authorizeQuery(""),
		"empty":  func() url.Values { q := authorizeQuery(""); q.Set("scope", ""); return q }(),
	} {
		t.Run(name, func(t *testing.T) {
			store.stored = nil
			loc := location(t, authorize(t, s, q))
			assert.NotEmpty(t, loc.Query().Get("code"))
			require.NotNil(t, store.stored, "a code is issued")
			assert.Equal(t, "read", store.stored.Scope)
		})
	}
}

// US2 scenario 2, edge case "scope répété": read and write in any order and count become "read write".
func TestHandleAuthorize_NormalisesReadWrite(t *testing.T) {
	store := &fakeStore{}
	s := newTestServerWithStore(store)

	for _, raw := range []string{"read write", "write read", "read read write"} {
		t.Run(raw, func(t *testing.T) {
			store.stored = nil
			loc := location(t, authorize(t, s, authorizeQuery(raw)))
			assert.NotEmpty(t, loc.Query().Get("code"))
			require.NotNil(t, store.stored)
			assert.Equal(t, "read write", store.stored.Scope)
		})
	}

	t.Run("read alone stays read", func(t *testing.T) {
		store.stored = nil
		location(t, authorize(t, s, authorizeQuery("read")))
		require.NotNil(t, store.stored)
		assert.Equal(t, "read", store.stored.Scope)
	})
}

// US2 scenario 3, FR-010: an unknown scope is refused with invalid_scope and no code is stored.
func TestHandleAuthorize_RefusesAnUnknownScope(t *testing.T) {
	store := &fakeStore{}
	s := newTestServerWithStore(store)

	for _, raw := range []string{"admin", "write", "read foo", "read admin write"} {
		t.Run(raw, func(t *testing.T) {
			store.stored = nil
			loc := location(t, authorize(t, s, authorizeQuery(raw)))
			assert.Equal(t, "http://localhost:33418/oauth/callback", loc.Scheme+"://"+loc.Host+loc.Path, "the error goes back to the client")
			assert.Equal(t, "invalid_scope", loc.Query().Get("error"))
			assert.NotEmpty(t, loc.Query().Get("error_description"))
			assert.Equal(t, "xyz", loc.Query().Get("state"))
			assert.Empty(t, loc.Query().Get("code"))
			assert.Nil(t, store.stored, "no code is issued")
		})
	}
}

// scopeStore hands the token endpoint a code or a refresh token with a known scope and records what gets issued.
type scopeStore struct {
	fakeRefreshStore
	code *MCPAuthCode
}

func (s *scopeStore) ConsumeCode(context.Context, string) (*MCPAuthCode, error) {
	return s.code, nil
}

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func postToken(t *testing.T, s *OAuthServer, form url.Values) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	form.Set("client_id", "maintenant-mcp")
	form.Set("client_secret", "secret")
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.HandleToken(rec, req)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return rec, body
}

// FR-011: the code exchange issues both tokens with the code's scope and says so in the response.
func TestHandleToken_CodeExchangeCarriesTheScope(t *testing.T) {
	for _, scope := range []string{"read", "read write"} {
		t.Run(scope, func(t *testing.T) {
			store := &scopeStore{code: &MCPAuthCode{
				ClientID:      "maintenant-mcp",
				RedirectURI:   "http://localhost:33418/oauth/callback",
				CodeChallenge: pkceChallenge("verifier-1"),
				Scope:         scope,
				ExpiresAt:     time.Now().Add(time.Minute),
			}}
			s := newTestServerWithStore(store)

			rec, body := postToken(t, s, url.Values{
				"grant_type":    {"authorization_code"},
				"code":          {"the-code"},
				"redirect_uri":  {"http://localhost:33418/oauth/callback"},
				"code_verifier": {"verifier-1"},
			})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, scope, body["scope"], "the response names the granted scope")
			require.Len(t, store.storedTokens, 2)
			for _, tok := range store.storedTokens {
				assert.Equal(t, scope, tok.Scope, "%s token", tok.TokenType)
			}
		})
	}
}

func refreshStore(scope string) *scopeStore {
	return &scopeStore{fakeRefreshStore: fakeRefreshStore{consumeToken: &MCPOAuthToken{
		TokenType: "refresh", ClientID: "maintenant-mcp", Scope: scope, FamilyID: "fam-1",
		ExpiresAt: time.Now().Add(time.Hour),
	}}}
}

// US2 scenario 7, FR-011, FR-013: a refresh keeps the scope of the refresh token, an empty one reading as read.
func TestHandleRefreshToken_KeepsTheScope(t *testing.T) {
	for stored, want := range map[string]string{"read write": "read write", "read": "read", "": "read"} {
		t.Run("["+stored+"]", func(t *testing.T) {
			store := refreshStore(stored)
			rec, body := postToken(t, newTestServerWithStore(store), url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"old"}})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, want, body["scope"])
			require.Len(t, store.storedTokens, 2)
			for _, tok := range store.storedTokens {
				assert.Equal(t, want, tok.Scope, "%s token", tok.TokenType)
			}
		})
	}
}

// US2 scenario 7: a read refresh token asking for write at renewal never gets it, whether the request is
// refused (invalid_scope, nothing issued) or the extra scope is ignored (a read pair).
func TestHandleRefreshToken_ReadTokenCannotGrowToWrite(t *testing.T) {
	store := refreshStore("read")
	rec, body := postToken(t, newTestServerWithStore(store), url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {"old"}, "scope": {"read write"},
	})
	if rec.Code != http.StatusOK {
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "invalid_scope", body["error"])
		assert.Empty(t, store.storedTokens, "a refused refresh issues nothing")
		return
	}
	assert.Equal(t, "read", body["scope"])
	require.Len(t, store.storedTokens, 2)
	for _, tok := range store.storedTokens {
		assert.Equal(t, "read", tok.Scope, "%s token", tok.TokenType)
	}
}

type tokenStore struct {
	fakeStore
	token *MCPOAuthToken
}

func (s *tokenStore) GetToken(context.Context, string) (*MCPOAuthToken, error) { return s.token, nil }

// FR-011, FR-013: the verifier exposes the scopes to the MCP server, an empty stored scope reading as read.
func TestNewTokenVerifier_ExposesTheScopes(t *testing.T) {
	for stored, want := range map[string][]string{
		"read":       {"read"},
		"read write": {"read", "write"},
		"":           {"read"},
	} {
		t.Run("["+stored+"]", func(t *testing.T) {
			verify := NewTokenVerifier(&tokenStore{token: &MCPOAuthToken{
				TokenType: "access", ClientID: "maintenant-mcp", Scope: stored, FamilyID: "fam",
				ExpiresAt: time.Now().Add(time.Hour),
			}})
			info, err := verify(context.Background(), "raw-token", nil)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, want, info.Scopes)
			assert.Equal(t, "maintenant-mcp", info.Extra["client_id"], "the existing extras stay")
		})
	}
}

// The verifier still refuses what it refused before: a scope never rescues a revoked or expired token.
func TestNewTokenVerifier_StillRefusesRevokedAndExpiredTokens(t *testing.T) {
	for name, tok := range map[string]*MCPOAuthToken{
		"revoked": {TokenType: "access", Scope: "read write", Revoked: true, ExpiresAt: time.Now().Add(time.Hour)},
		"expired": {TokenType: "access", Scope: "read write", ExpiresAt: time.Now().Add(-time.Minute)},
		"refresh": {TokenType: "refresh", Scope: "read write", ExpiresAt: time.Now().Add(time.Hour)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewTokenVerifier(&tokenStore{token: tok})(context.Background(), "raw", nil)
			assert.ErrorIs(t, err, auth.ErrInvalidToken)
		})
	}
}

var _ = slog.New(slog.NewTextHandler(io.Discard, nil))
