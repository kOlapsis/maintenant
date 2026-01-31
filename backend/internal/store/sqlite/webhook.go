package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kolapsis/pulseboard/internal/webhook"
)

// WebhookStoreImpl implements webhook.WebhookSubscriptionStore using SQLite.
type WebhookStoreImpl struct {
	db     *sql.DB
	writer *Writer
}

// NewWebhookStore creates a new SQLite-backed webhook subscription store.
func NewWebhookStore(d *DB) *WebhookStoreImpl {
	return &WebhookStoreImpl{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *WebhookStoreImpl) List(ctx context.Context) ([]*webhook.WebhookSubscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, url, secret, event_types, is_active,
		        last_delivery_status, last_delivery_at, failure_count, created_at
		 FROM webhook_subscriptions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer rows.Close()

	var subs []*webhook.WebhookSubscription
	for rows.Next() {
		sub, err := scanWebhookRow(rows)
		if err != nil {
			return nil, err
		}
		sub.Secret = "" // never expose secret in list
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *WebhookStoreImpl) ListActive(ctx context.Context) ([]*webhook.WebhookSubscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, url, secret, event_types, is_active,
		        last_delivery_status, last_delivery_at, failure_count, created_at
		 FROM webhook_subscriptions WHERE is_active = 1`)
	if err != nil {
		return nil, fmt.Errorf("list active webhooks: %w", err)
	}
	defer rows.Close()

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
		`SELECT id, user_id, name, url, secret, event_types, is_active,
		        last_delivery_status, last_delivery_at, failure_count, created_at
		 FROM webhook_subscriptions WHERE id = ?`, id)

	var sub webhook.WebhookSubscription
	var secret sql.NullString
	var eventTypesStr string
	var lastStatus sql.NullString
	var lastDeliveryAt sql.NullString
	var createdAt string

	err := row.Scan(&sub.ID, &sub.UserID, &sub.Name, &sub.URL, &secret,
		&eventTypesStr, &sub.IsActive, &lastStatus, &lastDeliveryAt,
		&sub.FailureCount, &createdAt)
	if err == sql.ErrNoRows {
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

	var secretVal interface{}
	if sub.Secret != "" {
		secretVal = sub.Secret
	}

	_, err = s.writer.Exec(ctx,
		`INSERT INTO webhook_subscriptions (id, user_id, name, url, secret, event_types, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.UserID, sub.Name, sub.URL, secretVal,
		string(eventTypesJSON), sub.CreatedAt.UTC().Format(time.RFC3339))
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

func (s *WebhookStoreImpl) UpdateDeliveryStatus(ctx context.Context, id string, status string, failureCount int) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// Auto-disable at MaxConsecutiveFailures
	isActive := 1
	if failureCount >= webhook.MaxConsecutiveFailures {
		isActive = 0
	}

	_, err := s.writer.Exec(ctx,
		`UPDATE webhook_subscriptions
		 SET last_delivery_status = ?, last_delivery_at = ?, failure_count = ?, is_active = ?
		 WHERE id = ?`,
		status, now, failureCount, isActive, id)
	if err != nil {
		return fmt.Errorf("update webhook delivery status: %w", err)
	}
	return nil
}

func scanWebhookRow(rows *sql.Rows) (*webhook.WebhookSubscription, error) {
	var sub webhook.WebhookSubscription
	var secret sql.NullString
	var eventTypesStr string
	var lastStatus sql.NullString
	var lastDeliveryAt sql.NullString
	var createdAt string

	err := rows.Scan(&sub.ID, &sub.UserID, &sub.Name, &sub.URL, &secret,
		&eventTypesStr, &sub.IsActive, &lastStatus, &lastDeliveryAt,
		&sub.FailureCount, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scan webhook row: %w", err)
	}

	populateWebhookFields(&sub, secret, eventTypesStr, lastStatus, lastDeliveryAt, createdAt)
	return &sub, nil
}

func populateWebhookFields(sub *webhook.WebhookSubscription, secret sql.NullString, eventTypesStr string, lastStatus sql.NullString, lastDeliveryAt sql.NullString, createdAt string) {
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
	if lastDeliveryAt.Valid {
		if parsed, err := time.Parse(time.RFC3339, lastDeliveryAt.String); err == nil {
			sub.LastDeliveryAt = &parsed
		}
	}
	if parsed, err := time.Parse(time.RFC3339, createdAt); err == nil {
		sub.CreatedAt = parsed
	}
}
