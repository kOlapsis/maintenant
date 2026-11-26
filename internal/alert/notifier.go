// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/ssrf"
	"github.com/kolapsis/maintenant/internal/uid"
)

const (
	notifierWorkerCount   = 10
	notifierChannelBuffer = 256
	webhookTimeout        = 10 * time.Second
	maxRetries            = 3

	firstRetryDelay = 5 * time.Second
	maxRetryDelay   = time.Hour
	deliveryMaxAge  = 24 * time.Hour
	attemptTimeout  = time.Minute
	claimLease      = 2 * attemptTimeout // longer than an attempt can last, so no second worker takes it meanwhile
	resumeInterval  = 5 * time.Second
	resumeBatch     = 100
	recordTimeout   = 5 * time.Second
	workerIdleAfter = time.Minute
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

	claim time.Time // end of this worker's hold on the durable row
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
	queuesMu     sync.Mutex
	queues       map[string]chan NotificationJob
	runCtx       context.Context
	stopped      bool
	subQueues    [notifierWorkerCount]chan NotificationJob
	channelStore ChannelStore
	httpClient   *http.Client
	logger       *slog.Logger
	webhook      *webhookSender
	senders      map[string]ChannelSender
	baseURL      string

	suspendedMu     sync.Mutex
	suspendedLogged map[string]bool

	queue          DeliveryQueue   // nil when the channel store keeps no durable queue
	unsavedRetries []time.Duration // waits between the in-memory attempts of a job without a durable row
	now            func() time.Time
	resumeEvery    time.Duration
	workerIdle     time.Duration
	running        sync.WaitGroup
}

// NotifierOption configures a Notifier at construction.
type NotifierOption func(*Notifier)

// WithBaseURL sets the public URL of the UI that webhook payloads link each alert to.
func WithBaseURL(baseURL string) NotifierOption {
	return func(n *Notifier) {
		n.baseURL = usableBaseURL(baseURL)
	}
}

// NewNotifier creates a new webhook notifier. Its HTTP client blocks delivery
// to private/internal IPs (SSRF guard) at dial time unless allowPrivate is set
// (dev only, via MAINTENANT_ALLOW_PRIVATE_WEBHOOKS).
func NewNotifier(channelStore ChannelStore, logger *slog.Logger, allowPrivate bool, opts ...NotifierOption) *Notifier {
	client := ssrf.NewHTTPClient(webhookTimeout, allowPrivate)
	client.CheckRedirect = keepBodyRedirects
	queue, _ := channelStore.(DeliveryQueue)
	n := &Notifier{
		channelStore:    channelStore,
		queue:           queue,
		unsavedRetries:  retryBackoffs,
		httpClient:      client,
		logger:          logger,
		suspendedLogged: make(map[string]bool),
		now:             time.Now,
		resumeEvery:     resumeInterval,
		workerIdle:      workerIdleAfter,
		queues:          make(map[string]chan NotificationJob),
	}
	for _, opt := range opts {
		opt(n)
	}
	n.webhook = &webhookSender{client: client, format: n.formatWebhookPayload, logger: logger}
	n.senders = map[string]ChannelSender{
		"webhook": n.webhook,
		"discord": NewWebhookSender(client, formatDiscordPayload, logger),
	}
	for i := range n.subQueues {
		n.subQueues[i] = make(chan NotificationJob, notifierChannelBuffer)
	}
	return n
}

