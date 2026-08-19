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

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/uid"
)

// SwarmTopologyStore persists per-agent swarm services and tasks. Nodes live in
// SwarmNodeStore. All three are reconciled together by the ingest service: each
// snapshot upserts the rows it carries and hard-deletes the agent's rows that
// are absent from it.
type SwarmTopologyStore struct {
	db     *sql.DB
	writer *Writer
}

// NewSwarmTopologyStore creates a SQLite-backed swarm topology store.
func NewSwarmTopologyStore(d *DB) *SwarmTopologyStore {
	return &SwarmTopologyStore{db: d.ReadDB(), writer: d.Writer()}
}

// ReplaceServicesForAgent upserts every service in the snapshot and hard-deletes
// any service previously held for this agent that is no longer present.
func (s *SwarmTopologyStore) ReplaceServicesForAgent(ctx context.Context, agentID string, services []swarm.SwarmService) error {
	agentID = uid.Agent(agentID)
	keep := make([]string, 0, len(services))
	for i := range services {
		svc := &services[i]
		id := uid.SwarmService(agentID, svc.ServiceID)
		keep = append(keep, id)
		labels := svc.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		labelsJSON, err := json.Marshal(labels)
		if err != nil {
			return fmt.Errorf("marshal service labels %s: %w", svc.ServiceID, err)
		}
		_, err = s.writer.Exec(ctx,
			`INSERT INTO swarm_services (id, agent_id, service_id, name, image, mode,
				desired_replicas, running_replicas, labels, stack_name, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(agent_id, service_id) DO UPDATE SET
				name=excluded.name,
				image=excluded.image,
				mode=excluded.mode,
				desired_replicas=excluded.desired_replicas,
				running_replicas=excluded.running_replicas,
				labels=excluded.labels,
				stack_name=excluded.stack_name,
				created_at=excluded.created_at`,
			id, agentID, svc.ServiceID, svc.Name, svc.Image, svc.Mode,
			svc.DesiredReplicas, svc.RunningReplicas, string(labelsJSON), svc.StackName, svc.CreatedAt.Unix(),
		)
		if err != nil {
			return fmt.Errorf("upsert swarm service %s: %w", svc.ServiceID, err)
		}
	}
	return deleteNotInForAgent(ctx, s.writer, "swarm_services", agentID, keep)
}

// ReplaceTasksForAgent upserts every task in the snapshot and hard-deletes any
// task previously held for this agent that is no longer present.
func (s *SwarmTopologyStore) ReplaceTasksForAgent(ctx context.Context, agentID string, tasks []swarm.SwarmTask) error {
	agentID = uid.Agent(agentID)
	keep := make([]string, 0, len(tasks))
	for i := range tasks {
		t := &tasks[i]
		id := uid.SwarmTask(agentID, t.TaskID)
		keep = append(keep, id)
		var exitCode sql.NullInt64
		if t.ExitCode != nil {
			exitCode = sql.NullInt64{Int64: int64(*t.ExitCode), Valid: true}
		}
		_, err := s.writer.Exec(ctx,
			`INSERT INTO swarm_tasks (id, agent_id, task_id, service_id, node_id, slot,
				state, desired_state, container_id, error, exit_code, timestamp, node_hostname)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(agent_id, task_id) DO UPDATE SET
				service_id=excluded.service_id,
				node_id=excluded.node_id,
				slot=excluded.slot,
				state=excluded.state,
				desired_state=excluded.desired_state,
				container_id=excluded.container_id,
				error=excluded.error,
				exit_code=excluded.exit_code,
				timestamp=excluded.timestamp,
				node_hostname=excluded.node_hostname`,
			id, agentID, t.TaskID, t.ServiceID, t.NodeID, t.Slot,
			t.State, t.DesiredState, t.ContainerID, t.Error, exitCode, t.Timestamp.Unix(), t.NodeHostname,
		)
		if err != nil {
			return fmt.Errorf("upsert swarm task %s: %w", t.TaskID, err)
		}
	}
	return deleteNotInForAgent(ctx, s.writer, "swarm_tasks", agentID, keep)
}

