// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package heartbeat_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestPingReceivedEvent_CarriesTheFieldsOfTheRestRepresentation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		pings    func(svc *heartbeat.Service, token string)
		required []string
	}{
		{"success after a start", func(svc *heartbeat.Service, token string) {
			_, _ = svc.ProcessStartPing(ctx, token, "10.0.0.1", "GET")
			_, _ = svc.ProcessPing(ctx, token, "10.0.0.1", "GET", nil, nil)
		}, []string{"last_ping_at", "next_deadline_at", "last_duration_ms"}},
		{"start after a success", func(svc *heartbeat.Service, token string) {
			_, _ = svc.ProcessPing(ctx, token, "10.0.0.1", "GET", nil, nil)
			_, _ = svc.ProcessStartPing(ctx, token, "10.0.0.1", "GET")
		}, []string{"last_ping_at", "next_deadline_at"}},
		{"exit code after a start", func(svc *heartbeat.Service, token string) {
			_, _ = svc.ProcessStartPing(ctx, token, "10.0.0.1", "GET")
			_, _ = svc.ProcessExitCodePing(ctx, token, 3, "10.0.0.1", "GET", nil)
		}, []string{"last_ping_at", "next_deadline_at", "last_duration_ms", "last_exit_code"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			var last []byte
			svc := heartbeat.NewService(heartbeat.Deps{
				Store:  store.NewHeartbeatStore(storetest.Open(t, logger)),
				Logger: logger,
				EventCallback: func(eventType string, data interface{}) {
					if eventType == event.HeartbeatPingReceived {
						var err error
						last, err = json.Marshal(data)
						require.NoError(t, err)
					}
				},
			})
			created, err := svc.CreateHeartbeat(ctx, heartbeat.CreateHeartbeatInput{Name: "nightly backup", IntervalSeconds: 3600, GraceSeconds: 60}, "tok-"+tc.name)
			require.NoError(t, err)

			tc.pings(svc, created.PingToken)

			got, err := svc.GetHeartbeat(ctx, created.ID)
			require.NoError(t, err)
			var rest, ev map[string]interface{}
			raw, err := json.Marshal(got)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &rest))
			require.NoError(t, json.Unmarshal(last, &ev))

			for _, key := range tc.required {
				assert.Contains(t, ev, key)
			}
			for _, key := range []string{"last_ping_at", "next_deadline_at", "last_duration_ms", "last_exit_code"} {
				want, inRest := rest[key]
				if !inRest {
					assert.NotContains(t, ev, key)
					continue
				}
				if s, ok := want.(string); ok {
					wantAt, err := time.Parse(time.RFC3339, s)
					require.NoError(t, err)
					gotStr, ok := ev[key].(string)
					require.True(t, ok, key)
					gotAt, err := time.Parse(time.RFC3339, gotStr)
					require.NoError(t, err, key)
					assert.Equal(t, wantAt.Unix(), gotAt.Unix(), key)
					continue
				}
				assert.Equal(t, want, ev[key], key)
			}
		})
	}
}
