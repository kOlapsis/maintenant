// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mcpInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"%s","version":"0"}}}`

func initializeBody(name string) string {
	return strings.Replace(mcpInitialize, "%s", name, 1)
}

func TestOAuthTokenBodyIsBounded(t *testing.T) {
	a := oauthApp(t, nil)
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}

	small := url.Values{"grant_type": {"bogus"}}.Encode()
	rec := serve(a, http.MethodPost, "/oauth/token", small, form)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a normal body still reaches the handler")
	assert.Contains(t, rec.Body.String(), "unsupported_grant_type")

	big := url.Values{"grant_type": {"bogus"}, "padding": {strings.Repeat("a", 1<<20)}}.Encode()
	rec = serve(a, http.MethodPost, "/oauth/token", big, form)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.NotContains(t, rec.Body.String(), "unsupported_grant_type", "an oversized body must be refused before it is parsed")
}

func TestOAuthTokenBodyWithoutLengthIsBounded(t *testing.T) {
	a := oauthApp(t, nil)
	big := url.Values{"grant_type": {"bogus"}, "padding": {strings.Repeat("a", 1<<20)}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(big))
	req.ContentLength = -1
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.srv.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.NotContains(t, rec.Body.String(), "unsupported_grant_type")
}

func TestMCPBodyIsBounded(t *testing.T) {
	a := oauthApp(t, func(c *Config) { c.MaxBodySize = 8192 })
	tok := authorizeAndExchange(t, a, "")
	headers := map[string]string{
		"Authorization": "Bearer " + tok.access,
		"Content-Type":  "application/json",
		"Accept":        "application/json, text/event-stream",
	}

	rec := serve(a, http.MethodPost, "/mcp", initializeBody("ok"), headers)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = serve(a, http.MethodPost, "/mcp", initializeBody(strings.Repeat("a", 64<<10)), headers)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
}
