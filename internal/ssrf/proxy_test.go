// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package ssrf

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func transportOf(t *testing.T, c *http.Client) *http.Transport {
	t.Helper()
	tr, ok := c.Transport.(*http.Transport)
	require.True(t, ok)
	return tr
}

func TestGuardedClientIgnoresTheEnvironmentProxy(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")

	client := NewHTTPClient(2*time.Second, false)
	assert.Nil(t, transportOf(t, client).Proxy, "a proxy would hide the destination from the dial guard")

	_, err := client.Get("http://webhook.invalid/")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrBlockedAddress, "the guard must judge the destination, not the proxy")
	assert.Zero(t, hits.Load(), "the proxy must not receive the request")
}

func TestUnguardedClientKeepsTheEnvironmentProxy(t *testing.T) {
	client := NewHTTPClient(2*time.Second, true)
	assert.NotNil(t, transportOf(t, client).Proxy)
}
