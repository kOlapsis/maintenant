// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCP_StartingUnderAnotherClientSecretRefusesTheOldTokens(t *testing.T) {
	a := oauthApp(t, nil)
	status := func(bearer string) int {
		resp, _ := postInitializeViaLocalProxy(t, a, bearer)
		return resp.StatusCode
	}
	issued := authorizeAndExchange(t, a, "read")
	require.Equal(t, http.StatusOK, status(issued.access))

	a.srv = a.buildHTTPServer()
	assert.Equal(t, http.StatusOK, status(issued.access), "a restart under the same secret keeps the token")

	a.cfg.MCP.Enabled = false
	a.srv = a.buildHTTPServer()
	a.cfg.MCP.Enabled = true
	a.cfg.MCP.ClientSecret = strings.Repeat("9c", 32)
	a.srv = a.buildHTTPServer()
	assert.Equal(t, http.StatusUnauthorized, status(issued.access), "MCP switched off and on again under a new secret")
}
