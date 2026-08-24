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
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

// engineTestSetup opens a fresh temp DB, runs all migrations, starts the writer,
// and returns stores + a started Engine. The engine has no notifier, so
// dispatchNotifications is a no-op — but we can verify trigger matching via
// the delivery rows created by enqueueDelivery when a notifier IS provided.
//
// For dispatch tests we build the engine WITH a notifier so that deliveries
// are recorded in the DB.
func engineTestSetup(t *testing.T) (
	ctx context.Context,
	cancel context.CancelFunc,
	db *store.DB,
	alertStore alert.AlertStore,
	channelStore alert.ChannelStore,
	triggerStore alert.TriggerStore,
	notifier *alert.Notifier,
	eng *alert.Engine,
) {
	t.Helper()
	ctx, cancel = context.WithCancel(context.Background())

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db = storetest.Open(t, logger)

	alertStore = store.NewAlertStore(db)
	channelStore = store.NewChannelStore(db)
	triggerStore = store.NewTriggerStore(db)
	silenceStore := store.NewSilenceStore(db)
	notifier = alert.NewNotifier(channelStore, logger, true)
	notifier.Start(ctx)

	eng = alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: channelStore,
		TriggerStore: triggerStore,
		SilenceStore: silenceStore,
		Notifier:     notifier,
		Logger:       logger,
	})
	eng.Start(ctx)
	return
}

func seedWebhookChannel(t *testing.T, cs alert.ChannelStore, name string, enabled bool) string {
	t.Helper()
	ch := &alert.NotificationChannel{
		Name:    name,
		Type:    "webhook",
		URL:     "https://example.com/hook",
		Enabled: enabled,
	}
	id, err := cs.InsertChannel(context.Background(), ch)
	require.NoError(t, err)
	return id
}

func seedTriggerForChannel(t *testing.T, ts alert.TriggerStore, name string, enabled bool, severities, sources string, channelIDs []string) string {
	t.Helper()
	trig := &alert.AlertTrigger{
		Name:             name,
		FilterSeverities: severities,
		FilterSources:    sources,
		Enabled:          enabled,
		NotifyOnResolve:  true,
		ChannelIDs:       channelIDs,
	}
	id, err := ts.InsertTrigger(context.Background(), trig)
	require.NoError(t, err)
	return id
}

func countDeliveriesForChannel(t *testing.T, cs alert.ChannelStore, alertID, channelID string) int {
	t.Helper()
	deliveries, err := cs.ListDeliveriesByAlert(context.Background(), alertID)
	require.NoError(t, err)
	count := 0
	for _, d := range deliveries {
		if d.ChannelID == channelID {
			count++
		}
	}
	return count
}

func fireEvent(eng *alert.Engine, severity, source, entityType string, entityID string) {
	eng.EventChannel() <- alert.Event{
		Source:     source,
		AlertType:  "test",
		Severity:   severity,
		EntityType: entityType,
		EntityID:   entityID,
		EntityName: "test-entity",
		Timestamp:  time.Now(),
	}
	time.Sleep(150 * time.Millisecond)
}

// capturingWebhookServer records every JSON payload POSTed to it.
func capturingWebhookServer(t *testing.T) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		bodies = append(bodies, payload)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		out := make([]map[string]any, len(bodies))
		copy(out, bodies)
		return out
	}
}

// TestEngineRecovery_NotificationReadsAsRecovery pins FIX 1: a recovery must
// route by the original alert (so a trigger filtered on the original severity
// still matches) but the notified payload must read as the recovery, not repeat
// the original failure's message/severity.
func TestEngineRecovery_NotificationReadsAsRecovery(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	defer cancel()
	_ = db

	srv, capturedBodies := capturingWebhookServer(t)
	chID, err := channelStore.InsertChannel(ctx, &alert.NotificationChannel{
		Name: "hb-channel", Type: "webhook", URL: srv.URL, Enabled: true,
	})
	require.NoError(t, err)
	seedTriggerForChannel(t, triggerStore, "CriticalHeartbeats", true, "critical", "heartbeat", []string{chID})

	eng.EventChannel() <- alert.Event{
		Source:     "heartbeat",
		AlertType:  "deadline_missed",
		Severity:   "critical",
		EntityType: "heartbeat",
		EntityID:   "hb-1",
		EntityName: "backup",
		Message:    "Heartbeat 'backup' missed deadline",
		Timestamp:  time.Now(),
	}
	time.Sleep(150 * time.Millisecond)

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	originalID := alerts[0].ID
	assert.Equal(t, 1, countDeliveriesForChannel(t, channelStore, originalID, chID), "initial critical alert must be delivered")

	eng.EventChannel() <- alert.Event{
		Source:     "heartbeat",
		AlertType:  "deadline_missed",
		Severity:   "info",
		IsRecover:  true,
		EntityType: "heartbeat",
		EntityID:   "hb-1",
		EntityName: "backup",
		Message:    "Heartbeat 'backup' recovered",
		Timestamp:  time.Now(),
	}
	time.Sleep(150 * time.Millisecond)

	active, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	assert.Empty(t, active, "original alert must be resolved")

	resolved, err := alertStore.GetAlert(ctx, originalID)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, alert.StatusResolved, resolved.Status)
	require.NotNil(t, resolved.ResolvedAt)
	require.NotNil(t, resolved.ResolvedByID, "resolved_by_id must point at the recovery record")

	// The trigger matches on the ORIGINAL severity (critical), so the recovery
	// must still be delivered to the same channel — but with the recovery payload.
	assert.Equal(t, 2, countDeliveriesForChannel(t, channelStore, originalID, chID), "recovery must still be delivered, trigger matched on the original severity")

	bodies := capturedBodies()
	require.Len(t, bodies, 2, "webhook must have received both the failure and the recovery")
	recoveryPayload, ok := bodies[1]["alert"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, originalID, recoveryPayload["id"], "recovery payload must carry the original alert's ID")
	assert.Equal(t, alert.StatusResolved, recoveryPayload["status"])
	assert.Equal(t, "Heartbeat 'backup' recovered", recoveryPayload["message"], "must read as a recovery, not repeat the failure message")
	assert.Equal(t, alert.SeverityInfo, recoveryPayload["severity"], "must carry the recovery severity, not the original critical severity")
	assert.NotEmpty(t, recoveryPayload["resolved_at"])
}

