// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"encoding/json"
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
		ch, err := scanChannel(rows)
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
	var attempts int
	err := s.db.QueryRowContext(ctx,
		`SELECT status, attempts FROM notification_deliveries
		WHERE channel_id = ? ORDER BY updated_at DESC LIMIT 1`,
		channelID).Scan(&status, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return "healthy", nil
	}
	if err != nil {
		return "", err
	}
	if status == alert.DeliveryFailed || (status == alert.DeliveryPending && attempts > 0) {
		return "failing", nil
	}
	return "healthy", nil
}

// --- Notification Deliveries ---

// InsertDelivery stores d as a pending delivery due at once, in place of a reminder of the same alert to the same
// channel still pending, and sets its id and dates.
func (s *ChannelStoreImpl) InsertDelivery(ctx context.Context, d *alert.NotificationDelivery) (string, error) {
	payload := ""
	if d.Payload != nil {
		b, err := json.Marshal(d.Payload)
		if err != nil {
			return "", fmt.Errorf("encode delivery payload: %w", err)
		}
		payload = string(b)
	}
	id := uid.New()
	now := time.Now().Unix()
	err := s.writer.Tx(ctx, func(ctx context.Context, tx *Tx) error {
		if d.EventType == alert.EventReminder {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM notification_deliveries WHERE alert_id=? AND channel_id=? AND event_type=? AND status=?`,
				d.AlertID, d.ChannelID, alert.EventReminder, alert.DeliveryPending,
			); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO notification_deliveries (id, alert_id, channel_id, event_type, status, attempts, payload, next_attempt_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, d.AlertID, d.ChannelID, d.EventType, d.Status, d.Attempts, NullableString(payload), now, now, now,
		)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("insert delivery: %w", err)
	}
	d.ID = id
	d.CreatedAt, d.UpdatedAt, d.NextAttemptAt = time.Unix(now, 0), time.Unix(now, 0), time.Unix(now, 0)
	return d.ID, nil
}

func (s *ChannelStoreImpl) UpdateDelivery(ctx context.Context, d *alert.NotificationDelivery) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE notification_deliveries SET status=?, attempts=?, last_error=?, next_attempt_at=?, updated_at=?,
			payload = CASE WHEN CAST(? AS TEXT) = 'pending' THEN payload END
		WHERE id=?`,
		d.Status, d.Attempts, NullableString(d.LastError), d.NextAttemptAt.Unix(),
		time.Now().Unix(), d.Status, d.ID,
	)
	if err != nil {
		return fmt.Errorf("update delivery: %w", err)
	}
	return nil
}

// ClaimDelivery holds the attempt of d due at d.NextAttemptAt until until, unless another worker took it, d moved on,
// or a delivery of the same alert to the same channel created before d is still pending.
func (s *ChannelStoreImpl) ClaimDelivery(ctx context.Context, d *alert.NotificationDelivery, until time.Time) (bool, error) {
	res, err := s.writer.Exec(ctx,
		`UPDATE notification_deliveries SET next_attempt_at = ?, updated_at = ?
		WHERE id = ? AND status = ? AND next_attempt_at = ?
		AND NOT EXISTS (SELECT 1 FROM notification_deliveries p
			WHERE p.alert_id = ? AND p.channel_id = ? AND p.status = ? AND p.id < ?)`,
		until.Unix(), time.Now().Unix(),
		d.ID, alert.DeliveryPending, d.NextAttemptAt.Unix(),
		d.AlertID, d.ChannelID, alert.DeliveryPending, d.ID,
	)
	if err != nil {
		return false, fmt.Errorf("claim delivery: %w", err)
	}
	if res.RowsAffected != 1 {
		return false, nil
	}
	d.NextAttemptAt = time.Unix(until.Unix(), 0)
	return true, nil
}

// SettleDelivery records the outcome of the attempt of d claimed until claim, and reports false when that claim no longer holds.
func (s *ChannelStoreImpl) SettleDelivery(ctx context.Context, d *alert.NotificationDelivery, claim time.Time) (bool, error) {
	res, err := s.writer.Exec(ctx,
		`UPDATE notification_deliveries SET status=?, attempts=?, last_error=?, next_attempt_at=?, updated_at=?,
			payload = CASE WHEN CAST(? AS TEXT) = 'pending' THEN payload END
		WHERE id=? AND status=? AND next_attempt_at=?`,
		d.Status, d.Attempts, NullableString(d.LastError), d.NextAttemptAt.Unix(), time.Now().Unix(), d.Status,
		d.ID, alert.DeliveryPending, claim.Unix(),
	)
	if err != nil {
		return false, fmt.Errorf("settle delivery: %w", err)
	}
	return res.RowsAffected == 1, nil
}

// DropDelivery deletes the delivery d claimed until claim, and reports false when that claim no longer holds.
func (s *ChannelStoreImpl) DropDelivery(ctx context.Context, d *alert.NotificationDelivery, claim time.Time) (bool, error) {
	res, err := s.writer.Exec(ctx,
		`DELETE FROM notification_deliveries WHERE id=? AND status=? AND next_attempt_at=?`,
		d.ID, alert.DeliveryPending, claim.Unix(),
	)
	if err != nil {
		return false, fmt.Errorf("drop delivery: %w", err)
	}
	return res.RowsAffected == 1, nil
}

// ListDueDeliveries lists up to limit pending deliveries due at now, without those that wait behind an earlier pending
// delivery of the same alert to the same channel, taking the oldest due of each channel in turn so that no channel
// fills the list while another has one due.
func (s *ChannelStoreImpl) ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]*alert.NotificationDelivery, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+deliveryColumns+` FROM (
			SELECT q.*, ROW_NUMBER() OVER (PARTITION BY q.channel_id ORDER BY q.next_attempt_at, q.id) AS channel_rank
			FROM notification_deliveries q
			WHERE q.status = ? AND q.next_attempt_at <= ?
			AND NOT EXISTS (SELECT 1 FROM notification_deliveries p
				WHERE p.alert_id = q.alert_id AND p.channel_id = q.channel_id AND p.status = ? AND p.id < q.id)
		) d
		ORDER BY d.channel_rank, d.next_attempt_at, d.id LIMIT ?`,
		alert.DeliveryPending, now.Unix(), alert.DeliveryPending, limit)
	if err != nil {
		return nil, fmt.Errorf("list due deliveries: %w", err)
	}
	return scanDeliveries(rows)
}

