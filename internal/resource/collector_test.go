// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package resource_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type statsRuntime struct {
	runtime.Runtime

	mu        sync.Mutex
	connected bool
	asked     []string
}

func (r *statsRuntime) IsConnected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.connected
}

func (r *statsRuntime) StatsSnapshot(_ context.Context, externalID string) (*runtime.RawStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked = append(r.asked, externalID)
	return &runtime.RawStats{CPUPercent: 5, MemUsed: 1, MemLimit: 2, Timestamp: time.Now()}, nil
}

func seedRunningContainer(t *testing.T, cstore *store.ContainerStore, extID, agentID string) *container.Container {
	t.Helper()
	now := time.Now()
	c := &container.Container{
		ExternalID: extID, AgentID: agentID, Name: extID, Image: "img:v1",
		State: container.StateRunning, AlertSeverity: container.SeverityWarning,
		RestartThreshold: 3, RuntimeType: "docker", FirstSeenAt: now, LastStateChangeAt: now,
	}
	id, err := cstore.InsertContainer(context.Background(), c)
	require.NoError(t, err)
	c.ID = id
	return c
}

func newCollector(db *store.DB, rt runtime.Runtime) *resource.Collector {
	svc := container.NewService(container.Deps{Store: store.NewContainerStore(db), Logger: slog.Default()})
	return resource.NewCollector(rt, svc, slog.Default())
}

func TestCollector_SamplesOnlyTheLocalRuntimeContainers(t *testing.T) {
	db := storetest.Open(t, nil)
	ctx := context.Background()
	cstore := store.NewContainerStore(db)

	require.NoError(t, store.NewAgentStore(db).Insert(ctx, &agent.Agent{
		AgentID: "agent-remote", PublicKey: make([]byte, 32), Hostname: "h", Label: "edge",
		OSArch: "linux/amd64", AgentVersion: "dev", DetectedRuntime: "docker",
		Status: "active", CreatedAt: time.Now(),
	}))
	seedRunningContainer(t, cstore, "ext-local", "")
	seedRunningContainer(t, cstore, "ext-remote", "agent-remote")

	rt := &statsRuntime{connected: true}
	newCollector(db, rt).Collect(ctx)

	assert.Equal(t, []string{"ext-local"}, rt.asked)
}