func recoverEvent(eng *alert.Engine, severity, source, entityType string, entityID string) {
	eng.EventChannel() <- alert.Event{
		Source:     source,
		AlertType:  "test",
		Severity:   severity,
		IsRecover:  true,
		EntityType: entityType,
		EntityID:   entityID,
		EntityName: "test-entity",
		Timestamp:  time.Now(),
	}
	time.Sleep(150 * time.Millisecond)
}

func seedTriggerNotifyOnResolve(t *testing.T, ts alert.TriggerStore, name string, notifyOnResolve bool, severities, sources string, channelIDs []string) string {
	t.Helper()
	trig := &alert.AlertTrigger{
		Name:             name,
		FilterSeverities: severities,
		FilterSources:    sources,
		Enabled:          true,
		NotifyOnResolve:  notifyOnResolve,
		ChannelIDs:       channelIDs,
	}
	id, err := ts.InsertTrigger(context.Background(), trig)
	require.NoError(t, err)
	return id
}

// T077 — E2E test 1: trigger matches → delivery created for each channel.
func TestEngineDispatch_TriggerMatch_DeliveriesCreated(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	defer cancel()
	_ = db

	ch1 := seedWebhookChannel(t, channelStore, "slack-1", true)
	ch2 := seedWebhookChannel(t, channelStore, "slack-2", true)
	seedTriggerForChannel(t, triggerStore, "CritAll", true, "critical", "container", []string{ch1, ch2})

	fireEvent(eng, "critical", "container", "container", "10")

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	alertID := alerts[0].ID
	assert.Equal(t, 1, countDeliveriesForChannel(t, channelStore, alertID, ch1), "ch1 should have 1 delivery")
	assert.Equal(t, 1, countDeliveriesForChannel(t, channelStore, alertID, ch2), "ch2 should have 1 delivery")
}

// T077 — E2E test 2: disabled channel → no delivery.
func TestEngineDispatch_DisabledChannel_NoDelivery(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	defer cancel()
	_ = db

	chID := seedWebhookChannel(t, channelStore, "disabled-ch", false)
	seedTriggerForChannel(t, triggerStore, "TrigDisCh", true, "critical", "endpoint", []string{chID})

	fireEvent(eng, "critical", "endpoint", "endpoint", "20")

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	assert.Equal(t, 0, countDeliveriesForChannel(t, channelStore, alerts[0].ID, chID), "disabled channel must not receive delivery")
}

// T077 — E2E test 3: disabled trigger → no delivery.
func TestEngineDispatch_DisabledTrigger_NoDelivery(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	defer cancel()
	_ = db

	chID := seedWebhookChannel(t, channelStore, "ch-dis-trig", true)
	seedTriggerForChannel(t, triggerStore, "DisabledTrigger", false, "", "", []string{chID})

	fireEvent(eng, "warning", "container", "container", "30")

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	assert.Equal(t, 0, countDeliveriesForChannel(t, channelStore, alerts[0].ID, chID), "disabled trigger must not produce delivery")
}

// T077 — E2E test 4: two triggers pointing same channel → single delivery (dedup).
func TestEngineDispatch_TwoTriggersOneChannel_Dedup(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	defer cancel()
	_ = db

	chID := seedWebhookChannel(t, channelStore, "dedup-ch", true)
	seedTriggerForChannel(t, triggerStore, "Trigger-A", true, "critical", "", []string{chID})
	seedTriggerForChannel(t, triggerStore, "Trigger-B", true, "critical", "", []string{chID})

	fireEvent(eng, "critical", "heartbeat", "heartbeat", "40")

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	assert.Equal(t, 1, countDeliveriesForChannel(t, channelStore, alerts[0].ID, chID), "channel must receive exactly 1 delivery even with two matching triggers")
}

