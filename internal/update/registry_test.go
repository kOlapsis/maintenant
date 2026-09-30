// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
