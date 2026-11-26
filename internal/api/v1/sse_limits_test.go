// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openStream(t *testing.T, url string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestSSEBroker_RefusesStreamsBeyondItsCeiling(t *testing.T) {
	broker := NewSSEBroker(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Equal(t, 1000, broker.maxClients)
	broker.maxClients = 2
	srv := httptest.NewServer(broker)
	t.Cleanup(srv.Close)

	for range 2 {
		require.Equal(t, http.StatusOK, openStream(t, srv.URL).StatusCode)
	}

	refused := openStream(t, srv.URL)
	require.Equal(t, http.StatusServiceUnavailable, refused.StatusCode)
	assert.Equal(t, "application/json", refused.Header.Get("Content-Type"))
	assert.Equal(t, 2, broker.ClientCount())
}

func TestSSEBroker_DropsAClientThatStoppedReading(t *testing.T) {
	prev := sseWriteTimeout
	sseWriteTimeout = 50 * time.Millisecond
	t.Cleanup(func() { sseWriteTimeout = prev })

	broker := NewSSEBroker(slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(broker)
	t.Cleanup(srv.Close)

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.(*net.TCPConn).SetReadBuffer(1024))
	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return broker.ClientCount() == 1 }, 5*time.Second, 5*time.Millisecond)

	flood := SSEEvent{Type: "flood", Data: strings.Repeat("x", 1<<20)}
	require.Eventually(t, func() bool {
		broker.Broadcast(flood)
		return broker.ClientCount() == 0
	}, 10*time.Second, 20*time.Millisecond, "a stream nobody reads must be closed")
}
