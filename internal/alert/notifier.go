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

package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
)

const (
	notifierWorkerCount   = 10
	notifierChannelBuffer = 256
	webhookTimeout        = 10 * time.Second
	maxRetries            = 3
)

// Retry backoff durations: 1s, 5s, 25s.
var retryBackoffs = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	25 * time.Second,
}

// NotificationJob represents a webhook delivery job.
type NotificationJob struct {
	Delivery *NotificationDelivery
	Channel  *NotificationChannel
	Alert    *Alert
}

// WebhookPayload is the JSON body sent to generic webhook URLs.
type WebhookPayload struct {
	Event     string                 `json:"event"`
	Alert     map[string]interface{} `json:"alert"`
	Timestamp string                 `json:"timestamp"`
}

// Notifier dispatches webhook notifications with a bounded worker pool.
type Notifier struct {
	jobs           chan NotificationJob
	channelStore   ChannelStore
	httpClient     *http.Client
	smtpSender     *SMTPSender
	templateEngine TemplateEngine
	logger         *slog.Logger
}

// NewNotifier creates a new webhook notifier.
func NewNotifier(channelStore ChannelStore, logger *slog.Logger) *Notifier {
	return &Notifier{
		jobs:           make(chan NotificationJob, notifierChannelBuffer),
		channelStore:   channelStore,
		templateEngine: noopTemplateEngine{},
		httpClient: &http.Client{
			Timeout: webhookTimeout,
		},
		logger: logger,
	}
}

// SetTemplateEngine sets the template rendering extension.
func (n *Notifier) SetTemplateEngine(t TemplateEngine) {
	n.templateEngine = t
}

// noopTemplateEngine is the Notifier-internal no-op default.
type noopTemplateEngine struct{}

func (noopTemplateEngine) Render(_ context.Context, _ string, _ map[string]any) (string, error) {
	return "", fmt.Errorf("no template engine configured")
}

// SetSMTPSender configures SMTP delivery for email channels.
func (n *Notifier) SetSMTPSender(sender *SMTPSender) {
	n.smtpSender = sender
}

// SMTPConfigured reports whether SMTP delivery is available.
func (n *Notifier) SMTPConfigured() bool {
	return n.smtpSender != nil
}

// Start begins the worker pool. Call in a goroutine.
func (n *Notifier) Start(ctx context.Context) {
	n.logger.Info("alert notifier: started", "workers", notifierWorkerCount)
	for i := 0; i < notifierWorkerCount; i++ {
		go n.worker(ctx)
	}
}

// Enqueue adds a notification job to the work queue.
func (n *Notifier) Enqueue(job NotificationJob) {
	select {
	case n.jobs <- job:
	default:
		n.logger.Warn("notifier: job queue full, dropping notification",
			"alert_id", job.Alert.ID, "channel_id", job.Channel.ID)
	}
}

func (n *Notifier) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-n.jobs:
			if !ok {
				return
			}
			n.processJob(ctx, job)
		}
	}
}

func (n *Notifier) processJob(ctx context.Context, job NotificationJob) {
	eventType := event.AlertFired
	if job.Alert.Status == StatusResolved {
		eventType = event.AlertResolved
	}

	channelType := job.Channel.Type

	// Email channel: use SMTP sender
	if channelType == "email" {
		n.processEmailJob(ctx, job, eventType)
		return
	}

	// Consult template engine extension (Pro: custom templates per channel)
	var body []byte
	rendered, renderErr := n.templateEngine.Render(ctx, channelType, map[string]any{
		"event":       eventType,
		"alert_id":    job.Alert.ID,
		"source":      job.Alert.Source,
		"alert_type":  job.Alert.AlertType,
		"severity":    job.Alert.Severity,
		"status":      job.Alert.Status,
		"message":     job.Alert.Message,
		"entity_type": job.Alert.EntityType,
		"entity_id":   job.Alert.EntityID,
		"entity_name": job.Alert.EntityName,
		"fired_at":    job.Alert.FiredAt,
	})
	if renderErr == nil {
		body = []byte(rendered)
	} else {
		// Fall through to default formatter
		var fmtErr error
		body, fmtErr = formatPayload(channelType, eventType, job.Alert)
		if fmtErr != nil {
			n.failDelivery(ctx, job.Delivery, fmt.Sprintf("marshal payload: %s", fmtErr))
			return
		}
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := retryBackoffs[attempt-1]
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		job.Delivery.Attempts = attempt + 1
		lastErr = n.sendWebhook(ctx, job.Channel, body)
		if lastErr == nil {
			job.Delivery.Status = DeliveryDelivered
			if job.Delivery.ID != 0 {
				if updateErr := n.channelStore.UpdateDelivery(ctx, job.Delivery); updateErr != nil {
					n.logger.Error("notifier: update delivery status", "error", updateErr)
				}
			}
			n.logger.Debug("alert notifier: delivered",
				"alert_id", job.Alert.ID,
				"channel_id", job.Channel.ID,
				"channel_type", channelType,
			)
			return
		}

		n.logger.Warn("notifier: webhook delivery attempt failed",
			"attempt", attempt+1, "channel_id", job.Channel.ID,
			"alert_id", job.Alert.ID, "error", lastErr)
	}

	// All retries exhausted
	n.failDelivery(ctx, job.Delivery, lastErr.Error())
}

