// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"net"
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
	rc.outbound.(*destinationGuard).lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
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

func TestRegistryClient_RefusesNamesResolvingToGuardedAddressesBeforeTheProxy(t *testing.T) {
	proxy := &recordingProxy{}
	srv := httptest.NewServer(proxy)
	defer srv.Close()
	proxyURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	rc := NewRegistryClient()
	guard := rc.outbound.(*destinationGuard)
	var mu sync.Mutex
	var lookups []string
	guard.lookup = func(_ context.Context, host string) ([]net.IPAddr, error) {
		mu.Lock()
		lookups = append(lookups, host)
		mu.Unlock()
		switch host {
		case "metadata.internal":
			return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
		case "mixed.internal":
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.5")}, {IP: net.ParseIP("fe80::1")}}, nil
		case "registry.example.com":
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return nil, errors.New("no such host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("without a proxy the dial guard is enough and nothing is resolved twice", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1/v2/", nil)
		require.NoError(t, err)
		req.URL.Host = "name.invalid:1"
		_, err = guard.RoundTrip(req)
		require.Error(t, err)
		assert.Empty(t, lookups)
	})

	guard.next.Proxy = http.ProxyURL(proxyURL)
	for _, registry := range []string{"metadata.internal", "mixed.internal:5000"} {
		t.Run(registry, func(t *testing.T) {
			_, err := rc.ListTags(ctx, registry+"/acme/web")
			require.ErrorContains(t, err, "probe guard: blocked connection to ")
		})
	}
	assert.Empty(t, proxy.requests(), "nothing may be handed to the proxy for a name that resolves to a guarded address")

	for _, registry := range []string{"registry.example.com", "unresolved.internal"} {
		_, err = rc.ListTags(ctx, registry+"/acme/web")
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "probe guard", registry)
	}
	assert.Len(t, proxy.requests(), 2, "a public name and a name we cannot resolve are left to the proxy")
}
