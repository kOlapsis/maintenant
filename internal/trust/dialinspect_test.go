// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package trust

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialInspect_ReturnsPeerCertificates(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "https://")

	state, err := DialInspect(context.Background(), addr, "example.com", time.Second)

	require.NoError(t, err)
	assert.NotEmpty(t, state.PeerCertificates, "the untrusted certificate must still be returned")
}

func TestDialInspect_ClosedPortIsAnError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close()) // nothing listening any more

	_, err = DialInspect(context.Background(), addr, "example.com", time.Second)

	assert.Error(t, err)
}

func TestDialInspect_RespectsCancelledContext(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "https://")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := DialInspect(ctx, addr, "example.com", time.Second)

	assert.Error(t, err)
}