// T077 — E2E test 5: alert doesn't match any trigger → no delivery.
func TestEngineDispatch_NoMatchingTrigger_NoDelivery(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	defer cancel()
	_ = db

	chID := seedWebhookChannel(t, channelStore, "no-match-ch", true)
	// Trigger only fires on "critical"; we fire "warning" → no match.
	seedTriggerForChannel(t, triggerStore, "CritOnly", true, "critical", "", []string{chID})

	fireEvent(eng, "warning", "container", "container", "50")

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	assert.Equal(t, 0, countDeliveriesForChannel(t, channelStore, alerts[0].ID, chID), "no delivery expected when alert doesn't match trigger filter")
}

// T077 — E2E test 6: source filter narrows correctly.
func TestEngineDispatch_SourceFilter_OnlyMatchesCorrectSource(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	defer cancel()
	_ = db

	chContainer := seedWebhookChannel(t, channelStore, "ch-container", true)
	chEndpoint := seedWebhookChannel(t, channelStore, "ch-endpoint", true)

	seedTriggerForChannel(t, triggerStore, "ContainerOnly", true, "", "container", []string{chContainer})
	seedTriggerForChannel(t, triggerStore, "EndpointOnly", true, "", "endpoint", []string{chEndpoint})

	// Fire a container alert.
	fireEvent(eng, "critical", "container", "container", "60")

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	assert.Equal(t, 1, countDeliveriesForChannel(t, channelStore, alerts[0].ID, chContainer), "container trigger must deliver to container channel")
	assert.Equal(t, 0, countDeliveriesForChannel(t, channelStore, alerts[0].ID, chEndpoint), "endpoint trigger must not fire for container alert")
}

// T079 — Reserved-escalation channel: channel with no trigger → 0 initial deliveries.
// (The escalation policy would deliver it later — tested here at engine dispatch level.)
func TestEngineDispatch_ReservedEscalationChannel_NoInitialDelivery(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	defer cancel()
	_ = db

	// "email-cto" is reserved for escalation only: no trigger references it.
	emailCTO := seedWebhookChannel(t, channelStore, "email-cto", true)

	// A different channel receives initial alerts via a trigger.
	slackOps := seedWebhookChannel(t, channelStore, "slack-ops", true)
	seedTriggerForChannel(t, triggerStore, "AllAlerts", true, "", "", []string{slackOps})

	fireEvent(eng, "critical", "container", "container", "70")

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	assert.Equal(t, 1, countDeliveriesForChannel(t, channelStore, alerts[0].ID, slackOps), "slack-ops should receive initial delivery")
	assert.Equal(t, 0, countDeliveriesForChannel(t, channelStore, alerts[0].ID, emailCTO), "email-cto must not receive initial delivery — it is reserved for escalation only")

	// Verify triggerStore confirms no trigger links email-cto.
	triggersForCTO, err := triggerStore.ListTriggersForChannel(ctx, emailCTO)
	require.NoError(t, err)
	assert.Empty(t, triggersForCTO, "email-cto must have zero triggers (reserved-escalation pattern)")
}

// NotifyOnResolve=false: fire delivers, recovery does not.
func TestEngineDispatch_NotifyOnResolveFalse_NoRecoveryDelivery(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	_ = db
	defer cancel()

	chID := seedWebhookChannel(t, channelStore, "fires-only-ch", true)
	seedTriggerNotifyOnResolve(t, triggerStore, "FiresOnly", false, "critical", "container", []string{chID})

	fireEvent(eng, "critical", "container", "container", "80")

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	alertID := alerts[0].ID
	assert.Equal(t, 1, countDeliveriesForChannel(t, channelStore, alertID, chID), "trigger must deliver the fire")

	recoverEvent(eng, "critical", "container", "container", "80")

	assert.Equal(t, 1, countDeliveriesForChannel(t, channelStore, alertID, chID), "trigger with notify_on_resolve=false must not deliver the recovery")
}

// NotifyOnResolve=true (default): both fire and recovery deliver.
func TestEngineDispatch_NotifyOnResolveTrue_RecoveryDelivered(t *testing.T) {
	ctx, cancel, db, alertStore, channelStore, triggerStore, _, eng := engineTestSetup(t)
	_ = db
	defer cancel()

	chID := seedWebhookChannel(t, channelStore, "fires-and-recovers-ch", true)
	seedTriggerNotifyOnResolve(t, triggerStore, "FiresAndRecovers", true, "critical", "container", []string{chID})

	fireEvent(eng, "critical", "container", "container", "90")

	alerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	alertID := alerts[0].ID
	assert.Equal(t, 1, countDeliveriesForChannel(t, channelStore, alertID, chID), "trigger must deliver the fire")

	recoverEvent(eng, "critical", "container", "container", "90")

	assert.Equal(t, 2, countDeliveriesForChannel(t, channelStore, alertID, chID), "trigger with notify_on_resolve=true must also deliver the recovery")
}
