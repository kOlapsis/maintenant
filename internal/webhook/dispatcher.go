// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

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
// Called from main.go when SSE events are broadcast.
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

		// Build webhook payload
		payload := WebhookEvent{
			Type:      webhookEventType,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Data:      data,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			d.logger.Error("webhook dispatcher: marshal payload", "error", err)
			continue
		}

		// Build custom headers
		deliveryID := uuid.New().String()
		headers := map[string]string{
			"X-maintenant-Event":    webhookEventType,
			"X-maintenant-Delivery": deliveryID,
		}

		// HMAC signature if secret is set
		if sub.Secret != "" {
			mac := hmac.New(sha256.New, []byte(sub.Secret))
			mac.Write(payloadBytes)
			sig := hex.EncodeToString(mac.Sum(nil))
			headers["X-maintenant-Signature"] = "sha256=" + sig
		}

		headersJSON, _ := json.Marshal(headers)

		// Create a synthetic NotificationChannel to reuse the notifier
		ch := &alert.NotificationChannel{
			Name:    sub.Name,
			Type:    "webhook",
			URL:     sub.URL,
			Headers: string(headersJSON),
			Enabled: true,
		}

		delivery := &alert.NotificationDelivery{
			Status: alert.DeliveryPending,
		}

		syntheticAlert := &alert.Alert{
			Source:    "webhook",
			AlertType: webhookEventType,
			Severity:  alert.SeverityInfo,
			Status:    alert.StatusActive,
			Message:   webhookEventType,
		}

		d.notifier.Enqueue(alert.NotificationJob{
			Delivery: delivery,
			Channel:  ch,
			Alert:    syntheticAlert,
		})
	}
}

// mapSSETypeToWebhookEvent maps internal SSE event types to webhook event types.
func mapSSETypeToWebhookEvent(sseType string) string {
	switch {
	case sseType == event.ContainerStateChanged || sseType == event.ContainerDiscovered || sseType == "container.removed":
		return event.ContainerStateChanged
	case sseType == event.EndpointStatusChanged || sseType == event.EndpointDiscovered || sseType == event.EndpointRemoved:
		return event.EndpointStatusChanged
	case sseType == event.HeartbeatStatusChanged:
		return event.HeartbeatStatusChanged
	case sseType == event.CertificateStatusChanged:
		return event.CertificateStatusChanged
	case sseType == event.AlertFired:
		return event.AlertFired
	case sseType == event.AlertResolved:
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
