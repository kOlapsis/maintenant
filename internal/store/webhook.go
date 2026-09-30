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

	"github.com/kolapsis/maintenant/internal/webhook"
)

// WebhookStoreImpl implements webhook.WebhookSubscriptionStore using SQLite.
type WebhookStoreImpl struct {
	db     *Reader
	writer *Writer
}

// NewWebhookStore creates a new SQLite-backed webhook subscription store.
func NewWebhookStore(d *DB) *WebhookStoreImpl {
	return &WebhookStoreImpl{
		db:     d.Reader(),
		writer: d.Writer(),
	}
}

func (s *WebhookStoreImpl) List(ctx context.Context) ([]*webhook.WebhookSubscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, secret, event_types, is_active,
		        last_delivery_status, last_delivery_at, failure_count, created_at
		 FROM webhook_subscriptions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var subs []*webhook.WebhookSubscription
	for rows.Next() {
		sub, err := scanWebhookRow(rows)
		if err != nil {
			return nil, err
		}
		sub.Secret = "" // never expose secret in a list
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *WebhookStoreImpl) ListActive(ctx context.Context) ([]*webhook.WebhookSubscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, secret, event_types, is_active,
		        last_delivery_status, last_delivery_at, failure_count, created_at
		 FROM webhook_subscriptions WHERE is_active = 1`)
	if err != nil {
		return nil, fmt.Errorf("list active webhooks: %w", err)
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	var subs []*webhook.WebhookSubscription
	for rows.Next() {
		sub, err := scanWebhookRow(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *WebhookStoreImpl) GetByID(ctx context.Context, id string) (*webhook.WebhookSubscription, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, secret, event_types, is_active,
		        last_delivery_status, last_delivery_at, failure_count, created_at
		 FROM webhook_subscriptions WHERE id = ?`, id)

	var sub webhook.WebhookSubscription
	var secret sql.NullString
	var eventTypesStr string
	var lastStatus sql.NullString
	var lastDeliveryAt, createdAt any

	err := row.Scan(&sub.ID, &sub.Name, &sub.URL, &secret,
		&eventTypesStr, &sub.IsActive, &lastStatus, &lastDeliveryAt,
		&sub.FailureCount, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook: %w", err)
	}

	populateWebhookFields(&sub, secret, eventTypesStr, lastStatus, lastDeliveryAt, createdAt)
	return &sub, nil
}

func (s *WebhookStoreImpl) Create(ctx context.Context, sub *webhook.WebhookSubscription) error {
	eventTypesJSON, err := json.Marshal(sub.EventTypes)
	if err != nil {
		return fmt.Errorf("marshal event_types: %w", err)
	}

	var secretVal any
	if sub.Secret != "" {
		secretVal = sub.Secret
	}

	_, err = s.writer.Exec(ctx,
		`INSERT INTO webhook_subscriptions (id, name, url, secret, event_types, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.Name, sub.URL, secretVal,
		string(eventTypesJSON), sub.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("create webhook: %w", err)
	}
	return nil
}

func (s *WebhookStoreImpl) Delete(ctx context.Context, id string) error {
	res, err := s.writer.Exec(ctx,
		`DELETE FROM webhook_subscriptions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("webhook not found")
	}
	return nil
}

func (s *WebhookStoreImpl) RecordDelivery(ctx context.Context, id string, delivered bool) error {
	status, success := webhook.DeliveryFailed, 0
	if delivered {
		status, success = webhook.DeliveryDelivered, 1
	}
	_, err := s.writer.Exec(ctx,
		`UPDATE webhook_subscriptions
		 SET last_delivery_status = ?, last_delivery_at = ?,
		     failure_count = CASE WHEN ? = 1 THEN 0 ELSE failure_count + 1 END,
		     is_active = CASE WHEN ? = 1 THEN 1 WHEN failure_count + 1 >= ? THEN 0 ELSE is_active END
		 WHERE id = ?`,
		status, time.Now().Unix(), success, success, webhook.MaxConsecutiveFailures, id)
	if err != nil {
		return fmt.Errorf("record webhook delivery: %w", err)
	}
	return nil
}

func scanWebhookRow(rows *sql.Rows) (*webhook.WebhookSubscription, error) {
	var sub webhook.WebhookSubscription
	var secret sql.NullString
	var eventTypesStr string
	var lastStatus sql.NullString
	var lastDeliveryAt, createdAt any

	err := rows.Scan(&sub.ID, &sub.Name, &sub.URL, &secret,
		&eventTypesStr, &sub.IsActive, &lastStatus, &lastDeliveryAt,
		&sub.FailureCount, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scan webhook row: %w", err)
	}

	populateWebhookFields(&sub, secret, eventTypesStr, lastStatus, lastDeliveryAt, createdAt)
	return &sub, nil
}

func populateWebhookFields(sub *webhook.WebhookSubscription, secret sql.NullString, eventTypesStr string, lastStatus sql.NullString, lastDeliveryAt, createdAt any) {
	if secret.Valid {
		sub.Secret = secret.String
	}
	if err := json.Unmarshal([]byte(eventTypesStr), &sub.EventTypes); err != nil {
		sub.EventTypes = []string{"*"}
	}
	if lastStatus.Valid {
		s := lastStatus.String
		sub.LastDeliveryStatus = &s
	}
	if t, ok := webhookTime(lastDeliveryAt); ok {
		sub.LastDeliveryAt = &t
	}
	if t, ok := webhookTime(createdAt); ok {
		sub.CreatedAt = t
	}
}

// webhookTime reads an epoch column that earlier builds filled with RFC 3339 text on SQLite.
func webhookTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case int64:
		return time.Unix(t, 0), true
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		return parsed, err == nil
	case []byte:
		parsed, err := time.Parse(time.RFC3339, string(t))
		return parsed, err == nil
	}
	return time.Time{}, false
}

// CountConfigured returns the number of operator-configured webhook
// subscriptions. is_active=0 means operator-paused (still counted per spec).
// Used by the telemetry subsystem; see specs/015-shm-telemetry.
func (s *WebhookStoreImpl) CountConfigured(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhook_subscriptions`,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count configured webhooks: %w", err)
	}
	return count, nil
}