func (n *Notifier) processEmailJob(ctx context.Context, job NotificationJob, eventType string) {
	if n.smtpSender == nil {
		n.failDelivery(ctx, job.Delivery, "SMTP not configured")
		return
	}

	to := job.Channel.URL
	subject := formatEmailSubject(eventType, job.Alert)
	body := formatEmailBody(eventType, job.Alert)

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := retryBackoffs[attempt-1]
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		job.Delivery.Attempts = attempt + 1
		err := n.smtpSender.Send(ctx, to, subject, body)
		if err == nil {
			job.Delivery.Status = DeliveryDelivered
			if job.Delivery.ID != 0 {
				if updateErr := n.channelStore.UpdateDelivery(ctx, job.Delivery); updateErr != nil {
					n.logger.Error("notifier: update delivery status", "error", updateErr)
				}
			}
			n.logger.Debug("alert notifier: email delivered",
				"alert_id", job.Alert.ID,
				"channel_id", job.Channel.ID,
			)
			return
		}

		n.logger.Warn("notifier: email delivery attempt failed",
			"attempt", attempt+1, "channel_id", job.Channel.ID,
			"alert_id", job.Alert.ID, "error", err)
	}

	n.failDelivery(ctx, job.Delivery, "email delivery failed after retries")
}

func (n *Notifier) sendWebhook(ctx context.Context, ch *NotificationChannel, body []byte) error {
	n.logger.Debug("alert notifier: sending webhook",
		"url", ch.URL,
		"channel_type", ch.Type,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ch.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Apply custom headers from channel config
	if ch.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(ch.Headers), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("non-2xx response: %d", resp.StatusCode)
	}

	return nil
}

