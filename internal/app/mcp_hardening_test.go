// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	mcpoauth "github.com/kolapsis/maintenant/internal/mcp/oauth"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Acceptance tests of 045-mcp-hardening through the real router: the OAuth
// flow, the scope check on tools/call and the read-only registry on /mcp.

const mcpSecret = "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f"

func oauthApp(t *testing.T, edit func(*Config)) *App {
	t.Helper()
	a, _ := newTestApp(t, func(c *Config) {
		c.MCP.Enabled = true
		c.MCP.ClientID = "claude"
		c.MCP.ClientSecret = mcpSecret
		c.BaseURL = "https://maintenant.example.com"
		if edit != nil {
			edit(c)
		}
	})
	return a
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession opens an MCP session on /mcp through a real HTTP listener, with or without a bearer token.
func mcpSession(t *testing.T, a *App, bearer string) *gomcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(a.srv.Handler)
	t.Cleanup(srv.Close)
	cs, err := gomcp.NewClient(&gomcp.Implementation{Name: "acceptance", Version: "0"}, nil).Connect(
		context.Background(),
		&gomcp.StreamableClientTransport{
			Endpoint:             srv.URL + "/mcp",
			HTTPClient:           &http.Client{Transport: bearerTransport{bearer}, Timeout: 10 * time.Second},
			DisableStandaloneSSE: true,
			MaxRetries:           -1,
		}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type issuedTokens struct {
	access, refresh, scope string
}

// authorizeAndExchange runs the code flow against the app's own routes and returns the issued pair.
func authorizeAndExchange(t *testing.T, a *App, scope string) issuedTokens {
	t.Helper()
	const verifier = "acceptance-verifier-0123456789"
	h := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {"claude"},
		"redirect_uri":          {"http://localhost:33418/cb"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(h[:])},
		"code_challenge_method": {"S256"},
		"state":                 {"s"},
	}
	if scope != "" {
		q.Set("scope", scope)
	}
	rec := serve(a, http.MethodGet, "/oauth/authorize?"+q.Encode(), "", nil)
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	require.Empty(t, loc.Query().Get("error"), loc.String())
	code := loc.Query().Get("code")
	require.NotEmpty(t, code)

	return exchange(t, a, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:33418/cb"},
		"code_verifier": {verifier},
	})
}

func exchange(t *testing.T, a *App, form url.Values) issuedTokens {
	t.Helper()
	form.Set("client_id", "claude")
	form.Set("client_secret", mcpSecret)
	rec := serve(a, http.MethodPost, "/oauth/token", form.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotEmpty(t, body.AccessToken)
	return issuedTokens{access: body.AccessToken, refresh: body.RefreshToken, scope: body.Scope}
}

func insertActiveAlert(t *testing.T, a *App, entity string) string {
	t.Helper()
	id, err := store.NewAlertStore(a.db).InsertAlert(context.Background(), &alert.Alert{
		Source: alert.SourceContainer, AlertType: "health_unhealthy", Severity: alert.SeverityWarning,
		Status: alert.StatusActive, Message: "unhealthy", EntityType: "container", EntityID: entity,
		EntityName: entity, FiredAt: time.Now(),
	})
	require.NoError(t, err)
	return id
}

func alertByID(t *testing.T, a *App, id string) *alert.Alert {
	t.Helper()
	stored, err := store.NewAlertStore(a.db).GetAlert(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, stored)
	return stored
}

func text(t *testing.T, res *gomcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, res.Content)
	tc, ok := res.Content[0].(*gomcp.TextContent)
	require.True(t, ok, "%T", res.Content[0])
	return tc.Text
}

func writeRefusal(tool string) string {
	return "tool " + tool + " requires the write scope; this token only grants read"
}

