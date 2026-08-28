package oauth

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// fakeStore is a no-op MCPOAuthStore used to exercise HandleAuthorize without a database.
type fakeStore struct {
	stored *MCPAuthCode
}

func (f *fakeStore) StoreCode(_ context.Context, code *MCPAuthCode) error { f.stored = code; return nil }
func (f *fakeStore) ConsumeCode(context.Context, string) (*MCPAuthCode, error) {
	return nil, nil
}
func (f *fakeStore) StoreToken(context.Context, *MCPOAuthToken) error      { return nil }
func (f *fakeStore) GetToken(context.Context, string) (*MCPOAuthToken, error) {
	return nil, nil
}
func (f *fakeStore) RevokeToken(context.Context, string) error    { return nil }
func (f *fakeStore) RevokeFamily(context.Context, string) error   { return nil }
func (f *fakeStore) DeleteExpired(context.Context) (int64, error) { return 0, nil }

func newTestServer(allowed string) *OAuthServer {
	return NewOAuthServer(Config{
		ClientID:            "maintenant-mcp",
		ClientSecret:        "secret",
		IssuerURL:           "https://now.kolapsis.com",
		AllowedRedirectURIs: allowed,
	}, &fakeStore{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestIsRedirectURIAllowed(t *testing.T) {
	s := newTestServer("https://claude.ai/api/mcp/auth_callback")

	tests := []struct {
		name string
		uri  string
		want bool
	}{
		{"exact full URI listed (Claude web)", "https://claude.ai/api/mcp/auth_callback", true},
		{"origin only does not match full URI", "https://claude.ai", false},
		{"different path on same host (open redirect)", "https://claude.ai/evil", false},
		{"unlisted remote host", "https://evil.example.com/cb", false},
		{"loopback localhost any port/path", "http://localhost:33418/oauth/callback", true},
		{"loopback 127.0.0.1", "http://127.0.0.1:8080/cb", true},
		{"loopback ::1", "http://[::1]:9000/cb", true},
		{"loopback https", "https://localhost:8443/cb", true},
		{"unparseable", "://nope", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.isRedirectURIAllowed(tt.uri); got != tt.want {
				t.Errorf("isRedirectURIAllowed(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestHandleAuthorize_RejectsUnlistedRedirectURI(t *testing.T) {
	s := newTestServer("https://claude.ai/api/mcp/auth_callback")

	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {"maintenant-mcp"},
		"redirect_uri":          {"https://evil.example.com/cb"},
		"code_challenge":        {"abc"},
		"code_challenge_method": {"S256"},
		"state":                 {"xyz"},
	}
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)
	rec := httptest.NewRecorder()

	s.HandleAuthorize(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAuthorize_RedirectsWithCodeForListedURI(t *testing.T) {
	s := newTestServer("https://claude.ai/api/mcp/auth_callback")

	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {"maintenant-mcp"},
		"redirect_uri":          {"https://claude.ai/api/mcp/auth_callback"},
		"code_challenge":        {"abc"},
		"code_challenge_method": {"S256"},
		"state":                 {"xyz"},
	}
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)
	rec := httptest.NewRecorder()

	s.HandleAuthorize(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if loc.Scheme+"://"+loc.Host+loc.Path != "https://claude.ai/api/mcp/auth_callback" {
		t.Errorf("redirect target = %q, want the configured callback", loc.String())
	}
	if loc.Query().Get("code") == "" {
		t.Error("redirect missing authorization code")
	}
	if loc.Query().Get("state") != "xyz" {
		t.Errorf("state = %q, want xyz", loc.Query().Get("state"))
	}
}
