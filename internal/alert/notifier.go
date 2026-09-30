// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/ssrf"
)

const (
	notifierWorkerCount   = 10
	notifierChannelBuffer = 256
	webhookTimeout        = 10 * time.Second
	maxRetries            = 3
)

var retryBackoffs = []time.Duration{
	1 * time.Second,
	5 * time.Second,
}

// NotificationJob represents a webhook delivery job.
type NotificationJob struct {
	Delivery *NotificationDelivery
	Channel  *NotificationChannel
	Alert    *Alert
	// Body, when non-nil, is the pre-rendered request body sent verbatim
	// (used by webhook subscriptions, which dispatch the raw event payload
	// they already marshalled and signed). When nil, the body is formatted
	// from Alert.
	Body []byte
	// Done, when set, receives the outcome once the last attempt is over.
	Done func(ctx context.Context, err error)
}

// jobAlertID returns the job's alert id for logging, or "" when the job has no
// alert (a pre-rendered Body job).
func jobAlertID(job NotificationJob) string {
	if job.Alert == nil {
		return ""
	}
	return job.Alert.ID
}

// WebhookPayload is the JSON body sent to generic webhook URLs.
type WebhookPayload struct {
	Event     string                 `json:"event"`
	Alert     map[string]interface{} `json:"alert"`
	Timestamp string                 `json:"timestamp"`
}

// Notifier dispatches alert notifications with a bounded worker pool.
type Notifier struct {
	queues       [notifierWorkerCount]chan NotificationJob
	channelStore ChannelStore
	httpClient   *http.Client
	logger       *slog.Logger
	webhook      *webhookSender
	senders      map[string]ChannelSender

	suspendedMu     sync.Mutex
	suspendedLogged map[string]bool
}

// NewNotifier creates a new webhook notifier. Its HTTP client blocks delivery
// to private/internal IPs (SSRF guard) at dial time unless allowPrivate is set
// (dev only, via MAINTENANT_ALLOW_PRIVATE_WEBHOOKS).
func NewNotifier(channelStore ChannelStore, logger *slog.Logger, allowPrivate bool) *Notifier {
	client := ssrf.NewHTTPClient(webhookTimeout, allowPrivate)
	webhook := &webhookSender{client: client, format: formatWebhookPayload, logger: logger}
	n := &Notifier{
		channelStore: channelStore,
		httpClient:   client,
		logger:       logger,
		webhook:      webhook,
		senders: map[string]ChannelSender{
			"webhook": webhook,
			"discord": NewWebhookSender(client, formatDiscordPayload, logger),
		},
		suspendedLogged: make(map[string]bool),
	}
	for i := range n.queues {
		n.queues[i] = make(chan NotificationJob, notifierChannelBuffer)
	}
	return n
}

// HTTPClient returns the SSRF-guarded client the notifier delivers webhooks with.
func (n *Notifier) HTTPClient() *http.Client {
	return n.httpClient
}

// RegisterChannel routes every channel of chType to s; call it before Start.
func (n *Notifier) RegisterChannel(chType string, s ChannelSender) {
	n.senders[chType] = s
}

// senderFor returns the sender of chType, the generic webhook for a type nobody registered.
func (n *Notifier) senderFor(chType string) ChannelSender {
	if s, ok := n.senders[chType]; ok {
		return s
	}
	return n.webhook
}

// Validator returns the validator of chType, if its sender has one.
func (n *Notifier) Validator(chType string) (ChannelValidator, bool) {
	v, ok := n.senders[chType].(ChannelValidator)
	return v, ok
}

// SMTPConfigured reports whether SMTP delivery is available.
func (n *Notifier) SMTPConfigured() bool {
	s, ok := n.senders["email"]
	return ok && s.Ready() == nil
}

// Start begins the worker pool. Call in a goroutine.
func (n *Notifier) Start(ctx context.Context) {
	n.logger.Info("alert notifier: started", "workers", notifierWorkerCount)
	for _, q := range n.queues {
		go n.worker(ctx, q)
	}
}

// Enqueue adds a notification job to the queue of its stream, whose jobs one worker delivers in order.
func (n *Notifier) Enqueue(job NotificationJob) {
	select {
	case n.queues[streamOf(job)] <- job:
	default:
		n.logger.Warn("notifier: job queue full, dropping notification",
			"alert_id", jobAlertID(job), "channel_id", job.Channel.ID)
	}
}

