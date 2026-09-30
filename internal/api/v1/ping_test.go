// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/ratelimit"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

func TestPing_SourceIPIsResolvedLikeTheRateLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := heartbeat.NewService(heartbeat.Deps{
		Store:          store.NewHeartbeatStore(storetest.Open(t, logger)),
		Logger:         logger,
		LicenseChecker: &heartbeat.DefaultLicenseChecker{MaxHeartbeats: -1},
	})
	resolver := ratelimit.NewClientIPResolver([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})

	ph := NewPingHandler(svc, resolver)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping/{uuid}/start", ph.HandleStartPing)
	mux.HandleFunc("GET /ping/{uuid}/{exit_code}", ph.HandleExitCodePing)
	mux.HandleFunc("GET /ping/{uuid}", ph.HandlePing)

	cases := []struct {
		name, peer, want string
	}{
		{"untrusted peer cannot claim an address", "203.0.113.9:40000", "203.0.113.9"},
		{"trusted proxy forwards the client", "10.0.0.2:40000", "198.51.100.7"},
	}
	for _, suffix := range []string{"", "/start", "/0"} {
		for _, tc := range cases {
			t.Run(tc.name+" "+suffix, func(t *testing.T) {
				ctx := context.Background()
				hb, err := svc.CreateHeartbeat(ctx, heartbeat.CreateHeartbeatInput{Name: "job", IntervalSeconds: 300, GraceSeconds: 60}, uid.New())
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodGet, "/ping/"+hb.ID+suffix, nil)
				req.RemoteAddr = tc.peer
				req.Header.Set("X-Forwarded-For", "198.51.100.7")
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

				pings, _, err := svc.ListPings(ctx, hb.ID, heartbeat.ListPingsOpts{Limit: 10})
				require.NoError(t, err)
				require.Len(t, pings, 1)
				assert.Equal(t, tc.want, pings[0].SourceIP)
			})
		}
	}
}
