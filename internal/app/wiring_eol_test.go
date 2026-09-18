// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

// TestOSEOLAlertsThroughTheEngine drives the real alert engine with the events
// the end-of-support service emits: a host whose support window closes keeps a
// single alert that escalates in place, and its upgrade resolves it.
func TestOSEOLAlertsThroughTheEngine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	alertStore := store.NewAlertStore(db)
	agentStore := store.NewAgentStore(db)
	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: store.NewChannelStore(db),
		TriggerStore: store.NewTriggerStore(db),
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
	})
	engine.Start(ctx)

	a := &App{
		alertStore:  alertStore,
		agentStore:  agentStore,
		alertEngine: engine,
		statusSvc:   status.NewService(status.Deps{Components: store.NewStatusComponentStore(db), Logger: logger}),
		logger:      logger,
	}

	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: "web-03", Hostname: "web-03", Label: "web-03", Status: "active",
		DetectedRuntime: "docker", CreatedAt: time.Now(),
	}))

	today := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc, err := eol.New(eol.Deps{
		Store:  agentStore,
		Emit:   a.emitAlert,
		Now:    func() time.Time { return today },
		Logger: logger,
	})
	require.NoError(t, err)

	setOS := func(id, versionID string) {
		_, err := agentStore.UpdateAgentOS(ctx, "web-03", agent.OSIdentity{
			ID: id, VersionID: versionID, PrettyName: "Debian GNU/Linux " + versionID, Source: "host_file",
		}, today)
		require.NoError(t, err)
	}
	hostAlerts := func() []*alert.Alert {
		active, err := alertStore.ListActiveAlerts(ctx)
		require.NoError(t, err)
		var out []*alert.Alert
		for _, al := range active {
			if al.Source == alert.SourceHost && al.AlertType == eol.AlertType {
				out = append(out, al)
			}
		}
		return out
	}
	waitForSeverity := func(severity string) {
		require.Eventually(t, func() bool {
			got := hostAlerts()
			return len(got) == 1 && got[0].Severity == severity && got[0].EntityID == "web-03"
		}, 5*time.Second, 10*time.Millisecond, "one active host/os_eol alert in "+severity+" expected")
	}

	setOS("debian", "11")
	require.NoError(t, svc.EvaluateAll(ctx))
	waitForSeverity(alert.SeverityWarning)

	today = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, svc.EvaluateAll(ctx))
	waitForSeverity(alert.SeverityCritical)

	setOS("debian", "12")
	require.NoError(t, svc.EvaluateAll(ctx))
	require.Eventually(t, func() bool {
		return len(hostAlerts()) == 0
	}, 5*time.Second, 10*time.Millisecond, "the upgrade must resolve the alert")

	// A recovery with nothing active must not open anything.
	require.NoError(t, svc.EvaluateAll(ctx))
	time.Sleep(200 * time.Millisecond)
	assert.Empty(t, hostAlerts())
}
