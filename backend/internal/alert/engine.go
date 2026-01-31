package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const engineChannelBuffer = 256

// activeAlertKey uniquely identifies an active alert for dedup and recovery linking.
type activeAlertKey struct {
	Source     string
	AlertType string
	EntityType string
	EntityID   int64
}

// SSEBroadcaster is the interface for broadcasting SSE events.
type SSEBroadcaster interface {
	Broadcast(eventType string, data interface{})
}

// Engine consumes alert events from monitoring services, persists them,
// evaluates silence rules, dispatches notifications, and broadcasts via SSE.
type Engine struct {
	eventCh      chan Event
	alertStore   AlertStore
	channelStore ChannelStore
	silenceStore SilenceStore
	notifier     *Notifier
	broadcaster  SSEBroadcaster
	logger       *slog.Logger

	// In-memory active alert map for recovery linking and dedup
	activeAlerts map[activeAlertKey]*Alert
	mu           sync.RWMutex
}

// NewEngine creates a new alert engine.
func NewEngine(alertStore AlertStore, channelStore ChannelStore, silenceStore SilenceStore, logger *slog.Logger) *Engine {
	return &Engine{
		eventCh:      make(chan Event, engineChannelBuffer),
		alertStore:   alertStore,
		channelStore: channelStore,
		silenceStore: silenceStore,
		logger:       logger,
		activeAlerts: make(map[activeAlertKey]*Alert),
	}
}

// SetNotifier sets the webhook notifier for dispatching notifications.
func (e *Engine) SetNotifier(n *Notifier) {
	e.notifier = n
}

// SetBroadcaster sets the SSE broadcaster.
func (e *Engine) SetBroadcaster(b SSEBroadcaster) {
	e.broadcaster = b
}

// EventChannel returns the channel for sending alert events to the engine.
func (e *Engine) EventChannel() chan<- Event {
	return e.eventCh
}

// Start begins the engine's event processing loop. Call this in a goroutine.
func (e *Engine) Start(ctx context.Context) {
	// Reload active alerts from DB on startup
	if err := e.reloadActiveAlerts(ctx); err != nil {
		e.logger.Error("alert engine: failed to reload active alerts", "error", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-e.eventCh:
				if !ok {
					return
				}
				e.processEvent(ctx, evt)
			}
		}
	}()
}

func (e *Engine) reloadActiveAlerts(ctx context.Context) error {
	active, err := e.alertStore.ListActiveAlerts(ctx)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, a := range active {
		key := activeAlertKey{
			Source:     a.Source,
			AlertType: a.AlertType,
			EntityType: a.EntityType,
			EntityID:   a.EntityID,
		}
		e.activeAlerts[key] = a
	}

	e.logger.Info("alert engine: reloaded active alerts", "count", len(active))
	return nil
}

func (e *Engine) processEvent(ctx context.Context, evt Event) {
	if evt.IsRecover {
		e.processRecovery(ctx, evt)
		return
	}

	key := activeAlertKey{
		Source:     evt.Source,
		AlertType: evt.AlertType,
		EntityType: evt.EntityType,
		EntityID:   evt.EntityID,
	}

	// Dedup: skip if there's already an active alert for this key
	e.mu.RLock()
	if _, exists := e.activeAlerts[key]; exists {
		e.mu.RUnlock()
		return
	}
	e.mu.RUnlock()

	// Serialize details to JSON
	detailsJSON := "{}"
	if evt.Details != nil {
		if b, err := json.Marshal(evt.Details); err == nil {
			detailsJSON = string(b)
		}
	}

	a := &Alert{
		Source:     evt.Source,
		AlertType: evt.AlertType,
		Severity:  evt.Severity,
		Status:    StatusActive,
		Message:   evt.Message,
		EntityType: evt.EntityType,
		EntityID:   evt.EntityID,
		EntityName: evt.EntityName,
		Details:    detailsJSON,
		FiredAt:    evt.Timestamp,
	}

	// Check silence rules before persisting
	silenced := e.checkSilenceRules(ctx, evt)
	if silenced {
		a.Status = StatusSilenced
	}

	// Persist the alert
	id, err := e.alertStore.InsertAlert(ctx, a)
	if err != nil {
		e.logger.Error("alert engine: persist alert", "error", err)
		return
	}
	a.ID = id

	// Track active alert (only if not silenced)
	if !silenced {
		e.mu.Lock()
		e.activeAlerts[key] = a
		e.mu.Unlock()
	}

	// SSE broadcast
	e.broadcastAlert(a, silenced)

	// Dispatch notifications (only if not silenced)
	if !silenced && e.notifier != nil {
		e.dispatchNotifications(ctx, a)
	}
}

