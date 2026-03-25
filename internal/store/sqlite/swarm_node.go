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
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/swarm"
)

const swarmNodeColumns = `id, node_id, hostname, role, status, availability,
	engine_version, address, task_count, first_seen_at, last_seen_at, last_status_change_at`

// SwarmNodeStore implements swarm node persistence using SQLite.
type SwarmNodeStore struct {
	db     *sql.DB
	writer *Writer
}

// NewSwarmNodeStore creates a new SQLite-backed swarm node store.
func NewSwarmNodeStore(d *DB) *SwarmNodeStore {
	return &SwarmNodeStore{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

// UpsertNode inserts or updates a swarm node by node_id.
func (s *SwarmNodeStore) UpsertNode(ctx context.Context, node *swarm.SwarmNode) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO swarm_nodes (node_id, hostname, role, status, availability,
			engine_version, address, task_count, first_seen_at, last_seen_at, last_status_change_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET
			hostname=excluded.hostname,
			role=excluded.role,
			status=excluded.status,
			availability=excluded.availability,
			engine_version=excluded.engine_version,
			address=excluded.address,
			task_count=excluded.task_count,
			last_seen_at=excluded.last_seen_at,
			last_status_change_at=CASE
				WHEN swarm_nodes.status != excluded.status OR swarm_nodes.availability != excluded.availability
				THEN excluded.last_status_change_at
				ELSE swarm_nodes.last_status_change_at
			END`,
		node.NodeID, node.Hostname, node.Role, node.Status, node.Availability,
		node.EngineVersion, node.Address, node.TaskCount,
		node.FirstSeenAt.Unix(), node.LastSeenAt.Unix(), node.LastStatusChangeAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("upsert swarm node %s: %w", node.NodeID, err)
	}
	return nil
}

// ListNodes returns all swarm nodes.
func (s *SwarmNodeStore) ListNodes(ctx context.Context) ([]*swarm.SwarmNode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+swarmNodeColumns+` FROM swarm_nodes ORDER BY hostname ASC`)
	if err != nil {
		return nil, fmt.Errorf("list swarm nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nodes []*swarm.SwarmNode
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// GetNodeByNodeID returns a single swarm node by its Docker node ID.
func (s *SwarmNodeStore) GetNodeByNodeID(ctx context.Context, nodeID string) (*swarm.SwarmNode, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+swarmNodeColumns+` FROM swarm_nodes WHERE node_id=?`, nodeID)
	return scanNode(row)
}

// UpdateNodeStatus updates the status and availability of a swarm node.
func (s *SwarmNodeStore) UpdateNodeStatus(ctx context.Context, nodeID, status, availability string) error {
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`UPDATE swarm_nodes SET status=?, availability=?, last_seen_at=?, last_status_change_at=?
		WHERE node_id=?`,
		status, availability, now, now, nodeID,
	)
	if err != nil {
		return fmt.Errorf("update swarm node status %s: %w", nodeID, err)
	}
	return nil
}

// UpdateNodeTaskCount updates the task count for a swarm node.
func (s *SwarmNodeStore) UpdateNodeTaskCount(ctx context.Context, nodeID string, count int) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE swarm_nodes SET task_count=? WHERE node_id=?`,
		count, nodeID,
	)
	if err != nil {
		return fmt.Errorf("update swarm node task count %s: %w", nodeID, err)
	}
	return nil
}

func scanNode(row rowScanner) (*swarm.SwarmNode, error) {
	var n swarm.SwarmNode
	var firstSeen, lastSeen, lastStatusChange int64

	err := row.Scan(
		&n.ID, &n.NodeID, &n.Hostname, &n.Role, &n.Status, &n.Availability,
		&n.EngineVersion, &n.Address, &n.TaskCount,
		&firstSeen, &lastSeen, &lastStatusChange,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan swarm node: %w", err)
	}

	n.FirstSeenAt = time.Unix(firstSeen, 0)
	n.LastSeenAt = time.Unix(lastSeen, 0)
	n.LastStatusChangeAt = time.Unix(lastStatusChange, 0)

	return &n, nil
}
