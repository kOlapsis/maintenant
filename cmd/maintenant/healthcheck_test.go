// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeServerHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	addr := srv.Listener.Addr().String()
	assert.NoError(t, probeServerHealth(addr, time.Second))
}

func TestProbeServerHealth_NonOKStatusIsUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := probeServerHealth(srv.Listener.Addr().String(), time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

// A server listening on every interface is still probed over loopback: the
// healthcheck runs inside the container and has no business dialing the
// published address.
func TestProbeServerHealth_WildcardAddressGoesToLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &httptest.Server{
		Listener: ln,
		Config:   &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })},
	}
	srv.Start()
	defer srv.Close()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)

	assert.NoError(t, probeServerHealth("0.0.0.0:"+port, time.Second))
	assert.NoError(t, probeServerHealth(":"+port, time.Second))
}

func TestProbeServerHealth_RejectsMalformedAddress(t *testing.T) {
	err := probeServerHealth("not-an-address", time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid listen address")
}

// Nothing listening is the plain case a container healthcheck exists for.
func TestProbeServerHealth_ClosedPortIsUnhealthy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	assert.Error(t, probeServerHealth(addr, time.Second))
}
