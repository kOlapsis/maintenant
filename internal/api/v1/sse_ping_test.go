// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13: a proxy sees the stream as open from the first byte.

// readLine reads one line, or fails once the deadline is past.
func readLine(t *testing.T, r *bufio.Reader, deadline time.Duration) string {
	t.Helper()
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := r.ReadString('\n')
		done <- result{line, err}
	}()
	select {
	case res := <-done:
		require.NoError(t, res.err)
		return strings.TrimRight(res.line, "\n")
	case <-time.After(deadline):
		t.Fatalf("nothing arrived within %s", deadline)
		return ""
	}
}

// FR-005, US2 scenario 3: the headers and a ": ping" comment are flushed on acceptance, before any event or keep-alive.
func TestSSEBroker_PingsAsSoonAsTheStreamOpens(t *testing.T) {
	for _, path := range []string{"/status/events", "/api/v1/containers/events"} {
		t.Run(path, func(t *testing.T) {
			broker := NewSSEBroker(slog.New(slog.NewTextHandler(io.Discard, nil)))
			srv := httptest.NewServer(broker)
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+path, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err, "the headers must be sent before the first event")
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			assertUnbufferedEventStream(t, resp.Header)
			assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))

			r := bufio.NewReader(resp.Body)
			assert.Equal(t, ": ping", readLine(t, r, 2*time.Second), "the first bytes are the ping comment")
			assert.Equal(t, "", readLine(t, r, 2*time.Second), "a comment ends with a blank line")

			broker.Broadcast(SSEEvent{Type: "container.state", Data: map[string]string{"id": "c1"}})
			assert.Equal(t, "event: container.state", readLine(t, r, 2*time.Second), "events follow the ping")
		})
	}
}
