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
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/outbound"
)

// OutboundHeartbeatStore implements outbound.Store.
type OutboundHeartbeatStore struct {
	db     *Reader
	writer *Writer
}

// NewOutboundHeartbeatStore creates an outbound heartbeat store.
func NewOutboundHeartbeatStore(d *DB) *OutboundHeartbeatStore {
	return &OutboundHeartbeatStore{db: d.Reader(), writer: d.Writer()}
}

const outboundHeartbeatColumns = `id, name, url, interval_seconds, enabled,
	last_sent_at, last_status_code, last_error, created_at, updated_at`

func (s *OutboundHeartbeatStore) List(ctx context.Context) ([]*outbound.OutboundHeartbeat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+outboundHeartbeatColumns+` FROM outbound_heartbeats ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list outbound heartbeats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*outbound.OutboundHeartbeat
	for rows.Next() {
		o, err := scanOutboundHeartbeat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *OutboundHeartbeatStore) Get(ctx context.Context, id string) (*outbound.OutboundHeartbeat, error) {
	o, err := scanOutboundHeartbeat(s.db.QueryRowContext(ctx,
		`SELECT `+outboundHeartbeatColumns+` FROM outbound_heartbeats WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return o, err
}

func (s *OutboundHeartbeatStore) Create(ctx context.Context, o *outbound.OutboundHeartbeat) error {
	_, err := s.writer.Exec(ctx,
		`INSERT INTO outbound_heartbeats (id, name, url, interval_seconds, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		o.ID, o.Name, o.URL, o.IntervalSeconds, boolToInt(o.Enabled),
		o.CreatedAt.Unix(), o.UpdatedAt.Unix())
	if err != nil {
		return fmt.Errorf("insert outbound heartbeat: %w", err)
	}
	return nil
}

func (s *OutboundHeartbeatStore) Update(ctx context.Context, o *outbound.OutboundHeartbeat) (bool, error) {
	res, err := s.writer.Exec(ctx,
		`UPDATE outbound_heartbeats SET name=?, url=?, interval_seconds=?, enabled=?, updated_at=?
		WHERE id=?`,
		o.Name, o.URL, o.IntervalSeconds, boolToInt(o.Enabled), o.UpdatedAt.Unix(), o.ID)
	if err != nil {
		return false, fmt.Errorf("update outbound heartbeat %s: %w", o.ID, err)
	}
	return res.RowsAffected > 0, nil
}

func (s *OutboundHeartbeatStore) Delete(ctx context.Context, id string) (bool, error) {
	res, err := s.writer.Exec(ctx, `DELETE FROM outbound_heartbeats WHERE id=?`, id)
	if err != nil {
		return false, fmt.Errorf("delete outbound heartbeat %s: %w", id, err)
	}
	return res.RowsAffected > 0, nil
}

func (s *OutboundHeartbeatStore) RecordSend(ctx context.Context, id string, r outbound.SendResult) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE outbound_heartbeats SET last_sent_at=?, last_status_code=?, last_error=? WHERE id=?`,
		r.SentAt.Unix(), r.StatusCode, r.Error, id)
	if err != nil {
		return fmt.Errorf("record outbound heartbeat send %s: %w", id, err)
	}
	return nil
}

func scanOutboundHeartbeat(row rowScanner) (*outbound.OutboundHeartbeat, error) {
	var o outbound.OutboundHeartbeat
	var enabled int
	var lastSentAt, lastStatusCode sql.NullInt64
	var lastError sql.NullString
	var createdAt, updatedAt int64
	if err := row.Scan(&o.ID, &o.Name, &o.URL, &o.IntervalSeconds, &enabled,
		&lastSentAt, &lastStatusCode, &lastError, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("scan outbound heartbeat: %w", err)
	}
	o.Enabled = enabled != 0
	o.CreatedAt = time.Unix(createdAt, 0)
	o.UpdatedAt = time.Unix(updatedAt, 0)
	if lastSentAt.Valid {
		t := time.Unix(lastSentAt.Int64, 0)
		o.LastSentAt = &t
	}
	if lastStatusCode.Valid {
		v := int(lastStatusCode.Int64)
		o.LastStatusCode = &v
	}
	if lastError.Valid {
		v := lastError.String
		o.LastError = &v
	}
	return &o, nil
}
