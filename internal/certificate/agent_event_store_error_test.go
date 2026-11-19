// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

var errStoreDown = errors.New("store down")

type flakyCertStore struct {
	*resultStore
	errLookup, errLatest, errInsert, errUpdate, errCreate error
}

func (f *flakyCertStore) GetMonitorByHostPortAgent(ctx context.Context, a *string, host string, port int, sn string) (*CertMonitor, error) {
	if f.errLookup != nil {
		return nil, f.errLookup
	}
	return f.resultStore.GetMonitorByHostPortAgent(ctx, a, host, port, sn)
}

func (f *flakyCertStore) CreateMonitor(ctx context.Context, m *CertMonitor) (string, error) {
	if f.errCreate != nil {
		return "", f.errCreate
	}
	return f.resultStore.CreateMonitor(ctx, m)
}

func (f *flakyCertStore) GetLatestCheckResult(ctx context.Context, id string) (*CertCheckResult, error) {
	if f.errLatest != nil {
		return nil, f.errLatest
	}
	return f.resultStore.GetLatestCheckResult(ctx, id)
}

func (f *flakyCertStore) InsertCheckResult(ctx context.Context, r *CertCheckResult) (string, error) {
	if f.errInsert != nil {
		return "", f.errInsert
	}
	return f.resultStore.InsertCheckResult(ctx, r)
}

func (f *flakyCertStore) UpdateMonitor(ctx context.Context, m *CertMonitor) error {
	if f.errUpdate != nil {
		return f.errUpdate
	}
	return f.resultStore.UpdateMonitor(ctx, m)
}

func agentScan() *agentpb.CertificateInfo {
	now := time.Now().Truncate(time.Second)
	return &agentpb.CertificateInfo{
		Host:      "app.example.com",
		Port:      443,
		SubjectCn: "app.example.com",
		IssuerCn:  "Example CA",
		NotBefore: timestamppb.New(now.Add(-24 * time.Hour)),
		NotAfter:  timestamppb.New(now.Add(90 * 24 * time.Hour)),
	}
}

func flakyAgentService(t *testing.T) (*Service, *flakyCertStore) {
	t.Helper()
	svc, base, monitor, _ := newResultService(t)
	monitor.AgentID = "agent-1"
	store := &flakyCertStore{resultStore: base}
	base.getMonitorByHostPortFn = func(context.Context, string, int, string) (*CertMonitor, error) { return monitor, nil }
	svc.store = store
	return svc, store
}

func TestHandleAgentEvent_StoreFailuresAreReturned(t *testing.T) {
	for name, set := range map[string]func(*flakyCertStore){
		"monitor lookup":      func(f *flakyCertStore) { f.errLookup = errStoreDown },
		"previous scan":       func(f *flakyCertStore) { f.errLatest = errStoreDown },
		"check result insert": func(f *flakyCertStore) { f.errInsert = errStoreDown },
		"monitor update":      func(f *flakyCertStore) { f.errUpdate = errStoreDown },
	} {
		t.Run(name, func(t *testing.T) {
			svc, store := flakyAgentService(t)
			set(store)
			require.ErrorIs(t, svc.HandleAgentEvent(context.Background(), "agent-1", agentScan()), errStoreDown,
				"a scan the store refused must not read as handled")
		})
	}
}

func TestHandleAgentEvent_MonitorCreationFailureIsReturned(t *testing.T) {
	svc, store := flakyAgentService(t)
	store.getMonitorByHostPortFn = nil
	store.errCreate = errStoreDown
	require.ErrorIs(t, svc.HandleAgentEvent(context.Background(), "agent-1", agentScan()), errStoreDown)
}

func TestHandleAgentEvent_ReplayAfterFailureStoresTheScanOnce(t *testing.T) {
	svc, store := flakyAgentService(t)
	scan := agentScan()

	store.errUpdate = errStoreDown
	require.ErrorIs(t, svc.HandleAgentEvent(context.Background(), "agent-1", scan), errStoreDown)
	store.errUpdate = nil

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", scan))
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", scan))

	assert.Equal(t, 1, store.count(), "the same scan replayed leaves one history row")
}