// streamOf picks the queue shared by an alert's notifications to one channel, or by a webhook subscription's
// events, so a recovery never overtakes the alert it resolves.
func streamOf(job NotificationJob) uint32 {
	h := fnv.New32a()
	if job.Alert != nil {
		_, _ = h.Write([]byte(job.Alert.ID + "\x00" + job.Channel.ID))
	} else {
		_, _ = h.Write([]byte(job.Channel.URL))
	}
	return h.Sum32() % notifierWorkerCount
}

func (n *Notifier) worker(ctx context.Context, jobs <-chan NotificationJob) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			n.processJob(ctx, job)
		}
	}
}

func (n *Notifier) processJob(ctx context.Context, job NotificationJob) {
	// Pre-rendered body: webhook subscriptions dispatch the raw event payload
	// (already marshalled and signed), so there is no alert to format from.
	if job.Body != nil {
		n.deliver(ctx, job, n.webhook, func() error { return n.webhook.post(ctx, job.Channel, job.Body) })
		return
	}

	if err := n.suspension(job.Channel); err != nil {
		job.Delivery.Status = DeliverySuspended
		job.Delivery.LastError = err.Error()
		if job.Delivery.ID != "" {
			if updateErr := n.channelStore.UpdateDelivery(ctx, job.Delivery); updateErr != nil {
				n.logger.Error("notifier: update suspended delivery", "error", updateErr)
			}
		}
		return
	}

	eventType := alertEventType(job.Alert)
	sender := n.senderFor(job.Channel.Type)
	if err := sender.Ready(); err != nil {
		n.failDelivery(ctx, job.Delivery, err.Error())
		return
	}
	n.deliver(ctx, job, sender, func() error { return sender.Send(ctx, job.Channel, eventType, job.Alert) })
}

// deliver runs send under the shared retry/backoff policy and records the delivery outcome.
func (n *Notifier) deliver(ctx context.Context, job NotificationJob, sender ChannelSender, send func() error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(sender.RetryDelay(retryBackoffs[attempt-1], lastErr)):
			}
		}

		job.Delivery.Attempts = attempt + 1
		lastErr = send()
		if lastErr == nil {
			job.Delivery.Status = DeliveryDelivered
			if job.Delivery.ID != "" {
				if updateErr := n.channelStore.UpdateDelivery(ctx, job.Delivery); updateErr != nil {
					n.logger.Error("notifier: update delivery status", "error", updateErr)
				}
			}
			n.logger.Debug("alert notifier: delivered",
				"alert_id", jobAlertID(job),
				"channel_id", job.Channel.ID,
				"channel_type", job.Channel.Type,
			)
			if job.Done != nil {
				job.Done(ctx, nil)
			}
			return
		}

		n.logger.Warn("notifier: delivery attempt failed",
			"attempt", attempt+1, "channel_id", job.Channel.ID, "channel_type", job.Channel.Type,
			"alert_id", jobAlertID(job), "error", lastErr)
	}

	n.failDelivery(ctx, job.Delivery, sender.FailureMessage(lastErr))
	if job.Done != nil {
		job.Done(ctx, lastErr)
	}
}

func (n *Notifier) failDelivery(ctx context.Context, d *NotificationDelivery, errMsg string) {
	d.Status = DeliveryFailed
	d.LastError = errMsg
	if d.ID != "" {
		if updateErr := n.channelStore.UpdateDelivery(ctx, d); updateErr != nil {
			n.logger.Error("notifier: update failed delivery", "error", updateErr)
		}
	}
}

// suspension returns a *SuspendedError when the running edition no longer opens ch's type, warning once per channel and transition.
func (n *Notifier) suspension(ch *NotificationChannel) error {
	required, suspended := ChannelSuspension(ch.Type)

	n.suspendedMu.Lock()
	defer n.suspendedMu.Unlock()
	if !suspended {
		delete(n.suspendedLogged, ch.ID)
		return nil
	}
	if !n.suspendedLogged[ch.ID] {
		n.suspendedLogged[ch.ID] = true
		n.logger.Warn("notifier: channel suspended, the running edition no longer opens its type",
			"channel_id", ch.ID, "channel_type", ch.Type, "required_edition", required)
	}
	return &SuspendedError{Required: required}
}

func alertEventType(a *Alert) string {
	if a.Status == StatusResolved {
		return event.AlertResolved
	}
	return event.AlertFired
}

