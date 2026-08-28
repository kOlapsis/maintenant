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

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/uid"
)

// ChannelStoreImpl implements alert.ChannelStore using SQLite.
type ChannelStoreImpl struct {
	db     *Reader
	writer *Writer
}

// NewChannelStore creates a new SQLite-backed channel store.
func NewChannelStore(d *DB) *ChannelStoreImpl {
	return &ChannelStoreImpl{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

func (s *ChannelStoreImpl) InsertChannel(ctx context.Context, ch *alert.NotificationChannel) (string, error) {
	ch.ID = uid.New()
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`INSERT INTO notification_channels (id, name, type, url, headers, secret, config, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ch.ID, ch.Name, ch.Type, ch.URL, NullableString(ch.Headers),
		NullableString(ch.Secret), NullableString(ch.Config),
		boolToInt(ch.Enabled), now, now,
	)
	if err != nil {
		return "", fmt.Errorf("insert channel: %w", err)
	}
	return ch.ID, nil
}

func (s *ChannelStoreImpl) GetChannel(ctx context.Context, id string) (*alert.NotificationChannel, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, type, url, headers, secret, config, enabled, created_at, updated_at
		FROM notification_channels WHERE id = ?`, id)

	ch, err := scanChannel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return ch, nil
}

func (s *ChannelStoreImpl) ListChannels(ctx context.Context) ([]*alert.NotificationChannel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, type, url, headers, secret, config, enabled, created_at, updated_at
		FROM notification_channels ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var channels []*alert.NotificationChannel
	for rows.Next() {
		ch, err := scanChannelRow(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Compute health for each channel
	for _, ch := range channels {
		health, err := s.GetChannelHealth(ctx, ch.ID)
		if err == nil {
			ch.Health = health
		}
	}

	return channels, nil
}

func (s *ChannelStoreImpl) UpdateChannel(ctx context.Context, ch *alert.NotificationChannel) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE notification_channels SET name=?, type=?, url=?, headers=?, secret=?, config=?, enabled=?, updated_at=?
		WHERE id=?`,
		ch.Name, ch.Type, ch.URL, NullableString(ch.Headers),
		NullableString(ch.Secret), NullableString(ch.Config), boolToInt(ch.Enabled),
		time.Now().Unix(), ch.ID,
	)
	if err != nil {
		return fmt.Errorf("update channel: %w", err)
	}
	return nil
}

func (s *ChannelStoreImpl) DeleteChannel(ctx context.Context, id string) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM notification_channels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	return nil
}

func (s *ChannelStoreImpl) GetChannelHealth(ctx context.Context, channelID string) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM notification_deliveries
		WHERE channel_id = ? ORDER BY updated_at DESC LIMIT 1`,
		channelID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "healthy", nil
	}
	if err != nil {
		return "", err
	}
	if status == alert.DeliveryFailed {
		return "failing", nil
	}
	return "healthy", nil
}

// --- Notification Deliveries ---

func (s *ChannelStoreImpl) InsertDelivery(ctx context.Context, d *alert.NotificationDelivery) (string, error) {
	d.ID = uid.New()
	now := time.Now().Unix()
	_, err := s.writer.Exec(ctx,
		`INSERT INTO notification_deliveries (id, alert_id, channel_id, status, attempts, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.AlertID, d.ChannelID, d.Status, d.Attempts, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("insert delivery: %w", err)
	}
	return d.ID, nil
}

func (s *ChannelStoreImpl) UpdateDelivery(ctx context.Context, d *alert.NotificationDelivery) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE notification_deliveries SET status=?, attempts=?, last_error=?, updated_at=?
		WHERE id=?`,
		d.Status, d.Attempts, NullableString(d.LastError),
		time.Now().Unix(), d.ID,
	)
	if err != nil {
		return fmt.Errorf("update delivery: %w", err)
	}
	return nil
}

func (s *ChannelStoreImpl) ListDeliveriesByAlert(ctx context.Context, alertID string) ([]*alert.NotificationDelivery, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, alert_id, channel_id, status, attempts, last_error, created_at, updated_at
		FROM notification_deliveries WHERE alert_id = ? ORDER BY created_at ASC`, alertID)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var deliveries []*alert.NotificationDelivery
	for rows.Next() {
		d := &alert.NotificationDelivery{}
		var lastError sql.NullString
		var createdAt, updatedAt int64
		if err := rows.Scan(&d.ID, &d.AlertID, &d.ChannelID, &d.Status, &d.Attempts, &lastError, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if lastError.Valid {
			d.LastError = lastError.String
		}
		d.CreatedAt = time.Unix(createdAt, 0)
		d.UpdatedAt = time.Unix(updatedAt, 0)
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// --- Scan helpers ---

func scanChannel(row *sql.Row) (*alert.NotificationChannel, error) {
	ch := &alert.NotificationChannel{}
	var headers, secret, config sql.NullString
	var enabled int
	var createdAt, updatedAt int64

	err := row.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.URL, &headers, &secret, &config, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	ch.Enabled = enabled == 1
	if headers.Valid {
		ch.Headers = headers.String
	}
	ch.Secret = secret.String
	ch.Config = config.String
	ch.HasSecret = ch.Secret != ""
	ch.CreatedAt = time.Unix(createdAt, 0)
	ch.UpdatedAt = time.Unix(updatedAt, 0)
	return ch, nil
}

func scanChannelRow(rows *sql.Rows) (*alert.NotificationChannel, error) {
	ch := &alert.NotificationChannel{}
	var headers, secret, config sql.NullString
	var enabled int
	var createdAt, updatedAt int64

	err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.URL, &headers, &secret, &config, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	ch.Enabled = enabled == 1
	if headers.Valid {
		ch.Headers = headers.String
	}
	ch.Secret = secret.String
	ch.Config = config.String
	ch.HasSecret = ch.Secret != ""
	ch.CreatedAt = time.Unix(createdAt, 0)
	ch.UpdatedAt = time.Unix(updatedAt, 0)
	return ch, nil
}
