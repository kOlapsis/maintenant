// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/uid"
)

// resultStore keeps the check results it is given, so the latest one is the previous scan.
type resultStore struct {
	*mockCertStore
	mu      sync.Mutex
	results []*CertCheckResult
}

func (r *resultStore) InsertCheckResult(_ context.Context, res *CertCheckResult) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *res
	cp.ID = uid.New()
	r.results = append(r.results, &cp)
	return cp.ID, nil
}

func (r *resultStore) GetLatestCheckResult(_ context.Context, _ string) (*CertCheckResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.results) == 0 {
		return nil, nil
	}
	cp := *r.results[len(r.results)-1]
	return &cp, nil
}

func (r *resultStore) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.results)
}

type emittedEvent struct {
	eventType string
	data      map[string]interface{}
}

func newResultService(t *testing.T) (*Service, *resultStore, *CertMonitor, func() []emittedEvent) {
	t.Helper()
	store := &resultStore{mockCertStore: newMockCertStore()}
	var mu sync.Mutex
	var events []emittedEvent
	svc := NewService(Deps{
		Store:  store,
		Logger: noopLogger(),
		EventCallback: func(eventType string, data interface{}) {
			mu.Lock()
			defer mu.Unlock()
			m, _ := data.(map[string]interface{})
			events = append(events, emittedEvent{eventType, m})
		},
	})
	monitor := &CertMonitor{
		ID:                   uid.New(),
		Hostname:             "app.example.com",
		Port:                 443,
		Source:               SourceAuto,
		CheckIntervalSeconds: 43200,
		WarningThresholds:    DefaultWarningThresholds(),
		AgentID:              uid.LocalAgent,
	}
	store.monitors[monitor.ID] = monitor
	drain := func() []emittedEvent {
		mu.Lock()
		defer mu.Unlock()
		out := events
		events = nil
		return out
	}
	return svc, store, monitor, drain
}

func healthyScan() *CheckCertificateResult {
	now := time.Now()
	return &CheckCertificateResult{
		SubjectCN:     "app.example.com",
		IssuerOrg:     "Example CA",
		SerialNumber:  "01",
		NotBefore:     now.Add(-24 * time.Hour),
		NotAfter:      now.Add(90 * 24 * time.Hour),
		ChainValid:    true,
		HostnameMatch: true,
	}
}

func recoveredTypes(events []emittedEvent) []string {
	var out []string
	for _, e := range events {
		if e.eventType == event.CertificateRecovery {
			out = append(out, e.data["previous_alert_type"].(string))
		}
	}
	return out
}

func alertTypes(events []emittedEvent) []string {
	var out []string
	for _, e := range events {
		if e.eventType == event.CertificateAlert {
			out = append(out, e.data["alert_type"].(string))
		}
	}
	return out
}

func TestProcessCheckResult_EachAlertRecoversWhenItsConditionClears(t *testing.T) {
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = prev })

	cases := map[string]struct {
		broken    func(*CheckCertificateResult)
		alertType string
	}{
		"chain":    {func(r *CheckCertificateResult) { r.ChainValid, r.ChainError = false, "unknown authority" }, AlertTypeChainInvalid},
		"hostname": {func(r *CheckCertificateResult) { r.HostnameMatch = false }, AlertTypeHostnameMismatch},
		"ocsp":     {func(r *CheckCertificateResult) { r.OCSPStapled, r.OCSPStatus = true, "revoked" }, AlertTypeOCSPRevoked},
		"expired": {func(r *CheckCertificateResult) {
			r.NotBefore, r.NotAfter = time.Now().Add(-90*24*time.Hour), time.Now().Add(-time.Hour)
		}, AlertTypeExpired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc, _, monitor, drain := newResultService(t)
			ctx := context.Background()

			bad := healthyScan()
			tc.broken(bad)
			svc.processCheckResult(ctx, monitor, bad, true)
			fired := drain()
			assert.Contains(t, alertTypes(fired), tc.alertType)
			for _, e := range fired {
				if e.eventType == event.CertificateAlert {
					assert.Equal(t, monitor.AgentID, e.data["agent_id"], "the alert names the agent that scans the certificate")
				}
			}

			svc.processCheckResult(ctx, monitor, bad, true)
			assert.Empty(t, recoveredTypes(drain()), "a condition still present must not recover")

			good := healthyScan()
			good.SerialNumber = "02"
			svc.processCheckResult(ctx, monitor, good, true)
			assert.Equal(t, []string{tc.alertType}, recoveredTypes(drain()))

			svc.processCheckResult(ctx, monitor, good, true)
			assert.Empty(t, recoveredTypes(drain()), "a healthy certificate must not keep announcing recoveries")
		})
	}
}

func TestProcessCheckResult_ScanAfterAFailureRecoversEveryClearCondition(t *testing.T) {
	svc, _, monitor, drain := newResultService(t)
	ctx := context.Background()

	svc.processCheckResult(ctx, monitor, &CheckCertificateResult{Error: "connection refused"}, false)
	drain()

	svc.processCheckResult(ctx, monitor, healthyScan(), false)
	assert.ElementsMatch(t,
		[]string{AlertTypeChainInvalid, AlertTypeHostnameMismatch, AlertTypeOCSPRevoked, AlertTypeExpired},
		recoveredTypes(drain()))
}

func TestProcessCheckResult_PushedScansAreStoredOncePerIntervalOrOnChange(t *testing.T) {
	svc, store, monitor, drain := newResultService(t)
	ctx := context.Background()

	for range 5 {
		svc.processCheckResult(ctx, monitor, healthyScan(), true)
	}
	assert.Equal(t, 1, store.count(), "identical probes within the interval write one row")

	renewed := healthyScan()
	renewed.SerialNumber = "02"
	renewed.NotAfter = renewed.NotAfter.Add(30 * 24 * time.Hour)
	svc.processCheckResult(ctx, monitor, renewed, true)
	assert.Equal(t, 2, store.count(), "a new certificate is recorded at once")

	store.mu.Lock()
	store.results[len(store.results)-1].CheckedAt = time.Now().Add(-13 * time.Hour)
	store.mu.Unlock()
	svc.processCheckResult(ctx, monitor, renewed, true)
	assert.Equal(t, 3, store.count(), "an unchanged certificate is still recorded once per interval")

	drain()
	chainBroken := healthyScan()
	chainBroken.SerialNumber = "02"
	chainBroken.NotAfter = renewed.NotAfter
	chainBroken.ChainValid = false
	svc.processCheckResult(ctx, monitor, chainBroken, true)
	assert.Equal(t, 4, store.count(), "a change of state is recorded at once")
	assert.Contains(t, alertTypes(drain()), AlertTypeChainInvalid)

	svc.processCheckResult(ctx, monitor, chainBroken, true)
	assert.Equal(t, 4, store.count())
	assert.Contains(t, alertTypes(drain()), AlertTypeChainInvalid, "an unrecorded scan is still evaluated")
}

func TestProcessCheckResult_ScheduledScansAreAlwaysStored(t *testing.T) {
	svc, store, monitor, _ := newResultService(t)
	monitor.Source = SourceStandalone

	for range 3 {
		svc.processCheckResult(context.Background(), monitor, healthyScan(), false)
	}
	require.Equal(t, 3, store.count())
}