func (n *Notifier) failDelivery(ctx context.Context, d *NotificationDelivery, errMsg string) {
	d.Status = DeliveryFailed
	d.LastError = errMsg
	if d.ID != 0 {
		if updateErr := n.channelStore.UpdateDelivery(ctx, d); updateErr != nil {
			n.logger.Error("notifier: update failed delivery", "error", updateErr)
		}
	}
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

	// Email channel: test via SMTP
	if ch.Type == "email" {
		if n.smtpSender == nil {
			return 0, fmt.Errorf("SMTP not configured")
		}
		err := n.smtpSender.Send(ctx, ch.URL, "maintenant Test Notification", "This is a test notification from maintenant.\n\nIf you received this email, your alert channel is configured correctly.")
		if err != nil {
			return 0, err
		}
		return 200, nil
	}

	body, err := formatPayload(ch.Type, "test", testAlert)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ch.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	if ch.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(ch.Headers), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// formatPayload builds the JSON payload appropriate for the channel type.
func formatPayload(channelType, eventType string, a *Alert) ([]byte, error) {
	switch channelType {
	case "slack":
		return formatSlackPayload(eventType, a)
	case "discord":
		return formatDiscordPayload(eventType, a)
	case "teams":
		return formatTeamsPayload(eventType, a)
	default:
		return formatWebhookPayload(eventType, a)
	}
}

func formatWebhookPayload(eventType string, a *Alert) ([]byte, error) {
	payload := WebhookPayload{
		Event:     eventType,
		Alert:     alertToMap(a),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	return json.Marshal(payload)
}

func formatSlackPayload(eventType string, a *Alert) ([]byte, error) {
	emoji := severityEmoji(a.Severity)
	title := fmt.Sprintf("%s *%s*", emoji, eventTitle(eventType, a))

	fields := fmt.Sprintf(
		"*Source:* %s  |  *Severity:* %s  |  *Entity:* %s\n%s",
		a.Source, a.Severity, a.EntityName, a.Message,
	)

	blocks := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": title,
			},
		},
		{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": fields,
			},
		},
	}

	if a.Source == "update" {
		if details := parseAlertDetails(a.Details); details != nil {
			if cmd, ok := details["update_command"].(string); ok && cmd != "" {
				blocks = append(blocks, map[string]interface{}{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("*Update command:*\n```%s```", cmd),
					},
				})
			}
			if cmd, ok := details["rollback_command"].(string); ok && cmd != "" {
				blocks = append(blocks, map[string]interface{}{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("*Rollback command:*\n```%s```", cmd),
					},
				})
			}
		}
	}

	payload := map[string]interface{}{
		"blocks": blocks,
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
		if details := parseAlertDetails(a.Details); details != nil {
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
		"title":       eventTitle(eventType, a),
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

func formatTeamsPayload(eventType string, a *Alert) ([]byte, error) {
	facts := []map[string]string{
		{"name": "Source", "value": a.Source},
		{"name": "Severity", "value": a.Severity},
		{"name": "Entity", "value": a.EntityName},
		{"name": "Type", "value": a.AlertType},
	}

	if a.Source == "update" {
		if details := parseAlertDetails(a.Details); details != nil {
			if cmd, ok := details["update_command"].(string); ok && cmd != "" {
				facts = append(facts, map[string]string{"name": "Update Command", "value": "`" + cmd + "`"})
			}
			if cmd, ok := details["rollback_command"].(string); ok && cmd != "" {
				facts = append(facts, map[string]string{"name": "Rollback Command", "value": "`" + cmd + "`"})
			}
		}
	}

	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": severityHexColor(a.Severity),
		"title":      eventTitle(eventType, a),
		"sections": []map[string]interface{}{
			{
				"activityTitle": a.Message,
				"facts":         facts,
			},
		},
	}
	return json.Marshal(payload)
}

func formatEmailSubject(eventType string, a *Alert) string {
	prefix := "ALERT"
	if strings.Contains(eventType, "resolved") {
		prefix = "RESOLVED"
	} else if eventType == "test" {
		prefix = "TEST"
	}
	return fmt.Sprintf("[maintenant] %s: %s", prefix, a.Message)
}

func formatEmailBody(eventType string, a *Alert) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Event: %s\n", eventType))
	b.WriteString(fmt.Sprintf("Source: %s\n", a.Source))
	b.WriteString(fmt.Sprintf("Severity: %s\n", a.Severity))
	b.WriteString(fmt.Sprintf("Entity: %s (%s)\n", a.EntityName, a.EntityType))
	b.WriteString(fmt.Sprintf("Message: %s\n", a.Message))
	b.WriteString(fmt.Sprintf("Time: %s\n", a.FiredAt.UTC().Format(time.RFC3339)))

	if a.Source == "update" {
		if details := parseAlertDetails(a.Details); details != nil {
			if cmd, ok := details["update_command"].(string); ok && cmd != "" {
				b.WriteString(fmt.Sprintf("\nUpdate command:\n  %s\n", cmd))
			}
			if cmd, ok := details["rollback_command"].(string); ok && cmd != "" {
				b.WriteString(fmt.Sprintf("\nRollback command:\n  %s\n", cmd))
			}
		}
	}

	return b.String()
}

// parseAlertDetails deserializes the JSON details string from a persisted alert.
func parseAlertDetails(details string) map[string]interface{} {
	if details == "" || details == "{}" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(details), &m); err != nil {
		return nil
	}
	return m
}

func eventTitle(eventType string, a *Alert) string {
	switch {
	case eventType == "test":
		return "maintenant Test Notification"
	case strings.Contains(eventType, "resolved"):
		return fmt.Sprintf("Resolved: %s", a.EntityName)
	default:
		return fmt.Sprintf("Alert: %s", a.EntityName)
	}
}

func severityEmoji(severity string) string {
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

func severityHexColor(severity string) string {
	switch severity {
	case SeverityCritical:
		return "EF4444"
	case SeverityWarning:
		return "F59E0B"
	default:
		return "22C55E"
	}
}
