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

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/swarm"
)

func seedTestAgent(t *testing.T, db *DB, id, runtime string) {
	t.Helper()
	require.NoError(t, NewAgentStore(db).Insert(context.Background(), &agent.Agent{
		AgentID:         id,
		PublicKey:       []byte("pubkeyplaceholder12345678901234"),
		Hostname:        "host-" + id,
		Label:           id,
		OSArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: runtime,
		Status:          "active",
		CreatedAt:       time.Now().UTC(),
	}))
}

func TestSwarmTopologyStore_ReplaceServices_ReconcilesAndPrunes(t *testing.T) {
	db := openTestDB(t)
	store := NewSwarmTopologyStore(db)
	ctx := context.Background()
	seedTestAgent(t, db, "agent-A", "swarm")
	seedTestAgent(t, db, "agent-B", "swarm")

	// First snapshot: two services for A, one for B.
	require.NoError(t, store.ReplaceServicesForAgent(ctx, "agent-A", []swarm.SwarmService{
		{ServiceID: "svc1", Name: "web", Image: "nginx", Mode: "replicated", DesiredReplicas: 3, RunningReplicas: 3, StackName: "stk"},
		{ServiceID: "svc2", Name: "api", Image: "go", Mode: "replicated", DesiredReplicas: 1, RunningReplicas: 0},
	}))
	require.NoError(t, store.ReplaceServicesForAgent(ctx, "agent-B", []swarm.SwarmService{
		{ServiceID: "svcB", Name: "db", Image: "pg", Mode: "global"},
	}))

	a, err := store.ListServices(ctx, "agent-A")
	require.NoError(t, err)
	require.Len(t, a, 2)

	b, err := store.ListServices(ctx, "agent-B")
	require.NoError(t, err)
	require.Len(t, b, 1)
	require.Equal(t, "db", b[0].Name)

	// Second snapshot for A drops svc2 — it must be hard-deleted, B untouched.
	require.NoError(t, store.ReplaceServicesForAgent(ctx, "agent-A", []swarm.SwarmService{
		{ServiceID: "svc1", Name: "web-renamed", Image: "nginx:2", Mode: "replicated", DesiredReplicas: 5, RunningReplicas: 5},
	}))

	a, err = store.ListServices(ctx, "agent-A")
	require.NoError(t, err)
	require.Len(t, a, 1)
	require.Equal(t, "web-renamed", a[0].Name)
	require.Equal(t, 5, a[0].DesiredReplicas)

	b, err = store.ListServices(ctx, "agent-B")
	require.NoError(t, err)
	require.Len(t, b, 1, "agent-B services must be untouched by agent-A reconcile")

	// Empty snapshot wipes the agent entirely.
	require.NoError(t, store.ReplaceServicesForAgent(ctx, "agent-A", nil))
	a, err = store.ListServices(ctx, "agent-A")
	require.NoError(t, err)
	require.Empty(t, a)

	// Cross-agent listing (empty agentID) returns both remaining (only B now).
	all, err := store.ListServices(ctx, "")
	require.NoError(t, err)
	require.Len(t, all, 1)
}

func TestSwarmTopologyStore_ReplaceTasks_ExitCodeNullable(t *testing.T) {
	db := openTestDB(t)
	store := NewSwarmTopologyStore(db)
	ctx := context.Background()
	seedTestAgent(t, db, "agent-A", "swarm")

	exit := 137
	require.NoError(t, store.ReplaceTasksForAgent(ctx, "agent-A", []swarm.SwarmTask{
		{TaskID: "t1", ServiceID: "svc1", NodeID: "n1", Slot: 1, State: "running", DesiredState: "running"},
		{TaskID: "t2", ServiceID: "svc1", NodeID: "n1", Slot: 2, State: "failed", DesiredState: "shutdown", ExitCode: &exit, Error: "oom"},
	}))

	tasks, err := store.ListTasks(ctx, "agent-A", "svc1")
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	require.Nil(t, tasks[0].ExitCode, "running task has no exit code")
	require.NotNil(t, tasks[1].ExitCode)
	require.Equal(t, 137, *tasks[1].ExitCode)
	require.Equal(t, "oom", tasks[1].Error)

	// Service filter isolates results.
	none, err := store.ListTasks(ctx, "agent-A", "other-svc")
	require.NoError(t, err)
	require.Empty(t, none)
}
