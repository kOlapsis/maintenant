// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/trust/trusttest"
)

func TestNewManager_TrustsTheConfiguredCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	trusttest.Trust(t, srv.Certificate())

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	prev := publicKeyB64
	InitPublicKey(base64.StdEncoding.EncodeToString(pub))
	t.Cleanup(func() { publicKeyB64 = prev })

	m, err := NewManager("key", t.TempDir(), "test", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	resp, err := m.client.Get(srv.URL)
	require.NoError(t, err, "a license server behind MAINTENANT_CA_CERT must be trusted")
	_ = resp.Body.Close()
}