func (e *Engine) processRecovery(ctx context.Context, evt Event) {
	key := activeAlertKey{
		Source:     evt.Source,
		AlertType: evt.AlertType,
		EntityType: evt.EntityType,
		EntityID:   evt.EntityID,
	}

	// Find the active alert to resolve
	e.mu.Lock()
	activeAlert, exists := e.activeAlerts[key]
	if exists {
		delete(e.activeAlerts, key)
	}
	e.mu.Unlock()

	if !exists {
		// No active alert to resolve — recovery without prior alert
		return
	}

	// Create recovery alert record
	detailsJSON := "{}"
	if evt.Details != nil {
		if b, err := json.Marshal(evt.Details); err == nil {
			detailsJSON = string(b)
		}
	}

	recoveryAlert := &Alert{
		Source:     evt.Source,
		AlertType: evt.AlertType,
		Severity:  evt.Severity,
		Status:    StatusResolved,
		Message:   evt.Message,
		EntityType: evt.EntityType,
		EntityID:   evt.EntityID,
		EntityName: evt.EntityName,
		Details:    detailsJSON,
		FiredAt:    evt.Timestamp,
	}

	recoveryID, err := e.alertStore.InsertAlert(ctx, recoveryAlert)
	if err != nil {
		e.logger.Error("alert engine: persist recovery", "error", err)
		return
	}
	recoveryAlert.ID = recoveryID

	// Resolve the original alert
	now := evt.Timestamp
	if err := e.alertStore.UpdateAlertStatus(ctx, activeAlert.ID, StatusResolved, &now, &recoveryID); err != nil {
		e.logger.Error("alert engine: resolve original alert", "error", err)
	}

	// Update the resolved alert fields for broadcasting
	activeAlert.Status = StatusResolved
	activeAlert.ResolvedAt = &now
	activeAlert.ResolvedByID = &recoveryID

	// SSE broadcast
	e.broadcastResolved(activeAlert)

	// Dispatch recovery notifications
	if e.notifier != nil {
		e.dispatchNotifications(ctx, activeAlert)
	}
}

func (e *Engine) checkSilenceRules(ctx context.Context, evt Event) bool {
	if e.silenceStore == nil {
		return false
	}

	rules, err := e.silenceStore.GetActiveSilenceRules(ctx)
	if err != nil {
		e.logger.Error("alert engine: get silence rules", "error", err)
		return false
	}

	for _, rule := range rules {
		if matchesSilenceRule(rule, evt) {
			return true
		}
	}
	return false
}

func matchesSilenceRule(rule *SilenceRule, evt Event) bool {
	// Global silence (no filters)
	if rule.EntityType == "" && rule.Source == "" && rule.EntityID == nil {
		return true
	}

	// Source filter
	if rule.Source != "" && rule.Source != evt.Source {
		return false
	}

	// Entity type filter
	if rule.EntityType != "" && rule.EntityType != evt.EntityType {
		return false
	}

	// Entity ID filter
	if rule.EntityID != nil && *rule.EntityID != evt.EntityID {
		return false
	}

	return true
}