// US2 scenarios 1, 4 and 5, FR-012, SC-002: a token obtained without a scope reads but cannot write.
func TestMCPScope_ReadTokenReadsButCannotWrite(t *testing.T) {
	a := oauthApp(t, nil)
	tokens := authorizeAndExchange(t, a, "")
	assert.Equal(t, "read", tokens.scope, "the default scope is read")
	id := insertActiveAlert(t, a, "web")
	cs := mcpSession(t, a, tokens.access)
	ctx := context.Background()

	for tool, args := range map[string]map[string]any{
		"acknowledge_alert": {"alert_id": id, "acknowledged_by": "claude"},
		"create_channel":    {"name": "hook", "type": "webhook", "url": "https://8.8.8.8/hook"},
		"pause_monitor":     {"monitor_type": "heartbeat", "monitor_id": "hb-1"},
	} {
		res, err := cs.CallTool(ctx, &gomcp.CallToolParams{Name: tool, Arguments: args})
		require.NoError(t, err, tool)
		require.True(t, res.IsError, "%s must be refused: %s", tool, text(t, res))
		assert.Equal(t, writeRefusal(tool), text(t, res))
	}
	assert.Nil(t, alertByID(t, a, id).AcknowledgedAt, "nothing is written")
	channels, err := store.NewChannelStore(a.db).ListChannels(ctx)
	require.NoError(t, err)
	assert.Empty(t, channels, "nothing is written")

	res, err := cs.CallTool(ctx, &gomcp.CallToolParams{Name: "list_alerts", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, res.IsError, text(t, res))
	assert.Contains(t, text(t, res), id, "a read tool answers with the data")
}

// US2 scenarios 2 and 6: a read write token writes as before.
func TestMCPScope_ReadWriteTokenWrites(t *testing.T) {
	for _, requested := range []string{"read write", "write read"} {
		t.Run(requested, func(t *testing.T) {
			a := oauthApp(t, nil)
			tokens := authorizeAndExchange(t, a, requested)
			assert.Equal(t, "read write", tokens.scope)
			id := insertActiveAlert(t, a, "db")
			cs := mcpSession(t, a, tokens.access)

			res, err := cs.CallTool(context.Background(), &gomcp.CallToolParams{Name: "acknowledge_alert", Arguments: map[string]any{"alert_id": id, "acknowledged_by": "claude"}})
			require.NoError(t, err)
			require.False(t, res.IsError, text(t, res))
			stored := alertByID(t, a, id)
			require.NotNil(t, stored.AcknowledgedAt)
			assert.Equal(t, "claude", stored.AcknowledgedBy)
		})
	}
}

// US2 scenario 3 through the router: an unknown scope never reaches the token endpoint.
func TestMCPScope_UnknownScopeIsRefusedOnTheRouter(t *testing.T) {
	a := oauthApp(t, nil)
	q := url.Values{
		"response_type": {"code"}, "client_id": {"claude"}, "redirect_uri": {"http://localhost:33418/cb"},
		"code_challenge": {"abc"}, "code_challenge_method": {"S256"}, "state": {"s"}, "scope": {"admin"},
	}
	rec := serve(a, http.MethodGet, "/oauth/authorize?"+q.Encode(), "", nil)
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "invalid_scope", loc.Query().Get("error"))
	assert.Empty(t, loc.Query().Get("code"))
}