// SendNow performs a synchronous send to the given channel with internal retries.
// Unlike Enqueue, it does not write to the notification_deliveries table — the
// caller (e.g. the escalation Runner) manages its own delivery row state.
// Returns nil on success, or the last error after maxRetries attempts.
func (n *Notifier) SendNow(ctx context.Context, a *Alert, ch *NotificationChannel) error {
	if err := n.suspension(ch); err != nil {
		return err
	}
	eventType := alertEventType(a)
	sender := n.senderFor(ch.Type)
	if err := sender.Ready(); err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sender.RetryDelay(retryBackoffs[attempt-1], lastErr)):
			}
		}
		lastErr = sender.Send(ctx, ch, eventType, a)
		if lastErr == nil {
			return nil
		}
		n.logger.Warn("notifier: attempt failed",
			"attempt", attempt+1, "channel_id", ch.ID, "channel_type", ch.Type, "alert_id", a.ID, "error", lastErr)
	}
	return lastErr
}

// SendBodyNow posts a pre-rendered body to ch once, with the channel's headers, and returns the HTTP status.
func (n *Notifier) SendBodyNow(ctx context.Context, ch *NotificationChannel, body []byte) (int, error) {
	return n.webhook.sendBody(ctx, ch, body)
}

// SendTestWebhook sends a test notification to verify a channel is reachable.
func (n *Notifier) SendTestWebhook(ctx context.Context, ch *NotificationChannel) (int, error) {
	testAlert := &Alert{
		Source:     "test",
		AlertType:  "test",
		Severity:   SeverityInfo,
		Status:     StatusActive,
		Message:    "maintenant test notification",
		EntityType: "test",
		EntityName: "test",
		FiredAt:    time.Now().UTC(),
		CreatedAt:  time.Now().UTC(),
	}

	if err := n.suspension(ch); err != nil {
		return 0, err
	}
	sender := n.senderFor(ch.Type)
	if err := sender.Ready(); err != nil {
		return 0, err
	}
	return sender.SendTest(ctx, ch, testAlert)
}

func formatWebhookPayload(eventType string, a *Alert) ([]byte, error) {
	payload := WebhookPayload{
		Event:     eventType,
		Alert:     alertToMap(a),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	return json.Marshal(payload)
}

func formatDiscordPayload(eventType string, a *Alert) ([]byte, error) {
	color := severityColor(a.Severity)

	fields := []map[string]interface{}{
		{"name": "Source", "value": a.Source, "inline": true},
		{"name": "Severity", "value": a.Severity, "inline": true},
		{"name": "Entity", "value": a.EntityName, "inline": true},
	}

	if a.Source == "update" {
		if details := ParseAlertDetails(a.Details); details != nil {
			if cmd, ok := details["update_command"].(string); ok && cmd != "" {
				fields = append(fields, map[string]interface{}{
					"name": "Update Command", "value": fmt.Sprintf("```%s```", cmd), "inline": false,
				})
			}
			if cmd, ok := details["rollback_command"].(string); ok && cmd != "" {
				fields = append(fields, map[string]interface{}{
					"name": "Rollback Command", "value": fmt.Sprintf("```%s```", cmd), "inline": false,
				})
			}
		}
	}

	embed := map[string]interface{}{
		"title":       EventTitle(eventType, a),
		"description": a.Message,
		"color":       color,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"fields":      fields,
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{embed},
	}
	return json.Marshal(payload)
}

// ParseAlertDetails deserializes the JSON details string from a persisted alert.
func ParseAlertDetails(details string) map[string]interface{} {
	if details == "" || details == "{}" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(details), &m); err != nil {
		return nil
	}
	return m
}

// EventTitle is the headline of a fired, resolved or test notification.
func EventTitle(eventType string, a *Alert) string {
	switch {
	case eventType == "test":
		return "maintenant Test Notification"
	case strings.Contains(eventType, "resolved"):
		return fmt.Sprintf("Resolved: %s", a.EntityName)
	default:
		return fmt.Sprintf("Alert: %s", a.EntityName)
	}
}

// SeverityEmoji is the coloured circle that leads a notification of this severity.
func SeverityEmoji(severity string) string {
	switch severity {
	case SeverityCritical:
		return "\xF0\x9F\x94\xB4" // red circle
	case SeverityWarning:
		return "\xF0\x9F\x9F\xA0" // orange circle
	default:
		return "\xF0\x9F\x9F\xA2" // green circle
	}
}

func severityColor(severity string) int {
	switch severity {
	case SeverityCritical:
		return 0xEF4444 // red
	case SeverityWarning:
		return 0xF59E0B // amber
	default:
		return 0x22C55E // green
	}
}
