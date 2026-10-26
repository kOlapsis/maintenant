// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package trust

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, C7: the inspection dial accepts a control hook.

func TestDialInspectControl_RunsTheHookOnTheDialledAddress(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	var seen string
	state, err := DialInspectControl(context.Background(), addr, "example.com", time.Second,
		func(_, address string, _ syscall.RawConn) error {
			seen = address
			return nil
		})

	require.NoError(t, err)
	assert.NotEmpty(t, state.PeerCertificates)
	assert.Equal(t, addr, seen, "the hook sees the concrete address being dialled")
}

func TestDialInspectControl_HookErrorAbortsBeforeConnecting(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	var accepted atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = c.Close()
		}
	}()

	refused := errors.New("probe guard: blocked connection to 127.0.0.1")
	_, err = DialInspectControl(context.Background(), ln.Addr().String(), "example.com", time.Second,
		func(_, _ string, _ syscall.RawConn) error { return refused })

	require.Error(t, err)
	assert.ErrorIs(t, err, refused)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), accepted.Load(), "no connection may be opened when the hook refuses")
}

func TestDialInspectControl_NilHookBehavesLikeDialInspect(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	state, err := DialInspectControl(context.Background(), addr, "example.com", time.Second, nil)

	require.NoError(t, err)
	assert.NotEmpty(t, state.PeerCertificates)
}