// US2 scenario 7, FR-011, SC-005: the scope survives a refresh through the real store, and never grows.
func TestMCPScope_RefreshKeepsTheScope(t *testing.T) {
	a := oauthApp(t, nil)
	ctx := context.Background()

	writer := authorizeAndExchange(t, a, "read write")
	renewed := exchange(t, a, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {writer.refresh}})
	assert.Equal(t, "read write", renewed.scope)
	id := insertActiveAlert(t, a, "cache")
	res, err := mcpSession(t, a, renewed.access).CallTool(ctx, &gomcp.CallToolParams{Name: "acknowledge_alert", Arguments: map[string]any{"alert_id": id}})
	require.NoError(t, err)
	require.False(t, res.IsError, text(t, res))
	require.NotNil(t, alertByID(t, a, id).AcknowledgedAt)

	reader := authorizeAndExchange(t, a, "")
	stillReader := exchange(t, a, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {reader.refresh}})
	assert.Equal(t, "read", stillReader.scope, "a refresh never adds write")
	other := insertActiveAlert(t, a, "queue")
	res, err = mcpSession(t, a, stillReader.access).CallTool(ctx, &gomcp.CallToolParams{Name: "acknowledge_alert", Arguments: map[string]any{"alert_id": other}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Equal(t, writeRefusal("acknowledge_alert"), text(t, res))
	assert.Nil(t, alertByID(t, a, other).AcknowledgedAt)
}

// FR-013, edge case "jetons émis avant la mise à niveau": an empty stored scope reads and does not write.
func TestMCPScope_LegacyTokenWithEmptyScopeIsRead(t *testing.T) {
	a := oauthApp(t, nil)
	token, hash := mcpoauth.GenerateToken()
	require.NoError(t, store.NewMCPOAuthStore(a.db).StoreToken(context.Background(), &mcpoauth.MCPOAuthToken{
		TokenHash: hash, TokenType: "access", ClientID: "claude", Scope: "",
		ExpiresAt: time.Now().Add(time.Hour), FamilyID: uid.New(), CreatedAt: time.Now(),
	}))
	id := insertActiveAlert(t, a, "legacy")
	cs := mcpSession(t, a, token)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &gomcp.CallToolParams{Name: "list_alerts", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, res.IsError, text(t, res))
	assert.Contains(t, text(t, res), id)

	res, err = cs.CallTool(ctx, &gomcp.CallToolParams{Name: "acknowledge_alert", Arguments: map[string]any{"alert_id": id}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Equal(t, writeRefusal("acknowledge_alert"), text(t, res))
	assert.Nil(t, alertByID(t, a, id).AcknowledgedAt)
}

// FR-001 on /mcp, US2 scenario 8, edge case "outil d'écriture appelé en lecture seule": the token may say
// read write, the write tools still do not exist.
func TestMCPReadOnly_WriteToolsDoNotExistOnHTTP(t *testing.T) {
	a := oauthApp(t, func(c *Config) { c.MCP.ReadOnly = true })
	tokens := authorizeAndExchange(t, a, "read write")
	assert.Equal(t, "read write", tokens.scope, "the token is issued as asked")
	id := insertActiveAlert(t, a, "ro")
	cs := mcpSession(t, a, tokens.access)
	ctx := context.Background()

	tools, err := cs.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 34, len(tools.Tools), "only the read tools exist")
	for _, tool := range tools.Tools {
		require.NotNil(t, tool.Annotations, tool.Name)
		assert.True(t, tool.Annotations.ReadOnlyHint, "%s is listed in read-only mode", tool.Name)
	}

	_, err = cs.CallTool(ctx, &gomcp.CallToolParams{Name: "acknowledge_alert", Arguments: map[string]any{"alert_id": id}})
	require.Error(t, err, "the tool does not exist, it is not a scope refusal")
	assert.Contains(t, err.Error(), "unknown tool")
	assert.NotContains(t, err.Error(), "write scope")
	assert.Nil(t, alertByID(t, a, id).AcknowledgedAt)
}

// US1 scenario 2 on /mcp: without the variable the 51 tools are served.
func TestMCPReadOnly_OffServesEveryTool(t *testing.T) {
	a := oauthApp(t, nil)
	tokens := authorizeAndExchange(t, a, "")
	tools, err := mcpSession(t, a, tokens.access).ListTools(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 51, len(tools.Tools))
}

// Contract: without OAuth there is no token and no scope, so a write tool runs. Passes today; it guards
// a scope check that would refuse every call lacking a token.
func TestMCPScope_UnauthenticatedModeKeepsWriting(t *testing.T) {
	a, _ := newTestApp(t, func(c *Config) {
		c.MCP.Enabled = true
		c.MCP.AllowUnauthenticated = true
	})
	id := insertActiveAlert(t, a, "open")
	res, err := mcpSession(t, a, "").CallTool(context.Background(), &gomcp.CallToolParams{Name: "acknowledge_alert", Arguments: map[string]any{"alert_id": id}})
	require.NoError(t, err)
	require.False(t, res.IsError, text(t, res))
	require.NotNil(t, alertByID(t, a, id).AcknowledgedAt)
}

// Edge case "valeur non booléenne", FR-030: the two variables parse like every other boolean.
func TestConfigFromEnv_MCPReadOnlyAndSocket(t *testing.T) {
	for raw, want := range map[string]bool{"true": true, "1": true, "yes": true, "false": false, "maybe": false, "": false} {
		t.Run("["+raw+"]", func(t *testing.T) {
			t.Setenv("MAINTENANT_MCP_READ_ONLY", raw)
			t.Setenv("MAINTENANT_MCP_SOCKET", raw)
			cfg := ConfigFromEnv()
			assert.Equal(t, want, cfg.MCP.ReadOnly)
			assert.Equal(t, want, cfg.MCP.Socket)
		})
	}
}
