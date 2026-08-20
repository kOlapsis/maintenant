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

package store

import (
	"context"
	"fmt"
	"time"
)

// Instance is one running server process, registered for visibility only
// (FR-012). The table informs, it never arbitrates: no lock, no lease, no
// election (FR-013) — exclusion belongs to the operator's cluster manager.
type Instance struct {
	ID         string
	Hostname   string
	Version    string
	StartedAt  time.Time
	LastSeenAt time.Time
}

// InstanceStore records this instance's heartbeat and reports peers seen on
// the same database.
type InstanceStore struct {
	db     *Reader
	writer *Writer
}

func NewInstanceStore(d *DB) *InstanceStore {
	return &InstanceStore{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

// Register inserts this instance's row at startup. The id is ephemeral,
// minted per process; a stale row with the same id cannot exist.
func (s *InstanceStore) Register(ctx context.Context, in Instance) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO instances (id, hostname, version, started_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)`,
		in.ID, in.Hostname, in.Version, in.StartedAt.Unix(), in.LastSeenAt.Unix())
	if err != nil {
		return fmt.Errorf("register instance: %w", err)
	}
	return nil
}

// Beat refreshes this instance's last_seen_at.
func (s *InstanceStore) Beat(ctx context.Context, id string, now time.Time) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE instances SET last_seen_at = ? WHERE id = ?`, now.Unix(), id)
	if err != nil {
		return fmt.Errorf("beat instance: %w", err)
	}
	return nil
}

// Peers returns the other instances seen since the given time, most recent
// first.
func (s *InstanceStore) Peers(ctx context.Context, selfID string, since time.Time) ([]Instance, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, hostname, version, started_at, last_seen_at FROM instances
		WHERE id != ? AND last_seen_at >= ? ORDER BY last_seen_at DESC`,
		selfID, since.Unix())
	if err != nil {
		return nil, fmt.Errorf("list peer instances: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var peers []Instance
	for rows.Next() {
		var in Instance
		var started, seen int64
		if err := rows.Scan(&in.ID, &in.Hostname, &in.Version, &started, &seen); err != nil {
			return nil, fmt.Errorf("scan peer instance: %w", err)
		}
		in.StartedAt = time.Unix(started, 0)
		in.LastSeenAt = time.Unix(seen, 0)
		peers = append(peers, in)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peer instances: %w", err)
	}
	return peers, nil
}

// Deregister removes this instance's row on a clean shutdown, so a restart
// does not see its own previous run as a peer for the next few minutes.
// A process that dies without getting here is caught by PurgeStale instead.
//
// It writes through the pool rather than the serialized writer: shutdown runs
// after the writer's context is cancelled, so submitting there would be
// dropped. Nothing else writes at that point, so SQLite's single-writer
// discipline is not at stake.
func (s *InstanceStore) Deregister(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM instances WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deregister instance: %w", err)
	}
	return nil
}

// PurgeStale removes instances whose heartbeat stopped before the cutoff, so
// crashed processes do not read as peers forever.
func (s *InstanceStore) PurgeStale(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`DELETE FROM instances WHERE last_seen_at < ?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("purge stale instances: %w", err)
	}
	return res.RowsAffected, nil
}
