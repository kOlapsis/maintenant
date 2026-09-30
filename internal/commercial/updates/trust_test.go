// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/trust/trusttest"
	"github.com/kolapsis/maintenant/internal/update"
)

func TestCVEClient_TrustsTheConfiguredCA(t *testing.T) {
	var batches atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, _ *http.Request) {
		batches.Add(1)
		_, _ = w.Write([]byte(`{"results":[{"vulns":[]}]}`))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	trusttest.Trust(t, srv.Certificate())

	client := newTestCVEClient(&cveStubStore{}, srv.URL)
	_, err := client.QueryCVEs(context.Background(), []ImageCVEQuery{
		{ContainerID: "c1", PackageName: "curl", Ecosystem: "Debian", Version: "7.0"},
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), batches.Load(), "an OSV mirror behind MAINTENANT_CA_CERT must be reached")
}

func TestChangelogResolver_TrustsTheConfiguredCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	trusttest.Trust(t, srv.Certificate())

	cr := NewChangelogResolver(update.NewRegistryClient(), testLogger())
	resp, err := cr.client.Get(srv.URL)

	require.NoError(t, err, "a changelog API behind MAINTENANT_CA_CERT must be trusted")
	_ = resp.Body.Close()
}
