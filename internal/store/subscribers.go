// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/uid"
)

// SubscriberStoreImpl implements status.SubscriberStore using SQLite.
type SubscriberStoreImpl struct {
	db     *Reader
	writer *Writer
}

// NewSubscriberStore creates a new SQLite-backed subscriber store.
func NewSubscriberStore(d *DB) *SubscriberStoreImpl {
	return &SubscriberStoreImpl{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

// UpsertPendingSubscriber records sub as an unconfirmed subscription, or hands a still unconfirmed one sub's confirmation token and a fresh 24 h window; issued is false when the address is already confirmed.
func (s *SubscriberStoreImpl) UpsertPendingSubscriber(ctx context.Context, sub *status.StatusSubscriber) (bool, error) {
	now := time.Now().Unix()
	var confirmExpires *int64
	if sub.ConfirmExpires != nil {
		v := sub.ConfirmExpires.Unix()
		confirmExpires = &v
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO status_subscribers (id, email, confirmed, confirm_token, confirm_expires, unsub_token, created_at)
		VALUES (?, ?, 0, ?, ?, ?, ?)
		ON CONFLICT (email) DO UPDATE SET
			confirm_token = excluded.confirm_token,
			confirm_expires = excluded.confirm_expires,
			created_at = excluded.created_at
		WHERE status_subscribers.confirmed = 0`,
		uid.New(), sub.Email, sub.ConfirmToken, confirmExpires, sub.UnsubToken, now,
	)
	if err != nil {
		return false, fmt.Errorf("upsert subscriber: %w", err)
	}
	return res.RowsAffected == 1, nil
}

func (s *SubscriberStoreImpl) GetSubscriberByToken(ctx context.Context, confirmToken string) (*status.StatusSubscriber, error) {
	return s.getSubscriberBy(ctx, "confirm_token", confirmToken)
}

func (s *SubscriberStoreImpl) GetSubscriberByUnsubToken(ctx context.Context, unsubToken string) (*status.StatusSubscriber, error) {
	return s.getSubscriberBy(ctx, "unsub_token", unsubToken)
}

func (s *SubscriberStoreImpl) getSubscriberBy(ctx context.Context, column, value string) (*status.StatusSubscriber, error) {
	switch column {
	case "confirm_token", "unsub_token":
		// allowed
	default:
		return nil, fmt.Errorf("invalid column: %s", column)
	}

	var sub status.StatusSubscriber
	var confirmed int
	var confirmToken sql.NullString
	var confirmExpires sql.NullInt64
	var createdAt int64

	err := s.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT id, email, confirmed, confirm_token, confirm_expires, unsub_token, created_at
		FROM status_subscribers WHERE %s = ?`, column), value,
	).Scan(&sub.ID, &sub.Email, &confirmed, &confirmToken, &confirmExpires, &sub.UnsubToken, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get subscriber by %s: %w", column, err)
	}

	sub.Confirmed = confirmed != 0
	if confirmToken.Valid {
		sub.ConfirmToken = &confirmToken.String
	}
	if confirmExpires.Valid {
		t := time.Unix(confirmExpires.Int64, 0).UTC()
		sub.ConfirmExpires = &t
	}
	sub.CreatedAt = time.Unix(createdAt, 0).UTC()
	return &sub, nil
}

func (s *SubscriberStoreImpl) ConfirmSubscriber(ctx context.Context, id string) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE status_subscribers SET confirmed = 1, confirm_token = NULL, confirm_expires = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("confirm subscriber: %w", err)
	}
	return nil
}

func (s *SubscriberStoreImpl) DeleteSubscriber(ctx context.Context, id string) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM status_subscribers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete subscriber: %w", err)
	}
	return nil
}

func (s *SubscriberStoreImpl) ListConfirmedSubscribers(ctx context.Context) ([]status.StatusSubscriber, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, confirmed, confirm_token, confirm_expires, unsub_token, created_at
		FROM status_subscribers WHERE confirmed = 1 ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list confirmed subscribers: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return scanStatusSubscribers(rows)
}

func (s *SubscriberStoreImpl) ListSubscribers(ctx context.Context) ([]status.StatusSubscriber, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, confirmed, confirm_token, confirm_expires, unsub_token, created_at
		FROM status_subscribers ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list subscribers: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	return scanStatusSubscribers(rows)
}

func (s *SubscriberStoreImpl) GetSubscriberStats(ctx context.Context) (*status.SubscriberStats, error) {
	var stats status.SubscriberStats
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(confirmed), 0) FROM status_subscribers`,
	).Scan(&stats.Total, &stats.Confirmed)
	if err != nil {
		return nil, fmt.Errorf("subscriber stats: %w", err)
	}
	return &stats, nil
}

func (s *SubscriberStoreImpl) CleanExpiredUnconfirmed(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-24 * time.Hour).Unix()
	res, err := s.writer.Exec(ctx,
		`DELETE FROM status_subscribers WHERE confirmed = 0 AND created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("clean expired: %w", err)
	}
	return res.RowsAffected, nil
}

func scanStatusSubscribers(rows *sql.Rows) ([]status.StatusSubscriber, error) {
	var subs []status.StatusSubscriber
	for rows.Next() {
		var sub status.StatusSubscriber
		var confirmed int
		var confirmToken sql.NullString
		var confirmExpires sql.NullInt64
		var createdAt int64

		if err := rows.Scan(&sub.ID, &sub.Email, &confirmed, &confirmToken,
			&confirmExpires, &sub.UnsubToken, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan subscriber: %w", err)
		}

		sub.Confirmed = confirmed != 0
		if confirmToken.Valid {
			sub.ConfirmToken = &confirmToken.String
		}
		if confirmExpires.Valid {
			t := time.Unix(confirmExpires.Int64, 0).UTC()
			sub.ConfirmExpires = &t
		}
		sub.CreatedAt = time.Unix(createdAt, 0).UTC()
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}