func (e *Engine) dispatchNotifications(ctx context.Context, a *Alert) {
	if e.channelStore == nil || e.notifier == nil {
		return
	}

	channels, err := e.channelStore.ListChannels(ctx)
	if err != nil {
		e.logger.Error("alert engine: list channels", "error", err)
		return
	}

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}

		// Evaluate routing rules
		if !matchesRoutingRules(ch, a) {
			continue
		}

		// Create delivery record
		delivery := &NotificationDelivery{
			AlertID:   a.ID,
			ChannelID: ch.ID,
			Status:    DeliveryPending,
		}
		deliveryID, err := e.channelStore.InsertDelivery(ctx, delivery)
		if err != nil {
			e.logger.Error("alert engine: create delivery", "error", err, "channel_id", ch.ID)
			continue
		}
		delivery.ID = deliveryID

		// Enqueue to notifier
		e.notifier.Enqueue(NotificationJob{
			Delivery: delivery,
			Channel:  ch,
			Alert:    a,
		})
	}
}

func matchesRoutingRules(ch *NotificationChannel, a *Alert) bool {
	if len(ch.RoutingRules) == 0 {
		// No rules = receive everything
		return true
	}

	for _, rule := range ch.RoutingRules {
		if matchesRule(rule, a) {
			return true
		}
	}
	return false
}

func matchesRule(rule RoutingRule, a *Alert) bool {
	sourceMatch := rule.SourceFilter == "" || containsCSV(rule.SourceFilter, a.Source)
	severityMatch := rule.SeverityFilter == "" || containsCSV(rule.SeverityFilter, a.Severity)
	return sourceMatch && severityMatch
}

func containsCSV(csv, value string) bool {
	for _, item := range splitCSV(csv) {
		if item == value {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			item := trimSpace(s[start:i])
			if item != "" {
				result = append(result, item)
			}
			start = i + 1
		}
	}
	item := trimSpace(s[start:])
	if item != "" {
		result = append(result, item)
	}
	return result
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}

func (e *Engine) broadcastAlert(a *Alert, silenced bool) {
	if e.broadcaster == nil {
		return
	}

	eventType := "alert.fired"
	if silenced {
		eventType = "alert.silenced"
	}

	e.broadcaster.Broadcast(eventType, alertToMap(a))
}

func (e *Engine) broadcastResolved(a *Alert) {
	if e.broadcaster == nil {
		return
	}
	e.broadcaster.Broadcast("alert.resolved", alertToMap(a))
}

func alertToMap(a *Alert) map[string]interface{} {
	m := map[string]interface{}{
		"id":          a.ID,
		"source":      a.Source,
		"alert_type":  a.AlertType,
		"severity":    a.Severity,
		"status":      a.Status,
		"message":     a.Message,
		"entity_type": a.EntityType,
		"entity_id":   a.EntityID,
		"entity_name": a.EntityName,
		"fired_at":    a.FiredAt.UTC().Format(time.RFC3339),
		"created_at":  a.CreatedAt.UTC().Format(time.RFC3339),
	}

	if a.Details != "" && a.Details != "{}" {
		var details map[string]interface{}
		if err := json.Unmarshal([]byte(a.Details), &details); err == nil {
			m["details"] = details
		} else {
			m["details"] = a.Details
		}
	}

	if a.ResolvedByID != nil {
		m["resolved_by_id"] = *a.ResolvedByID
	}
	if a.ResolvedAt != nil {
		m["resolved_at"] = a.ResolvedAt.UTC().Format(time.RFC3339)
	}

	return m
}

// AlertCount returns the number of active alerts (for monitoring).
func (e *Engine) AlertCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.activeAlerts)
}

// sseBroadcasterAdapter adapts the SSEBroker to the SSEBroadcaster interface.
type sseBroadcasterAdapter struct {
	broadcast func(eventType string, data interface{})
}

func (a *sseBroadcasterAdapter) Broadcast(eventType string, data interface{}) {
	a.broadcast(eventType, data)
}

// NewSSEBroadcasterFunc creates an SSEBroadcaster from a function.
func NewSSEBroadcasterFunc(fn func(eventType string, data interface{})) SSEBroadcaster {
	return &sseBroadcasterAdapter{broadcast: fn}
}

// Ensure AlertStoreImpl type assertion works (compile-time check will be in store package).
var _ fmt.Stringer = (*activeAlertKey)(nil) // removed — not needed

func (k activeAlertKey) String() string {
	return fmt.Sprintf("%s/%s/%s/%d", k.Source, k.AlertType, k.EntityType, k.EntityID)
}
