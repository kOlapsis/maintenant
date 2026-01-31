package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	notifierWorkerCount  = 10
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

// WebhookPayload is the JSON body sent to webhook URLs.
type WebhookPayload struct {
	Event     string                 `json:"event"`
	Alert     map[string]interface{} `json:"alert"`
	Timestamp string                 `json:"timestamp"`
}

// Notifier dispatches webhook notifications with a bounded worker pool.
type Notifier struct {
	jobs         chan NotificationJob
	channelStore ChannelStore
	httpClient   *http.Client
	logger       *slog.Logger
}

// NewNotifier creates a new webhook notifier.
func NewNotifier(channelStore ChannelStore, logger *slog.Logger) *Notifier {
	return &Notifier{
		jobs:         make(chan NotificationJob, notifierChannelBuffer),
		channelStore: channelStore,
		httpClient: &http.Client{
			Timeout: webhookTimeout,
		},
		logger: logger,
	}
}

// Start begins the worker pool. Call in a goroutine.
func (n *Notifier) Start(ctx context.Context) {
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
	eventType := "alert.fired"
	if job.Alert.Status == StatusResolved {
		eventType = "alert.resolved"
	}

	payload := WebhookPayload{
		Event:     eventType,
		Alert:     alertToMap(job.Alert),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		n.failDelivery(ctx, job.Delivery, fmt.Sprintf("marshal payload: %s", err))
		return
	}

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
		err = n.sendWebhook(ctx, job.Channel, body)
		if err == nil {
			// Success
			job.Delivery.Status = DeliveryDelivered
			if updateErr := n.channelStore.UpdateDelivery(ctx, job.Delivery); updateErr != nil {
				n.logger.Error("notifier: update delivery status", "error", updateErr)
			}
			return
		}

		n.logger.Warn("notifier: webhook delivery attempt failed",
			"attempt", attempt+1, "channel_id", job.Channel.ID,
			"alert_id", job.Alert.ID, "error", err)
	}

	// All retries exhausted
	n.failDelivery(ctx, job.Delivery, err.Error())
}

func (n *Notifier) sendWebhook(ctx context.Context, ch *NotificationChannel, body []byte) error {
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
	if updateErr := n.channelStore.UpdateDelivery(ctx, d); updateErr != nil {
		n.logger.Error("notifier: update failed delivery", "error", updateErr)
	}
}

// SendTestWebhook sends a test webhook to verify a channel URL is reachable.
func (n *Notifier) SendTestWebhook(ctx context.Context, ch *NotificationChannel) (int, error) {
	payload := WebhookPayload{
		Event: "test",
		Alert: map[string]interface{}{
			"message": "PulseBoard webhook test",
			"source":  "test",
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
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
