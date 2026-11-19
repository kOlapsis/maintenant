// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

type missedAlertEnv struct {
	ctx    context.Context
	store  *store.EndpointStore
	svc    *endpoint.Service
	alerts int
	id     string
}

func newMissedAlertEnv(t *testing.T) *missedAlertEnv {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	e := &missedAlertEnv{ctx: context.Background(), store: store.NewEndpointStore(storetest.Open(t, logger))}
	e.svc = endpoint.NewService(endpoint.Deps{
		Store:  e.store,
		Engine: endpoint.NewCheckEngine(nil, logger),
		Logger: logger,
	})
	detector := alert.NewEndpointAlertDetector()
	e.svc.SetAlertCallback(func(ep *endpoint.Endpoint, res endpoint.CheckResult) (string, any) {
		al := detector.EvaluateCheckResult(ep, res)
		if al == nil {
			return "", nil
		}
		if al.Type == "alert" {
			e.alerts++
			return "endpoint.alert", nil
		}
		return "endpoint.recovery", nil
	})
	e.id = uid.EndpointLabel(uid.LocalAgent, "web", "maintenant.endpoint.tcp")
	return e
}

func (e *missedAlertEnv) sync(externalID string) {
	e.svc.SyncEndpoints(e.ctx, "web", externalID, map[string]string{"maintenant.endpoint.tcp": "127.0.0.1:1"})
}

func (e *missedAlertEnv) fail(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		e.svc.ProcessCheckResult(e.ctx, e.id, endpoint.CheckResult{
			EndpointID: e.id, Success: false, ErrorMessage: "refused", Timestamp: time.Now(),
		})
	}
}

func TestMissedAlert_RecreatedContainerAlertsAgain(t *testing.T) {
	e := newMissedAlertEnv(t)

	e.sync("ext-1")
	e.fail(t, 3)
	require.Equal(t, 1, e.alerts, "first outage alerts")

	e.svc.HandleContainerDestroy(e.ctx, "ext-1")
	e.sync("ext-2")

	ep, err := e.store.GetEndpointByID(e.ctx, e.id)
	require.NoError(t, err)
	require.Equal(t, endpoint.AlertNormal, ep.AlertState, "the recreated container starts without a stale lock")

	e.fail(t, 3)
	require.Equal(t, 2, e.alerts, "the recreated container still down alerts again")
}

func TestMissedAlert_EndpointFreedFromTheCapAlertsAgain(t *testing.T) {
	e := newMissedAlertEnv(t)

	e.sync("ext-1")
	e.fail(t, 3)
	require.Equal(t, 1, e.alerts)

	require.NoError(t, e.store.SetEndpointLimited(e.ctx, e.id, true))
	require.NoError(t, e.store.SetEndpointLimited(e.ctx, e.id, false))

	e.fail(t, 3)
	require.Equal(t, 2, e.alerts, "the freed endpoint still down alerts again")
}
