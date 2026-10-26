// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Shared helpers of the 043-public-surfaces-hardening acceptance tests.

var testIndexHTML = []byte(`<!DOCTYPE html><html><head><title>maintenant</title></head><body><div id="app"></div></body></html>`)

// hit serves one request through the whole HTTP stack. Host defaults to
// localhost, which every configuration accepts; mod adjusts the request first.
func hit(a *App, method, target string, mod func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = "localhost"
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	a.srv.Handler.ServeHTTP(rec, req)
	return rec
}

func host(h string) func(*http.Request) {
	return func(r *http.Request) { r.Host = h }
}

func from(addr string) func(*http.Request) {
	return func(r *http.Request) { r.RemoteAddr = addr }
}

func newHeartbeat(t *testing.T, a *App, name string) *heartbeat.Heartbeat {
	t.Helper()
	hb, err := a.heartbeatSvc.CreateHeartbeat(context.Background(), heartbeat.CreateHeartbeatInput{Name: name, IntervalSeconds: 300, GraceSeconds: 60}, uid.New())
	require.NoError(t, err)
	return hb
}

// warnLines returns the WARN log lines that mention needle.
func warnLines(logs *syncBuffer, needle string) []string {
	var out []string
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "level=WARN") && strings.Contains(line, needle) {
			out = append(out, line)
		}
	}
	return out
}
