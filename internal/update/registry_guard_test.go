// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryClient_RefusesGuardedAddressesAndReachesLoopback(t *testing.T) {
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	_, port, err := net.SplitHostPort(host)
	require.NoError(t, err)

	img, err := random.Image(64, 1)
	require.NoError(t, err)
	ref, err := name.ParseReference(host + "/acme/web:1.0")
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, img))

	rc := NewRegistryClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tags, err := rc.ListTags(ctx, host+"/acme/web")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0"}, tags)

	for _, addr := range []string{"0.0.0.0:" + port, "169.254.169.254:" + port} {
		t.Run(addr, func(t *testing.T) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/v2/", nil)
			require.NoError(t, err)

			resp, err := rc.transport.RoundTrip(req)
			if err == nil {
				_ = resp.Body.Close()
			}

			require.ErrorContains(t, err, "probe guard: blocked connection to ")
		})
	}
}
