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

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
)

// TriggerStoreImpl implements alert.TriggerStore using SQLite.
type TriggerStoreImpl struct {
	db     *sql.DB
	writer *Writer
}

// NewTriggerStore creates a new SQLite-backed trigger store.
func NewTriggerStore(d *DB) *TriggerStoreImpl {
	return &TriggerStoreImpl{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *TriggerStoreImpl) InsertTrigger(ctx context.Context, t *alert.AlertTrigger) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO alert_triggers
			(name, filter_severities, filter_sources, filter_scopes, filter_tags, enabled)
			VALUES (?, ?, ?, ?, ?, ?)`,
		t.Name, t.FilterSeverities, t.FilterSources, t.FilterScopes, t.FilterTags, boolToInt(t.Enabled),
	)
	if err != nil {
		return 0, fmt.Errorf("insert trigger: %w", err)
	}
	t.ID = res.LastInsertID
	if err := s.replaceChannels(ctx, t.ID, t.ChannelIDs); err != nil {
		return 0, err
	}
	return t.ID, nil
}

func (s *TriggerStoreImpl) GetTrigger(ctx context.Context, id int64) (*alert.AlertTrigger, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, filter_severities, filter_sources, filter_scopes, filter_tags,
			enabled, created_at, updated_at
			FROM alert_triggers WHERE id = ?`, id)

	t, err := scanTrigger(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	channelIDs, err := s.ListChannelsForTrigger(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	t.ChannelIDs = channelIDs

	return t, nil
}

func (s *TriggerStoreImpl) ListTriggers(ctx context.Context) ([]*alert.AlertTrigger, error) {
	return s.listTriggersWhere(ctx, "")
}

func (s *TriggerStoreImpl) ListEnabledTriggers(ctx context.Context) ([]*alert.AlertTrigger, error) {
	return s.listTriggersWhere(ctx, "WHERE enabled = 1")
}

func (s *TriggerStoreImpl) listTriggersWhere(ctx context.Context, where string) ([]*alert.AlertTrigger, error) {
	query := `SELECT id, name, filter_severities, filter_sources, filter_scopes, filter_tags,
		enabled, created_at, updated_at
		FROM alert_triggers ` + where + ` ORDER BY created_at ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list triggers: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var triggers []*alert.AlertTrigger
	for rows.Next() {
		t, err := scanTriggerRow(rows)
		if err != nil {
			return nil, err
		}
		triggers = append(triggers, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, t := range triggers {
		channelIDs, err := s.ListChannelsForTrigger(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		t.ChannelIDs = channelIDs
	}
	return triggers, nil
}

func (s *TriggerStoreImpl) UpdateTrigger(ctx context.Context, t *alert.AlertTrigger) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE alert_triggers
			SET name=?, filter_severities=?, filter_sources=?, filter_scopes=?, filter_tags=?,
				enabled=?, updated_at=?
			WHERE id=?`,
		t.Name, t.FilterSeverities, t.FilterSources, t.FilterScopes, t.FilterTags,
		boolToInt(t.Enabled), time.Now().UTC().Format(time.RFC3339), t.ID,
	)
	if err != nil {
		return fmt.Errorf("update trigger: %w", err)
	}
	return s.replaceChannels(ctx, t.ID, t.ChannelIDs)
}

func (s *TriggerStoreImpl) DeleteTrigger(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM alert_triggers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete trigger: %w", err)
	}
	return nil
}

// SetChannels replaces all channel links for a trigger atomically (within
// the writer's serialization guarantee).
func (s *TriggerStoreImpl) SetChannels(ctx context.Context, triggerID int64, channelIDs []int64) error {
	return s.replaceChannels(ctx, triggerID, channelIDs)
}

func (s *TriggerStoreImpl) replaceChannels(ctx context.Context, triggerID int64, channelIDs []int64) error {
	if _, err := s.writer.Exec(ctx,
		`DELETE FROM alert_trigger_channels WHERE trigger_id = ?`, triggerID); err != nil {
		return fmt.Errorf("clear channel links: %w", err)
	}
	if len(channelIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(channelIDs))
	args := make([]interface{}, 0, len(channelIDs)*2)
	for i, id := range channelIDs {
		placeholders[i] = "(?, ?)"
		args = append(args, triggerID, id)
	}
	q := `INSERT INTO alert_trigger_channels (trigger_id, channel_id) VALUES ` +
		strings.Join(placeholders, ", ")
	if _, err := s.writer.Exec(ctx, q, args...); err != nil {
		return fmt.Errorf("insert channel links: %w", err)
	}
	return nil
}

func (s *TriggerStoreImpl) ListChannelsForTrigger(ctx context.Context, triggerID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT channel_id FROM alert_trigger_channels WHERE trigger_id = ? ORDER BY channel_id ASC`,
		triggerID)
	if err != nil {
		return nil, fmt.Errorf("list channels for trigger: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []int64{}
	}
	return ids, rows.Err()
}

func (s *TriggerStoreImpl) ListTriggersForChannel(ctx context.Context, channelID int64) ([]*alert.AlertTrigger, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.name, t.filter_severities, t.filter_sources, t.filter_scopes, t.filter_tags,
			t.enabled, t.created_at, t.updated_at
			FROM alert_triggers t
			JOIN alert_trigger_channels atc ON atc.trigger_id = t.id
			WHERE atc.channel_id = ?
			ORDER BY t.created_at ASC`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list triggers for channel: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var triggers []*alert.AlertTrigger
	for rows.Next() {
		t, err := scanTriggerRow(rows)
		if err != nil {
			return nil, err
		}
		triggers = append(triggers, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, t := range triggers {
		channelIDs, err := s.ListChannelsForTrigger(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		t.ChannelIDs = channelIDs
	}
	return triggers, nil
}

// --- Scan helpers ---

func scanTrigger(row *sql.Row) (*alert.AlertTrigger, error) {
	t := &alert.AlertTrigger{}
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&t.ID, &t.Name, &t.FilterSeverities, &t.FilterSources,
		&t.FilterScopes, &t.FilterTags, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled == 1
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return t, nil
}

func scanTriggerRow(rows *sql.Rows) (*alert.AlertTrigger, error) {
	t := &alert.AlertTrigger{}
	var enabled int
	var createdAt, updatedAt string
	err := rows.Scan(&t.ID, &t.Name, &t.FilterSeverities, &t.FilterSources,
		&t.FilterScopes, &t.FilterTags, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled == 1
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return t, nil
}
