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
	"github.com/kolapsis/pulseboard/internal/alert"
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
			"X-PulseBoard-Event":    webhookEventType,
			"X-PulseBoard-Delivery": deliveryID,
		}

		// HMAC signature if secret is set
		if sub.Secret != "" {
			mac := hmac.New(sha256.New, []byte(sub.Secret))
			mac.Write(payloadBytes)
			sig := hex.EncodeToString(mac.Sum(nil))
			headers["X-PulseBoard-Signature"] = "sha256=" + sig
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
	case sseType == "container.state_changed" || sseType == "container.discovered" || sseType == "container.removed":
		return EventContainerStateChanged
	case sseType == "endpoint.status_changed" || sseType == "endpoint.discovered" || sseType == "endpoint.removed":
		return EventEndpointStatusChanged
	case sseType == "heartbeat.status_changed":
		return EventHeartbeatStatusChanged
	case sseType == "certificate.status_changed":
		return EventCertificateStatusChanged
	case sseType == "alert.fired":
		return EventAlertFired
	case sseType == "alert.resolved":
		return EventAlertResolved
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
