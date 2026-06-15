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

package alert_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/store/sqlite"
)

// syncBuffer is a goroutine-safe sink for the engine's structured logs.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestEngine_DedupKeyCollision_LogsAndRefreshes is the guardrail against the
// "keycloak message on bitwarden entity" class of bug: two distinct subjects
// sharing one dedup key (e.g. both emitted with a blank entity_id) must (1) be
// logged loudly as a collision and (2) never produce a Frankenstein record —
// an escalation refreshes the whole record to the latest event.
func TestEngine_DedupKeyCollision_LogsAndRefreshes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "eng.db"), logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, sqlite.Migrate(db.ReadDB(), logger))
	db.StartWriter(ctx)

	alertStore := sqlite.NewAlertStore(db)
	eng := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: sqlite.NewChannelStore(db),
		TriggerStore: sqlite.NewTriggerStore(db),
		SilenceStore: sqlite.NewSilenceStore(db),
		Logger:       logger,
	})
	eng.Start(ctx)

	send := func(name, severity string) {
		eng.EventChannel() <- alert.Event{
			Source:     "update",
			AlertType:  "update_available",
			Severity:   severity,
			EntityType: "container",
			EntityID:   "", // both events share the (blank) id → same dedup key
			EntityName: name,
			Message:    "Update available for " + name,
			Timestamp:  time.Now(),
		}
		time.Sleep(150 * time.Millisecond)
	}

	send("bitwarden-postgres", alert.SeverityWarning) // creates the alert
	send("keycloak", alert.SeverityCritical)          // same key, higher severity → escalates

	// 1. The collision must be logged loudly (never silent).
	assert.Contains(t, logs.String(), "collision", "a dedup key collision must be logged")

	// 2. The record must be fully refreshed to the latest event — not a mix of
	//    keycloak's message with bitwarden's entity.
	active, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1)
	got := active[0]
	assert.Equal(t, alert.SeverityCritical, got.Severity)
	assert.Equal(t, "keycloak", got.EntityName, "entity name must follow the escalating event")
	assert.Equal(t, "Update available for keycloak", got.Message)
}
