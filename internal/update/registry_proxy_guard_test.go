// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingProxy struct {
	mu    sync.Mutex
	asked []string
}

func (p *recordingProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.asked = append(p.asked, r.Method+" "+r.Host)
	p.mu.Unlock()
	http.Error(w, "no route", http.StatusBadGateway)
}

func (p *recordingProxy) requests() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.asked...)
}

func TestRegistryClient_RefusesGuardedLiteralsBeforeTheProxy(t *testing.T) {
	proxy := &recordingProxy{}
	srv := httptest.NewServer(proxy)
	defer srv.Close()
	proxyURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	rc := NewRegistryClient()
	rc.transport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, registry := range []string{"169.254.169.254", "169.254.169.254:5000", "[fe80::1]:5000", "0.0.0.0:5000"} {
		t.Run(registry, func(t *testing.T) {
			_, err := rc.ListTags(ctx, registry+"/acme/web")
			require.ErrorContains(t, err, "probe guard: blocked connection to ")
		})
	}
	assert.Empty(t, proxy.requests(), "nothing may be handed to the proxy for a guarded address")

	_, err = rc.ListTags(ctx, "registry.example.com/acme/web")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "probe guard")
	assert.NotEmpty(t, proxy.requests(), "a named registry still goes through the proxy")
}