// keepBodyRedirects follows a 307 or 308, which resend the method and the body, and stops on any other redirect.
func keepBodyRedirects(req *http.Request, via []*http.Request) error {
	switch req.Response.StatusCode {
	case http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return http.ErrUseLastResponse
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
	n.logger.Info("alert notifier: started", "subscription_workers", notifierWorkerCount)
	// Held until no worker can be started any more, so that no Add runs at zero against Wait.
	n.running.Add(1)
	go func() {
		defer n.running.Done()
		<-ctx.Done()
		n.queuesMu.Lock()
		n.stopped = true
		n.queuesMu.Unlock()
	}()
	n.queuesMu.Lock()
	n.runCtx = ctx
	for id, q := range n.queues {
		n.startWorker(id, q)
	}
	n.queuesMu.Unlock()
	for i := range n.subQueues {
		n.running.Add(1)
		go n.worker(ctx, n.subQueues[i])
	}
	if n.queue != nil {
		n.running.Add(1)
		go n.resumeLoop(ctx)
	}
}

// Wait blocks until the goroutines of Start have returned, which follows the end of its context, or until ctx ends.
func (n *Notifier) Wait(ctx context.Context) error {
	stopped := make(chan struct{})
	go func() {
		n.running.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Enqueue adds a notification job to the queue of its channel, or of its webhook subscription, whose jobs one worker delivers in order.
func (n *Notifier) Enqueue(job NotificationJob) {
	if n.offer(job) {
		return
	}
	if n.durable(job) {
		n.logger.Info("notifier: job queue full, the delivery stays due in the database",
			"alert_id", jobAlertID(job), "channel_id", job.Channel.ID, "delivery_id", job.Delivery.ID)
		return
	}
	n.logger.Warn("notifier: job queue full, dropping notification",
		"alert_id", jobAlertID(job), "channel_id", job.Channel.ID)
}

func (n *Notifier) offer(job NotificationJob) bool {
	if job.Alert == nil {
		select {
		case n.subQueues[streamOf(job)] <- job:
			return true
		default:
			return false
		}
	}
	n.queuesMu.Lock()
	defer n.queuesMu.Unlock()
	q, ok := n.queues[job.Channel.ID]
	if !ok {
		if n.stopped {
			return false
		}
		q = make(chan NotificationJob, notifierChannelBuffer)
		n.queues[job.Channel.ID] = q
		if n.runCtx != nil {
			n.startWorker(job.Channel.ID, q)
		}
	}
	select {
	case q <- job:
		return true
	default:
		return false
	}
}

// startWorker starts the worker of a channel's queue; call it with queuesMu held, while the notifier runs.
func (n *Notifier) startWorker(channelID string, jobs chan NotificationJob) {
	n.running.Add(1)
	go n.channelWorker(n.runCtx, channelID, jobs)
}

// channelWorker delivers a channel's jobs one at a time, and stops once the channel has had none for workerIdle.
func (n *Notifier) channelWorker(ctx context.Context, channelID string, jobs chan NotificationJob) {
	defer n.running.Done()
	idle := time.NewTimer(n.workerIdle)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-jobs:
			n.handle(ctx, job)
			idle.Reset(n.workerIdle)
		case <-idle.C:
			if n.retire(channelID, jobs) {
				return
			}
			idle.Reset(n.workerIdle)
		}
	}
}

// retire drops an empty queue, so that the channel's next job starts a new worker.
func (n *Notifier) retire(channelID string, jobs chan NotificationJob) bool {
	n.queuesMu.Lock()
	defer n.queuesMu.Unlock()
	if len(jobs) > 0 {
		return false
	}
	delete(n.queues, channelID)
	return true
}

// durable reports whether job's delivery is a row of the durable queue, which outlives the job.
func (n *Notifier) durable(job NotificationJob) bool {
	return n.queue != nil && job.Body == nil && job.Delivery.ID != ""
}

func (n *Notifier) resumeLoop(ctx context.Context) {
	defer n.running.Done()
	ticker := time.NewTicker(n.resumeEvery)
	defer ticker.Stop()
	for {
		n.resume(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// resume hands the due deliveries to the workers of their channels, leaving those of a full queue for a later pass.
func (n *Notifier) resume(ctx context.Context) {
	due, err := n.queue.ListDueDeliveries(ctx, n.now(), resumeBatch)
	if err != nil {
		n.logger.Warn("notifier: list due deliveries", "error", err)
		return
	}
	for _, d := range due {
		ch, err := n.channelStore.GetChannel(ctx, d.ChannelID)
		if err != nil {
			n.logger.Warn("notifier: read the channel of a due delivery", "error", err, "delivery_id", d.ID)
			return
		}
		if ch == nil {
			continue // deleted since the list was read, its deliveries with it
		}
		n.offer(NotificationJob{Delivery: d, Channel: ch, Alert: d.Payload})
	}
}

// streamOf picks the queue shared by a webhook subscription's events, so they reach it in order.
func streamOf(job NotificationJob) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(job.Channel.URL))
	return h.Sum32() % notifierWorkerCount
}

func (n *Notifier) worker(ctx context.Context, jobs <-chan NotificationJob) {
	defer n.running.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			n.handle(ctx, job)
		}
	}
}