// ListServices returns swarm services. When agentID is non-empty only that
// agent's services are returned; otherwise every agent's services are returned.
func (s *SwarmTopologyStore) ListServices(ctx context.Context, agentID string) ([]*swarm.SwarmService, error) {
	q := `SELECT agent_id, service_id, name, image, mode, desired_replicas,
		running_replicas, labels, stack_name, created_at FROM swarm_services`
	var rows *sql.Rows
	var err error
	if agentID != "" {
		q += ` WHERE agent_id=? ORDER BY name ASC`
		rows, err = s.db.QueryContext(ctx, q, uid.Agent(agentID))
	} else {
		q += ` ORDER BY name ASC`
		rows, err = s.db.QueryContext(ctx, q)
	}
	if err != nil {
		return nil, fmt.Errorf("list swarm services: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*swarm.SwarmService
	for rows.Next() {
		var (
			svc        swarm.SwarmService
			labelsJSON string
			createdAt  int64
		)
		if err := rows.Scan(&svc.AgentID, &svc.ServiceID, &svc.Name, &svc.Image, &svc.Mode,
			&svc.DesiredReplicas, &svc.RunningReplicas, &labelsJSON, &svc.StackName, &createdAt); err != nil {
			return nil, fmt.Errorf("scan swarm service: %w", err)
		}
		if labelsJSON != "" && labelsJSON != "{}" {
			_ = json.Unmarshal([]byte(labelsJSON), &svc.Labels)
		}
		svc.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, &svc)
	}
	return out, rows.Err()
}

// ListTasks returns swarm tasks. agentID and serviceID are optional filters
// (empty = no filter on that dimension).
func (s *SwarmTopologyStore) ListTasks(ctx context.Context, agentID, serviceID string) ([]*swarm.SwarmTask, error) {
	q := `SELECT agent_id, task_id, service_id, node_id, slot, state, desired_state,
		container_id, error, exit_code, timestamp, node_hostname FROM swarm_tasks`
	var conds []string
	var args []interface{}
	if agentID != "" {
		conds = append(conds, "agent_id=?")
		args = append(args, uid.Agent(agentID))
	}
	if serviceID != "" {
		conds = append(conds, "service_id=?")
		args = append(args, serviceID)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ") // #nosec G202 -- conds are constant "col=?" fragments; values are bound.
	}
	q += " ORDER BY service_id ASC, slot ASC"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list swarm tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*swarm.SwarmTask
	for rows.Next() {
		var (
			t         swarm.SwarmTask
			exitCode  sql.NullInt64
			timestamp int64
		)
		if err := rows.Scan(&t.AgentID, &t.TaskID, &t.ServiceID, &t.NodeID, &t.Slot, &t.State, &t.DesiredState,
			&t.ContainerID, &t.Error, &exitCode, &timestamp, &t.NodeHostname); err != nil {
			return nil, fmt.Errorf("scan swarm task: %w", err)
		}
		if exitCode.Valid {
			v := int(exitCode.Int64)
			t.ExitCode = &v
		}
		t.Timestamp = time.Unix(timestamp, 0)
		out = append(out, &t)
	}
	return out, rows.Err()
}

// deleteNotInForAgent hard-deletes every row of table owned by agentID whose id
// is not in keep. An empty keep wipes all of the agent's rows. table is a fixed
// identifier supplied by callers, never user input.
func deleteNotInForAgent(ctx context.Context, w *Writer, table, agentID string, keep []string) error {
	if len(keep) == 0 {
		_, err := w.Exec(ctx, "DELETE FROM "+table+" WHERE agent_id=?", agentID)
		if err != nil {
			return fmt.Errorf("prune %s for agent %s: %w", table, agentID, err)
		}
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keep)), ",")
	args := make([]interface{}, 0, len(keep)+1)
	args = append(args, agentID)
	for _, id := range keep {
		args = append(args, id)
	}
	_, err := w.Exec(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE agent_id=? AND id NOT IN (%s)", table, placeholders),
		args...)
	if err != nil {
		return fmt.Errorf("prune %s for agent %s: %w", table, agentID, err)
	}
	return nil
}
