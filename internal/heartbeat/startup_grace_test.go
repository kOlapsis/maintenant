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

package heartbeat

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newServiceStartedAt(store *mockStore, startedAt time.Time) *Service {
	return NewService(Deps{
		Store:          store,
		Logger:         slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		LicenseChecker: &mockLicense{canCreate: true},
		StartedAt:      startedAt,
	})
}

func TestService_checkDeadlines_StartupGrace(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		startedAt   time.Time
		deadline    time.Time
		expectAlert bool
	}{
		{
			name:        "deadline missed just before startup is not alerted",
			startedAt:   now,
			deadline:    now.Add(-30 * time.Second),
			expectAlert: false,
		},
		{
			name:        "deadline missed just after startup is not alerted",
			startedAt:   now.Add(-10 * time.Second),
			deadline:    now.Add(-1 * time.Second),
			expectAlert: false,
		},
		{
			name:        "monitor already down well before startup is alerted",
			startedAt:   now,
			deadline:    now.Add(-5 * time.Minute),
			expectAlert: true,
		},
		{
			name:        "still silent once the grace window closes, alerted as usual",
			startedAt:   now.Add(-3 * time.Minute),
			deadline:    now.Add(-1 * time.Second),
			expectAlert: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			svc := newServiceStartedAt(store, tc.startedAt)

			var alertCalls []string
			svc.SetAlertCallback(func(_ *Heartbeat, alertType string, _ map[string]interface{}) {
				alertCalls = append(alertCalls, alertType)
			})

			h := seedHeartbeat(store, "uuid-grace", StatusUp, AlertNormal)
			deadline := tc.deadline
			store.heartbeats[h.ID].NextDeadlineAt = &deadline
			store.overdue = []*Heartbeat{store.heartbeats[h.ID]}

			svc.checkDeadlines(context.Background())

			updated := store.heartbeats[h.ID]
			if tc.expectAlert {
				require.Len(t, alertCalls, 1)
				assert.Equal(t, "alert", alertCalls[0])
				assert.Equal(t, StatusDown, updated.Status)
				assert.Equal(t, AlertAlerting, updated.AlertState)
				return
			}
			assert.Empty(t, alertCalls)
			assert.Equal(t, StatusUp, updated.Status)
			assert.Equal(t, AlertNormal, updated.AlertState)
			assert.Equal(t, 0, updated.ConsecutiveFailures)
		})
	}
}

func TestService_withinStartupGrace_NilDeadline(t *testing.T) {
	svc := newServiceStartedAt(newMockStore(), time.Now())
	assert.False(t, svc.withinStartupGrace(time.Now(), nil))
}

func TestService_startupMarkDefaultsToNow(t *testing.T) {
	svc := newService(newMockStore(), &mockLicense{canCreate: true})
	deadline := time.Now().Add(-time.Second)
	assert.True(t, svc.withinStartupGrace(time.Now(), &deadline))
}
