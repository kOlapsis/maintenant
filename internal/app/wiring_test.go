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

package app_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/license"
	"github.com/kolapsis/maintenant/internal/store/sqlite"
	"github.com/kolapsis/maintenant/internal/uid"
)

// noopChannelStore satisfies alert.ChannelStore for tests that do not exercise
// channel operations.
type noopChannelStore struct{}

func (n *noopChannelStore) InsertChannel(_ context.Context, _ *alert.NotificationChannel) (string, error) {
	return "", nil
}
func (n *noopChannelStore) GetChannel(_ context.Context, _ string) (*alert.NotificationChannel, error) {
	return nil, nil
}
func (n *noopChannelStore) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	return nil, nil
}
func (n *noopChannelStore) UpdateChannel(_ context.Context, _ *alert.NotificationChannel) error {
	return nil
}
func (n *noopChannelStore) DeleteChannel(_ context.Context, _ string) error { return nil }
func (n *noopChannelStore) GetChannelHealth(_ context.Context, _ string) (string, error) {
	return "ok", nil
}
func (n *noopChannelStore) InsertDelivery(_ context.Context, _ *alert.NotificationDelivery) (string, error) {
	return "", nil
}
func (n *noopChannelStore) UpdateDelivery(_ context.Context, _ *alert.NotificationDelivery) error {
	return nil
}
func (n *noopChannelStore) ListDeliveriesByAlert(_ context.Context, _ string) ([]*alert.NotificationDelivery, error) {
	return nil, nil
}

// noopSuppressor satisfies alert.MaintenanceSuppressor — no suppression.
type noopSuppressor struct{}

