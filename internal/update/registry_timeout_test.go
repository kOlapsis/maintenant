// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func silentRegistry(t *testing.T) string {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return strings.TrimPrefix(srv.URL, "http://")
}

func returnsWithin(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not return after its context expired", what)
	}
}

func TestRegistryClient_StopsWhenContextExpires(t *testing.T) {
	host := silentRegistry(t)
	rc := NewRegistryClient()

	calls := map[string]func(ctx context.Context) error{
		"ListTags": func(ctx context.Context) error {
			_, err := rc.ListTags(ctx, host+"/acme/api")
			return err
		},
		"ResolveDigests": func(ctx context.Context) error {
			_, err := rc.ResolveDigests(ctx, host+"/acme/api:1.0.0")
			return err
		},
		"GetManifest": func(ctx context.Context) error {
			_, err := rc.GetManifest(ctx, host+"/acme/api:1.0.0")
			return err
		},
		"GetConfigLabels": func(ctx context.Context) error {
			_, err := rc.GetConfigLabels(ctx, host+"/acme/api:1.0.0")
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			var err error
			returnsWithin(t, 5*time.Second, name, func() { err = call(ctx) })
			assert.Error(t, err)
		})
	}
}

func TestRegistryClient_TransportBoundsResponseHeaders(t *testing.T) {
	rc := NewRegistryClient()
	tr, ok := rc.transport.(*http.Transport)
	require.True(t, ok)
	assert.Greater(t, tr.ResponseHeaderTimeout, time.Duration(0))
}

func TestScanner_SilentRegistryDoesNotBlockTheScan(t *testing.T) {
	host := silentRegistry(t)
	sc := newTestScanner(NewRegistryClient(), &stubStore{})
	sc.imageTimeout = 200 * time.Millisecond

	containers := []ContainerInfo{
		{ExternalID: "ctr1", Name: "api", Image: host + "/acme/api:1.0.0"},
		{ExternalID: "ctr2", Name: "web", Image: host + "/acme/web:2.0.0"},
	}
	for scan := 1; scan <= 2; scan++ {
		var errs []ScanError
		returnsWithin(t, 10*time.Second, "Scan", func() {
			_, errs = sc.Scan(context.Background(), containers)
		})
		assert.Len(t, errs, 2, "scan %d: each silent image fails on its own deadline", scan)
	}
}