// handle takes the due attempt of a durable delivery before running it, so that no attempt is ever sent twice.
func (n *Notifier) handle(ctx context.Context, job NotificationJob) {
	if n.durable(job) {
		taken, err := n.queue.ClaimDelivery(ctx, job.Delivery, n.now().Add(claimLease))
		if err != nil {
			n.logger.Warn("notifier: claim delivery, left for the next resume pass", "error", err, "delivery_id", job.Delivery.ID)
			return
		}
		if !taken {
			return
		}
		job.claim = job.Delivery.NextAttemptAt
		if !job.Channel.Enabled {
			if _, err := n.queue.DropDelivery(ctx, job.Delivery, job.claim); err != nil {
				n.logger.Warn("notifier: drop a delivery of a disabled channel", "error", err, "delivery_id", job.Delivery.ID)
			}
			return
		}
		if reason := n.unsendable(job); reason != "" {
			n.failDelivery(ctx, job, reason)
			return
		}
	}
	n.processJob(ctx, job)
}

// unsendable says why a claimed delivery is closed without being sent, or "" when it can go.
func (n *Notifier) unsendable(job NotificationJob) string {
	switch {
	case job.Alert == nil:
		return "abandoned: notification unreadable"
	case n.now().After(job.Delivery.CreatedAt.Add(deliveryMaxAge)):
		return fmt.Sprintf("abandoned: not delivered within %d hours", int(deliveryMaxAge.Hours()))
	}
	return ""
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
		n.record(ctx, job)
		return
	}

	eventType := sentEvent(job)
	sender := n.senderFor(job.Channel.Type)
	if err := sender.Ready(); err != nil {
		n.failDelivery(ctx, job, err.Error())
		return
	}
	if n.durable(job) {
		n.attempt(ctx, job, sender, eventType)
		return
	}
	n.deliver(ctx, job, sender, func() error { return sender.Send(ctx, job.Channel, eventType, job.Alert) })
}

// sentEvent is the event a channel receives for job, given by its delivery's event type, or by the alert's status when it has none.
func sentEvent(job NotificationJob) string {
	switch job.Delivery.EventType {
	case EventResolved:
		return event.AlertResolved
	case EventFired, EventSeverityRaised, EventReminder:
		return event.AlertFired
	}
	return alertEventType(job.Alert)
}

type deliveryIDKey struct{}

// attempt sends a durable delivery once, then records it delivered, failed, or due again at its next attempt.
func (n *Notifier) attempt(ctx context.Context, job NotificationJob, sender ChannelSender, eventType string) {
	d := job.Delivery
	d.Attempts++
	sendCtx, cancel := context.WithTimeout(context.WithValue(ctx, deliveryIDKey{}, d.ID), attemptTimeout)
	err := sender.Send(sendCtx, job.Channel, eventType, job.Alert)
	cancel()

	if err == nil {
		d.Status, d.LastError = DeliveryDelivered, ""
		if !n.record(ctx, job) {
			return
		}
		n.logger.Debug("alert notifier: delivered",
			"alert_id", jobAlertID(job), "channel_id", job.Channel.ID, "channel_type", job.Channel.Type)
	} else {
		n.logger.Warn("notifier: delivery attempt failed",
			"attempt", d.Attempts, "channel_id", job.Channel.ID, "channel_type", job.Channel.Type,
			"alert_id", jobAlertID(job), "error", err)
		if !isPermanent(err) {
			if next := n.now().Add(sender.RetryDelay(retryDelay(d.Attempts), err)); !next.After(d.CreatedAt.Add(deliveryMaxAge)) {
				d.LastError, d.NextAttemptAt = err.Error(), next
				n.record(ctx, job)
				return
			}
		}
		if !n.failDelivery(ctx, job, sender.FailureMessage(err)) {
			return
		}
	}
	if job.Done != nil {
		job.Done(ctx, err)
	}
}

