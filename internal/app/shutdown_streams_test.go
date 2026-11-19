// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func instanceRows(t *testing.T, dbPath, id string) int {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM instances WHERE id = ?`, id).Scan(&n))
	return n
}

func TestShutdown_OpenEventStreamDoesNotHoldTheGrace(t *testing.T) {
	cfg, logs, logger := storageEnv(t)
	cfg.Addr = loopbackPort(t)
	a, err := New(cfg, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()

	var stream *http.Response
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + cfg.Addr + "/api/v1/containers/events") // #nosec G107 -- loopback test server
		if err != nil {
			return false
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return false
		}
		stream = resp
		return true
	}, 10*time.Second, 50*time.Millisecond, "the event stream must open")
	defer func() { _ = stream.Body.Close() }()
	require.Equal(t, 1, instanceRows(t, cfg.DBPath, a.instanceID), "the instance is registered while it runs")

	began := time.Now()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("Start did not return after cancellation")
	}

	assert.Less(t, time.Since(began), 3*time.Second, "an open event stream must not hold the shutdown grace")
	assert.False(t, strings.Contains(logs.String(), "HTTP server shutdown error"), "the HTTP server drains within the grace")
	assert.False(t, strings.Contains(logs.String(), "instance deregistration failed"), "deregistration gets a live context")
	assert.Zero(t, instanceRows(t, cfg.DBPath, a.instanceID), "the instance row is dropped at shutdown")
}
