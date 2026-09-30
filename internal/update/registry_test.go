// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/trust/trusttest"
)

func TestRegistryClient_TrustsTheConfiguredCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	trusttest.Trust(t, srv.Certificate())

	rc := NewRegistryClient()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v2/", nil)
	require.NoError(t, err)
	resp, err := rc.transport.RoundTrip(req)

	require.NoError(t, err, "a private registry behind MAINTENANT_CA_CERT must be trusted")
	_ = resp.Body.Close()
}

func TestRegistryClient_ResolveDigests(t *testing.T) {
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")

	amd64, err := random.Image(64, 1)
	require.NoError(t, err)
	arm64, err := random.Image(64, 1)
	require.NoError(t, err)
	index := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: amd64, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}},
		mutate.IndexAddendum{Add: arm64, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}},
	)
	multi, err := name.ParseReference(host + "/acme/web:latest")
	require.NoError(t, err)
	require.NoError(t, remote.WriteIndex(multi, index))
	single, err := name.ParseReference(host + "/acme/tool:1.0")
	require.NoError(t, err)
	require.NoError(t, remote.Write(single, amd64))

	digestOf := func(d interface{ Digest() (v1.Hash, error) }) string {
		h, err := d.Digest()
		require.NoError(t, err)
		return h.String()
	}
	rc := NewRegistryClient()

	got, err := rc.ResolveDigests(context.Background(), host+"/acme/web:latest")
	require.NoError(t, err)
	assert.Equal(t, digestOf(index), got.Digest, "runtimes record the index digest of a multi-platform tag")
	assert.ElementsMatch(t, []string{digestOf(amd64), digestOf(arm64)}, got.Platforms)
	assert.True(t, got.Contains(digestOf(arm64)))
	assert.False(t, got.Contains("sha256:other"))

	got, err = rc.ResolveDigests(context.Background(), host+"/acme/tool:1.0")
	require.NoError(t, err)
	assert.Equal(t, digestOf(amd64), got.Digest)
	assert.Empty(t, got.Platforms)
}
