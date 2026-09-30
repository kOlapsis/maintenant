// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

const testEventType = "test"

// Dispatcher subscribes to SSE events and fans out to webhook subscriptions
// using the existing alert.Notifier worker pool.
type Dispatcher struct {
	store    WebhookSubscriptionStore
	notifier *alert.Notifier
	logger   *slog.Logger
}

// NewDispatcher creates a new webhook dispatcher.
func NewDispatcher(store WebhookSubscriptionStore, notifier *alert.Notifier, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		store:    store,
		notifier: notifier,
		logger:   logger,
	}
}

// HandleEvent processes a single SSE event and dispatches to matching webhooks.
func (d *Dispatcher) HandleEvent(ctx context.Context, eventType string, data interface{}) {
	webhookEventType := mapSSETypeToWebhookEvent(eventType)
	if webhookEventType == "" {
		return
	}

	subs, err := d.store.ListActive(ctx)
	if err != nil {
		d.logger.Error("webhook dispatcher: list active subscriptions", "error", err)
		return
	}

	for _, sub := range subs {
		if !matchesEventTypes(sub.EventTypes, webhookEventType) {
			continue
		}
		ch, body, err := signedDelivery(sub, webhookEventType, data)
		if err != nil {
			d.logger.Error("webhook dispatcher: build delivery", "webhook_id", sub.ID, "error", err)
			continue
		}

		id := sub.ID
		d.notifier.Enqueue(alert.NotificationJob{
			Delivery: &alert.NotificationDelivery{Status: alert.DeliveryPending},
			Channel:  ch,
			Body:     body,
			Done:     func(ctx context.Context, err error) { d.recordDelivery(ctx, id, err) },
		})
	}
}

// Test sends sub a test event with the headers and signature of a real delivery and records the outcome like one.
func (d *Dispatcher) Test(ctx context.Context, sub *WebhookSubscription) (int, error) {
	ch, body, err := signedDelivery(sub, testEventType, map[string]interface{}{"message": "maintenant webhook test"})
	if err != nil {
		return 0, err
	}
	status, err := d.notifier.SendBodyNow(ctx, ch, body)
	d.recordDelivery(ctx, sub.ID, err)
	return status, err
}

func (d *Dispatcher) recordDelivery(ctx context.Context, id string, deliveryErr error) {
	if err := d.store.RecordDelivery(ctx, id, deliveryErr == nil); err != nil {
		d.logger.Error("webhook dispatcher: record delivery", "webhook_id", id, "error", err)
	}
}

// signedDelivery builds the channel and body of one event sent to sub, signed with its secret when it has one.
func signedDelivery(sub *WebhookSubscription, eventType string, data interface{}) (*alert.NotificationChannel, []byte, error) {
	body, err := json.Marshal(WebhookEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      data,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal payload: %w", err)
	}

	headers := map[string]string{
		"X-maintenant-Event":    eventType,
		"X-maintenant-Delivery": uuid.New().String(),
	}
	if sub.Secret != "" {
		mac := hmac.New(sha256.New, []byte(sub.Secret))
		mac.Write(body)
		headers["X-maintenant-Signature"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal headers: %w", err)
	}

	return &alert.NotificationChannel{
		Name:    sub.Name,
		Type:    "webhook",
		URL:     sub.URL,
		Headers: string(headersJSON),
		Enabled: true,
	}, body, nil
}

// mapSSETypeToWebhookEvent maps internal SSE event types to webhook event types.
func mapSSETypeToWebhookEvent(sseType string) string {
	switch sseType {
	case event.ContainerStateChanged, event.ContainerDiscovered, "container.removed":
		return event.ContainerStateChanged
	case event.EndpointStatusChanged, event.EndpointDiscovered, event.EndpointRemoved:
		return event.EndpointStatusChanged
	case event.HeartbeatStatusChanged:
		return event.HeartbeatStatusChanged
	case event.CertificateStatusChanged:
		return event.CertificateStatusChanged
	case event.AlertFired:
		return event.AlertFired
	case event.AlertResolved:
		return event.AlertResolved
	default:
		return ""
	}
}

func matchesEventTypes(subscribed []string, eventType string) bool {
	for _, t := range subscribed {
		if t == "*" || t == eventType {
			return true
		}
	}
	return false
}