func (s *ChannelStoreImpl) ListDeliveriesByAlert(ctx context.Context, alertID string) ([]*alert.NotificationDelivery, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+deliveryColumns+` FROM notification_deliveries d WHERE d.alert_id = ? ORDER BY d.created_at ASC, d.id ASC`, alertID)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	return scanDeliveries(rows)
}

const deliveryColumns = `d.id, d.alert_id, d.channel_id, d.event_type, d.status, d.attempts, d.last_error, d.payload,
	d.next_attempt_at, d.created_at, d.updated_at`

func scanDeliveries(rows *sql.Rows) ([]*alert.NotificationDelivery, error) {
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var deliveries []*alert.NotificationDelivery
	for rows.Next() {
		d := &alert.NotificationDelivery{}
		var lastError, payload sql.NullString
		var nextAttemptAt, createdAt, updatedAt int64
		if err := rows.Scan(&d.ID, &d.AlertID, &d.ChannelID, &d.EventType, &d.Status, &d.Attempts, &lastError, &payload,
			&nextAttemptAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if lastError.Valid {
			d.LastError = lastError.String
		}
		if payload.Valid {
			a := &alert.Alert{}
			if json.Unmarshal([]byte(payload.String), a) == nil {
				d.Payload = a
			}
		}
		d.NextAttemptAt = time.Unix(nextAttemptAt, 0)
		d.CreatedAt = time.Unix(createdAt, 0)
		d.UpdatedAt = time.Unix(updatedAt, 0)
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// --- Scan helpers ---

func scanChannel(row rowScanner) (*alert.NotificationChannel, error) {
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
