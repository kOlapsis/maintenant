// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package v1

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/stretchr/testify/require"
)

// wiringStubRuntime satisfies runtime.Runtime via embedding; only the methods
// the router touches during construction and log fetching are implemented.
type wiringStubRuntime struct {
	runtime.Runtime
}

func (s *wiringStubRuntime) FetchLogs(_ context.Context, _ string, _ int, _ bool) ([]string, error) {
	return []string{"line"}, nil
}
func (s *wiringStubRuntime) IsConnected() bool { return true }

// TestRouter_WiresLogFetcher pins the REST logs endpoint wiring: the container
// handler must receive the runtime as its LogFetcher, otherwise
// GET /api/v1/containers/{id}/logs always answers RUNTIME_UNAVAILABLE even
// with a healthy runtime (the SSE-fallback path in the UI).
func TestRouter_WiresLogFetcher(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := NewRouter(HandlerDeps{
		Logger:  logger,
		Runtime: &wiringStubRuntime{},
	})

	require.NotNil(t, r.containerHandler)
	require.NotNil(t, r.containerHandler.logFetcher,
		"router must wire the runtime as the container handler's LogFetcher")
}

// TestRouter_NoRuntime_LeavesLogFetcherNil covers the degraded construction
// path: with no runtime at all, the handler keeps answering RUNTIME_UNAVAILABLE
// instead of panicking on a nil interface call.
func TestRouter_NoRuntime_LeavesLogFetcherNil(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := NewRouter(HandlerDeps{Logger: logger})

	require.NotNil(t, r.containerHandler)
	require.Nil(t, r.containerHandler.logFetcher)
}