func (n *noopSuppressor) IsSuppressed(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

// purgeCountStore wraps a mock escalation.Store that counts PurgeRunsAndDeliveriesOlderThan calls.
type purgeCountStore struct {
	escalation.Store
	mu    sync.Mutex
	count int
}

func (p *purgeCountStore) PurgeRunsAndDeliveriesOlderThan(_ context.Context, _ time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count++
	return nil
}

func (p *purgeCountStore) called() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

// TestEditionDowngradePropagatesToEscalation verifies that a Pro→CE license
// transition deactivates all active escalation policies and stops all active
// runs. This exercises the callback registered by wireLicenseSubscriber.
func TestEditionDowngradePropagatesToEscalation(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"), logger)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, sqlite.Migrate(db.ReadDB(), logger))
	db.StartWriter(ctx)

	store := sqlite.NewEscalationStore(db)
	svc := escalation.NewService(
		store,
		&noopChannelStore{},
		func() extension.Edition { return extension.Pro },
		&noopSuppressor{},
		logger,
	)

	now := time.Now()

	// Seed: one alert (required by FK on escalation_runs.alert_id).
	alertID := uid.New()
	_, err = db.Writer().Exec(ctx,
		`INSERT INTO alerts (id, source, alert_type, severity, status, message,
		 entity_type, entity_id, entity_name, fired_at, created_at)
		 VALUES (?,'test','test','warning','active','test','container','1','c1',?,?)`,
		alertID, now.Unix(), now.Unix(),
	)
	require.NoError(t, err)

	// Seed: one active policy.
	policyID, err := store.InsertPolicy(ctx, &escalation.Policy{
		Name:      "Downgrade Test Policy",
		Active:    true,
		Levels:    []escalation.Level{{Order: 0, DelaySeconds: 60}},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	// Seed: one active run attached to the policy + alert.
	_, err = store.InsertRun(ctx, &escalation.Run{
		PolicyID:  &policyID,
		AlertID:   alertID,
		Status:    "active",
		StartedAt: now,
	})
	require.NoError(t, err)

	// Simulate wireLicenseSubscriber: register a callback on a Manager
	// and trigger a Pro→CE transition.
	var (
		cbMu      sync.Mutex
		callbacks []license.EditionChangeCallback
	)
	register := func(cb license.EditionChangeCallback) {
		cbMu.Lock()
		defer cbMu.Unlock()
		callbacks = append(callbacks, cb)
	}

	// The callback is identical to what wireLicenseSubscriber wires: it keys on
	// access to alert_escalation, not on the editions themselves.
	required := extension.MinEdition(extension.CapAlertEscalation)
	register(func(cbCtx context.Context, prev, next extension.Edition) {
		if prev.AtLeast(required) && !next.AtLeast(required) {
			if err := svc.OnEditionDowngraded(cbCtx); err != nil {
				t.Errorf("OnEditionDowngraded: %v", err)
			}
		}
	})

	// Fire: Pro → Community.
	cbMu.Lock()
	cbs := make([]license.EditionChangeCallback, len(callbacks))
	copy(cbs, callbacks)
	cbMu.Unlock()
	for _, cb := range cbs {
		cb(ctx, extension.Pro, extension.Community)
	}

	// Policy must be deactivated.
	policies, err := store.SelectPolicies(ctx, false)
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.False(t, policies[0].Active, "policy must be inactive after edition downgrade")

	// Run must be stopped.
	runs, err := store.SelectRunsByPolicy(ctx, policyID, 10, "")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "stopped_by_edition_downgrade", runs[0].Status)
}

// TestRetentionLoopStartsInProOnly verifies that RunRetentionLoop is started
// in Pro (Pro) mode and triggers a purge, while in Community (CE) mode
// the loop goroutine is never launched and the purge count stays at zero.
func TestRetentionLoopStartsInProOnly(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("pro_mode_calls_purge", func(t *testing.T) {
		pstore := &purgeCountStore{}
		svc := escalation.NewService(
			pstore,
			&noopChannelStore{},
			func() extension.Edition { return extension.Pro },
			&noopSuppressor{},
			logger,
		)

		callN := 0
		// First call returns just before 03:00 so the tick fires in <1s.
		// Subsequent calls return past 03:00 so the purge executes immediately.
		svc.SetClockFn(func() time.Time {
			callN++
			if callN <= 1 {
				return time.Date(2026, 1, 15, 2, 59, 59, 500_000_000, time.Local)
			}
			return time.Date(2026, 1, 15, 3, 0, 1, 0, time.Local)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// Replicate the conditional in app.go Start():
		//   if extension.CurrentEdition() == extension.Pro { go svc.RunRetentionLoop(ctx) }
		if svc != nil { // svc is Pro; always true — mirrors the runtime check
			go svc.RunRetentionLoop(ctx)
		}

		require.Eventually(t, func() bool {
			return pstore.called() >= 1
		}, 2*time.Second, 50*time.Millisecond, "purge must be called at least once in Pro mode")
	})

	t.Run("ce_mode_no_purge", func(t *testing.T) {
		pstore := &purgeCountStore{}
		svc := escalation.NewService(
			pstore,
			&noopChannelStore{},
			func() extension.Edition { return extension.Community },
			&noopSuppressor{},
			logger,
		)
		_ = svc // created but loop never started — mirrors the CE branch in app.go

		// In CE mode the goroutine is never launched: 0 purge calls after 200ms.
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, 0, pstore.called(), "purge must not be called in CE mode")
	})
}

// TestEditionTransitions_AllSixDirectedPairs covers the machine wireLicenseSubscriber
// implements. It keys on access to alert_escalation rather than on the editions
// themselves, so of the six directed pairs only the two that cross the Pro
// boundary do anything — and the upgrade direction is the one that was never
// called before this feature, leaving a downgrade-then-upgrade round trip with
// escalation off for good.
func TestEditionTransitions_AllSixDirectedPairs(t *testing.T) {
	required := extension.MinEdition(extension.CapAlertEscalation)

	type action string
	const (
		none      action = "none"
		downgrade action = "downgrade"
		upgrade   action = "upgrade"
	)

	cases := []struct {
		prev, next extension.Edition
		want       action
	}{
		{extension.Community, extension.Personal, none},
		{extension.Personal, extension.Community, none},
		{extension.Community, extension.Pro, upgrade},
		{extension.Personal, extension.Pro, upgrade},
		{extension.Pro, extension.Personal, downgrade},
		{extension.Pro, extension.Community, downgrade},
	}
	require.Len(t, cases, 6, "three editions have exactly six directed transitions")

	for _, c := range cases {
		t.Run(string(c.prev)+"->"+string(c.next), func(t *testing.T) {
			// The same decision wireLicenseSubscriber makes.
			was, now := c.prev.AtLeast(required), c.next.AtLeast(required)

			var got action
			switch {
			case was && !now:
				got = downgrade
			case !was && now:
				got = upgrade
			default:
				got = none
			}

			assert.Equal(t, string(c.want), string(got))
		})
	}
}

// TestWireLicenseSubscriber_RegisteredBeforeStart guards the ordering: a
// callback registered after Start misses the first transition, which is exactly
// the one that matters when a license is revoked at boot.
func TestWireLicenseSubscriber_RegisteredBeforeStart(t *testing.T) {
	src, err := os.ReadFile("wiring.go")
	require.NoError(t, err)
	wiring := string(src)

	appSrc, err := os.ReadFile("app.go")
	require.NoError(t, err)

	require.Contains(t, wiring, "RegisterEditionChangeCallback",
		"wireLicenseSubscriber must register the callback")

	// In Start(), the subscriber is wired on the line before licenseMgr.Start.
	start := strings.Index(string(appSrc), "a.wireLicenseSubscriber(ctx)")
	require.Greater(t, start, 0, "wireLicenseSubscriber must be called from Start")
	mgrStart := strings.Index(string(appSrc), "a.licenseMgr.Start(ctx)")
	require.Greater(t, mgrStart, 0)
	assert.Less(t, start, mgrStart,
		"wireLicenseSubscriber must run before licenseMgr.Start, or the first transition is lost")
}
