// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
)

// Acceptance tests of 046-agent-grpc-hardening, US2 scenario 7, FR-015 and the edge
// case "inventaire exactement à la borne": 10 000 containers and 256 labels pass,
// one more is refused and nothing is written.

func inventoryOf(n int) *agentpb.ContainerInventory {
	inv := &agentpb.ContainerInventory{Complete: true, Containers: make([]*agentpb.ContainerEvent, 0, n)}
	for i := range n {
		inv.Containers = append(inv.Containers, &agentpb.ContainerEvent{
			ContainerId: fmt.Sprintf("c%05d", i), Name: fmt.Sprintf("n%d", i), Image: "app:v1",
			State: agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		})
	}
	return inv
}

func labelsOf(n int) map[string]string {
	labels := make(map[string]string, n)
	for i := range n {
		labels[fmt.Sprintf("k%d", i)] = "v"
	}
	return labels
}

func (m *svcStore) countFor(agentID string) (live, archived int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.byID {
		if c.AgentID != agentID {
			continue
		}
		if c.Archived {
			archived++
		} else {
			live++
		}
	}
	return live, archived
}

func TestHandleAgentInventory_ContainerCountBound(t *testing.T) {
	const agentID = "a1b2c3d4-0000-4000-8000-00000000c0de"
	meta := agentevent.Meta{ObservedAt: time.Now()}

	t.Run("10 001 containers: refused, nothing written, nothing archived", func(t *testing.T) {
		store := newSvcStore()
		svc := newTestService(store)
		known := makeTestContainer("already-there", StateRunning)
		known.AgentID = agentID
		store.seed(known)

		err := svc.HandleAgentInventory(context.Background(), agentID, inventoryOf(10_001), meta)
		require.Error(t, err, "an inventory past the bound is refused")

		live, archived := store.countFor(agentID)
		assert.Equal(t, 1, live, "nothing from the refused inventory is written")
		assert.Equal(t, 0, archived, "a refused inventory archives nothing either")
	})

	t.Run("10 000 containers: accepted", func(t *testing.T) {
		store := newSvcStore()
		svc := newTestService(store)
		require.NoError(t, svc.HandleAgentInventory(context.Background(), agentID, inventoryOf(10_000), meta))
		live, _ := store.countFor(agentID)
		assert.Equal(t, 10_000, live)
	})
}

func TestHandleAgentInventory_LabelCountBound(t *testing.T) {
	const agentID = "a1b2c3d4-0000-4000-8000-0000000labe1"
	meta := agentevent.Meta{ObservedAt: time.Now()}

	t.Run("one container with 257 labels: refused, nothing written", func(t *testing.T) {
		store := newSvcStore()
		svc := newTestService(store)
		inv := inventoryOf(3)
		inv.Containers[1].Labels = labelsOf(257)

		require.Error(t, svc.HandleAgentInventory(context.Background(), agentID, inv, meta))
		live, _ := store.countFor(agentID)
		assert.Equal(t, 0, live, "the whole inventory is refused, not just the offending container")
	})

	t.Run("256 labels on every container: accepted", func(t *testing.T) {
		store := newSvcStore()
		svc := newTestService(store)
		inv := inventoryOf(3)
		for _, c := range inv.Containers {
			c.Labels = labelsOf(256)
		}
		require.NoError(t, svc.HandleAgentInventory(context.Background(), agentID, inv, meta))
		live, _ := store.countFor(agentID)
		assert.Equal(t, 3, live)
	})
}
