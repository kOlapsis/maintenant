// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/swarm"
)

// failInsertOf makes every later INSERT on table raise when column holds 'boom'.
func failInsertOf(t *testing.T, db *DB, table, column string) {
	t.Helper()
	if db.dialect == DialectPostgres {
		t.Skip("failure injection uses a SQLite trigger")
	}
	_, err := db.db.Exec(`CREATE TRIGGER boom_` + table + ` BEFORE INSERT ON ` + table +
		` WHEN NEW.` + column + ` = 'boom' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
}

func TestKubernetesStore_ReplaceKeepsThePreviousRowsOnFailure(t *testing.T) {
	const agent = "kube-A"
	ctx := context.Background()

	t.Run("workloads", func(t *testing.T) {
		db := openTestDB(t)
		s := NewKubernetesStore(db)
		seedTestAgent(t, db, agent, "kubernetes")
		wl := func(name, status string) kubernetes.K8sWorkload {
			return kubernetes.K8sWorkload{ID: "default/Deployment/" + name, Name: name, Namespace: "default", Kind: "Deployment", Status: status}
		}
		require.NoError(t, s.ReplaceWorkloadsForAgent(ctx, agent, []kubernetes.K8sWorkload{wl("kept", "old"), wl("gone", "old")}))
		failInsertOf(t, db, "kubernetes_workloads", "name")

		require.Error(t, s.ReplaceWorkloadsForAgent(ctx, agent, []kubernetes.K8sWorkload{wl("kept", "new"), wl("boom", "new")}))

		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM kubernetes_workloads WHERE status='old'`))
		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM kubernetes_workloads`))
	})

	t.Run("pods", func(t *testing.T) {
		db := openTestDB(t)
		s := NewKubernetesStore(db)
		seedTestAgent(t, db, agent, "kubernetes")
		pod := func(name, status string) kubernetes.K8sPod {
			return kubernetes.K8sPod{Name: name, Namespace: "default", Status: status}
		}
		require.NoError(t, s.ReplacePodsForAgent(ctx, agent, []kubernetes.K8sPod{pod("kept", "old"), pod("gone", "old")}))
		failInsertOf(t, db, "kubernetes_pods", "name")

		require.Error(t, s.ReplacePodsForAgent(ctx, agent, []kubernetes.K8sPod{pod("kept", "new"), pod("boom", "new")}))

		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM kubernetes_pods WHERE status='old'`))
		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM kubernetes_pods`))
	})

	t.Run("nodes", func(t *testing.T) {
		db := openTestDB(t)
		s := NewKubernetesStore(db)
		seedTestAgent(t, db, agent, "kubernetes")
		node := func(name, status string) kubernetes.K8sNode {
			return kubernetes.K8sNode{Name: name, Status: status}
		}
		require.NoError(t, s.ReplaceNodesForAgent(ctx, agent, []kubernetes.K8sNode{node("kept", "old"), node("gone", "old")}))
		failInsertOf(t, db, "kubernetes_nodes", "name")

		require.Error(t, s.ReplaceNodesForAgent(ctx, agent, []kubernetes.K8sNode{node("kept", "new"), node("boom", "new")}))

		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM kubernetes_nodes WHERE status='old'`))
		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM kubernetes_nodes`))
	})
}

func TestSwarmStores_ReplaceKeepsThePreviousRowsOnFailure(t *testing.T) {
	const agent = "swarm-A"
	ctx := context.Background()
	now := time.Now()

	t.Run("services", func(t *testing.T) {
		db := openTestDB(t)
		s := NewSwarmTopologyStore(db)
		seedTestAgent(t, db, agent, "swarm")
		svc := func(name, image string) swarm.SwarmService {
			return swarm.SwarmService{ServiceID: "id-" + name, Name: name, Image: image, Mode: "replicated", CreatedAt: now}
		}
		require.NoError(t, s.ReplaceServicesForAgent(ctx, agent, []swarm.SwarmService{svc("kept", "old"), svc("gone", "old")}))
		failInsertOf(t, db, "swarm_services", "name")

		require.Error(t, s.ReplaceServicesForAgent(ctx, agent, []swarm.SwarmService{svc("kept", "new"), svc("boom", "new")}))

		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM swarm_services WHERE image='old'`))
		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM swarm_services`))
	})

	t.Run("tasks", func(t *testing.T) {
		db := openTestDB(t)
		s := NewSwarmTopologyStore(db)
		seedTestAgent(t, db, agent, "swarm")
		task := func(id, state string) swarm.SwarmTask {
			return swarm.SwarmTask{TaskID: id, ServiceID: "svc", State: state, DesiredState: "running", Timestamp: now}
		}
		require.NoError(t, s.ReplaceTasksForAgent(ctx, agent, []swarm.SwarmTask{task("kept", "old"), task("gone", "old")}))
		failInsertOf(t, db, "swarm_tasks", "task_id")

		require.Error(t, s.ReplaceTasksForAgent(ctx, agent, []swarm.SwarmTask{task("kept", "new"), task("boom", "new")}))

		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM swarm_tasks WHERE state='old'`))
		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM swarm_tasks`))
	})

	t.Run("nodes", func(t *testing.T) {
		db := openTestDB(t)
		s := NewSwarmNodeStore(db)
		seedTestAgent(t, db, agent, "swarm")
		node := func(hostname, version string) *swarm.SwarmNode {
			return &swarm.SwarmNode{
				NodeID: "id-" + hostname, Hostname: hostname, Role: "manager", Status: "ready", Availability: "active",
				EngineVersion: version, FirstSeenAt: now, LastSeenAt: now, LastStatusChangeAt: now,
			}
		}
		require.NoError(t, s.ReplaceNodesForAgent(ctx, agent, []*swarm.SwarmNode{node("kept", "old"), node("gone", "old")}))
		failInsertOf(t, db, "swarm_nodes", "hostname")

		require.Error(t, s.ReplaceNodesForAgent(ctx, agent, []*swarm.SwarmNode{node("kept", "new"), node("boom", "new")}))

		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM swarm_nodes WHERE engine_version='old'`))
		assert.Equal(t, 2, countAtomicRows(t, db, `SELECT COUNT(*) FROM swarm_nodes`))
	})
}
