// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
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

	"github.com/kolapsis/maintenant/internal/mcp/oauth"
)

type oauthStart struct {
	t      *testing.T
	secret string
	store  *MCPOAuthStoreImpl
	srv    *oauth.OAuthServer
	verify auth.TokenVerifier
}

func startOAuth(t *testing.T, db *DB, secret string) *oauthStart {
	t.Helper()
	s := NewMCPOAuthStore(db)
	s.BindClientSecret(oauth.HashToken(secret))
	return &oauthStart{
		t: t, secret: secret, store: s,
		srv: oauth.NewOAuthServer(oauth.Config{ClientID: "claude", ClientSecret: secret, IssuerURL: "https://mnt.example.com"},
			s, slog.New(slog.NewTextHandler(io.Discard, nil))),
		verify: oauth.NewTokenVerifier(s),
	}
}

func (o *oauthStart) code() string {
	o.t.Helper()
	challenge := sha256.Sum256([]byte("verifier"))
	rec := httptest.NewRecorder()
	o.srv.HandleAuthorize(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+url.Values{
		"response_type": {"code"}, "client_id": {"claude"}, "redirect_uri": {"http://localhost:7777/cb"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"},
	}.Encode(), nil))
	require.Equal(o.t, http.StatusFound, rec.Code, rec.Body.String())
	location, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(o.t, err)
	return location.Query().Get("code")
}

func (o *oauthStart) token(form url.Values) (int, map[string]any) {
	o.t.Helper()
	form.Set("client_id", "claude")
	form.Set("client_secret", o.secret)
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	o.srv.HandleToken(rec, req)
	var body map[string]any
	require.NoError(o.t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return rec.Code, body
}

func (o *oauthStart) exchange(code string) (int, map[string]any) {
	return o.token(url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {"http://localhost:7777/cb"}, "code_verifier": {"verifier"},
	})
}

func TestMCPOAuth_ChangingTheClientSecretSignsEveryClientOut(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := startOAuth(t, db, "secret-A")
	status, pair := first.exchange(first.code())
	require.Equal(t, http.StatusOK, status, pair)
	access, refresh := pair["access_token"].(string), pair["refresh_token"].(string)
	pendingCode := first.code()

	var stored string
	require.NoError(t, db.Reader().QueryRowContext(ctx, `SELECT secret_fingerprint FROM mcp_oauth_client`).Scan(&stored))
	assert.Equal(t, oauth.HashToken("secret-A"), stored)
	assert.NotContains(t, stored, "secret-A")

	same := startOAuth(t, db, "secret-A")
	_, err := same.verify(ctx, access, nil)
	require.NoError(t, err, "a restart under the same secret keeps the tokens")

	rotated := startOAuth(t, db, "secret-B")
	_, err = rotated.verify(ctx, access, nil)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
	status, body := rotated.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	assert.Equal(t, http.StatusBadRequest, status, body)
	assert.Equal(t, "invalid_grant", body["error"])
	status, body = rotated.exchange(pendingCode)
	assert.Equal(t, http.StatusBadRequest, status, body)
	assert.Equal(t, "invalid_grant", body["error"])

	status, pair = rotated.exchange(rotated.code())
	require.Equal(t, http.StatusOK, status, pair)
	_, err = rotated.verify(ctx, pair["access_token"].(string), nil)
	require.NoError(t, err, "a client that signs in again under the new secret is served")

	back := startOAuth(t, db, "secret-A")
	_, err = back.verify(ctx, access, nil)
	assert.ErrorIs(t, err, auth.ErrInvalidToken, "what was revoked stays revoked")
}

func TestMCPOAuth_FirstStartAfterTheUpgradeKeepsTheTokens(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewMCPOAuthStore(db)
	require.NoError(t, s.StoreToken(ctx, &oauth.MCPOAuthToken{
		TokenHash: oauth.HashToken("issued-before"), TokenType: "access", ClientID: "claude", Scope: "read",
		ExpiresAt: time.Now().Add(time.Hour), FamilyID: "fam", CreatedAt: time.Now().Add(time.Hour),
	}))

	started := startOAuth(t, db, "secret-A")
	_, err := started.verify(ctx, "issued-before", nil)
	require.NoError(t, err)
	_, err = startOAuth(t, db, "secret-B").verify(ctx, "issued-before", nil)
	assert.ErrorIs(t, err, auth.ErrInvalidToken, "the secret it was first seen under is the one recorded")
}

func TestMigration52_DownAndUpRunWithARecordedFingerprint(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	startOAuth(t, db, "secret-A").code()

	for _, direction := range []string{"down", "up"} {
		matches, err := fs.Glob(migrationFS, "migrations/"+db.dialect.String()+"/52_*."+direction+".sql")
		require.NoError(t, err)
		require.Len(t, matches, 1)
		sqlText, err := fs.ReadFile(migrationFS, matches[0])
		require.NoError(t, err)
		_, err = db.ReadDB().ExecContext(ctx, string(sqlText))
		require.NoError(t, err, direction)
	}

	startOAuth(t, db, "secret-B").code()
	var stored string
	require.NoError(t, db.Reader().QueryRowContext(ctx, `SELECT secret_fingerprint FROM mcp_oauth_client`).Scan(&stored))
	assert.Equal(t, oauth.HashToken("secret-B"), stored)
}