// retryDelay is the wait after the attempts-th failed attempt: 5 seconds, doubling up to an hour.
func retryDelay(attempts int) time.Duration {
	d := firstRetryDelay
	for i := 1; i < attempts && d < maxRetryDelay; i++ {
		d *= 2
	}
	return min(d, maxRetryDelay)
}

// deliver runs send under the shared retry/backoff policy and records the delivery outcome.
func (n *Notifier) deliver(ctx context.Context, job NotificationJob, sender ChannelSender, send func() error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(sender.RetryDelay(n.unsavedRetries[attempt-1], lastErr)):
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
		if isPermanent(lastErr) {
			break
		}
	}

	n.failDelivery(ctx, job, sender.FailureMessage(lastErr))
	if job.Done != nil {
		job.Done(ctx, lastErr)
	}
}

func (n *Notifier) failDelivery(ctx context.Context, job NotificationJob, errMsg string) bool {
	job.Delivery.Status = DeliveryFailed
	job.Delivery.LastError = errMsg
	return n.record(ctx, job)
}

// record writes the outcome of job's delivery, for a durable row only while this worker's claim on it still holds.
func (n *Notifier) record(ctx context.Context, job NotificationJob) bool {
	d := job.Delivery
	if !n.durable(job) {
		if d.ID != "" {
			if err := n.channelStore.UpdateDelivery(ctx, d); err != nil {
				n.logger.Error("notifier: record delivery", "error", err)
			}
		}
		return true
	}
	// An outcome is still recorded once ctx ends, when the process stops right after a send.
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	var held bool
	err := withRetry(wctx, func() (err error) {
		held, err = n.queue.SettleDelivery(wctx, d, job.claim)
		return err
	})
	if err != nil {
		n.logger.Error("notifier: record delivery, it is due again once its claim runs out", "error", err, "delivery_id", d.ID)
		return false
	}
	if !held {
		n.logger.Info("notifier: delivery claimed by another worker meanwhile, outcome left to it", "delivery_id", d.ID)
	}
	return held
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
	return n.SendEventNow(ctx, alertEventType(a), a, ch)
}

// SendEventNow is SendNow for a caller that names the event the channel receives.
func (n *Notifier) SendEventNow(ctx context.Context, eventType string, a *Alert, ch *NotificationChannel) error {
	if err := n.suspension(ch); err != nil {
		return err
	}
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
		if isPermanent(lastErr) {
			break
		}
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
		AgentID:    uid.LocalAgent,
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

func (n *Notifier) formatWebhookPayload(eventType string, a *Alert) ([]byte, error) {
	m := alertToMap(a)
	if n.baseURL != "" && a.ID != "" {
		m["url"] = n.baseURL + "/alerts/history?alert=" + url.QueryEscape(a.ID)
	}
	payload := WebhookPayload{
		Event:     eventType,
		Alert:     m,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	return json.Marshal(payload)
}

// usableBaseURL returns baseURL without its trailing slash, or "" when it names no host a recipient could open.
func usableBaseURL(baseURL string) string {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return ""
	}
	return u.String()
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

// EventTitle is the headline of a fired, acknowledged, resolved or test notification.
func EventTitle(eventType string, a *Alert) string {
	switch {
	case eventType == "test":
		return "maintenant Test Notification"
	case eventType == event.AlertAcknowledged:
		return fmt.Sprintf("Acknowledged: %s", a.EntityName)
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
