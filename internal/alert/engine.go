// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/uid"
)

const engineChannelBuffer = 256

// activeAlertKey uniquely identifies an active alert for dedup and recovery linking.
type activeAlertKey struct {
	Source     string
	AlertType  string
	EntityType string
	EntityID   string
}

// SSEBroadcaster is the interface for broadcasting SSE events.
type SSEBroadcaster interface {
	Broadcast(eventType string, data interface{})
}

// EngineDeps holds all dependencies for the alert Engine.
type EngineDeps struct {
	AlertStore   AlertStore     // required
	ChannelStore ChannelStore   // required
	TriggerStore TriggerStore   // required
	SilenceStore SilenceStore   // required
	Logger       *slog.Logger   // required
	Notifier     *Notifier      // optional — nil-safe
	Broadcaster  SSEBroadcaster // optional — nil-safe
}

// Engine consumes alert events from monitoring services, persists them,
// evaluates silence rules, dispatches notifications, and broadcasts via SSE.
type Engine struct {
	eventCh      chan Event
	alertStore   AlertStore
	channelStore ChannelStore
	triggerStore TriggerStore
	silenceStore SilenceStore
	notifier     *Notifier
	broadcaster  SSEBroadcaster
	logger       *slog.Logger

	// Extension points (Pro injects real implementations; CE uses no-ops)
	escalator  Escalator
	suppressor MaintenanceSuppressor

	// In-memory active alert map for recovery linking and dedup
	activeAlerts map[activeAlertKey]*Alert
	silenced     map[activeAlertKey]*Alert // held back by a silence or a maintenance window, guarded by procMu alone
	recheckEvery time.Duration
	mu           sync.RWMutex
	procMu       sync.Mutex // serializes a store write with the map update that follows it
}

const (
	defaultSilenceRecheckInterval = 30 * time.Second
	silencedReloadMaxAge          = 24 * time.Hour
)

var storeRetryDelays = []time.Duration{100 * time.Millisecond, 500 * time.Millisecond}

func withRetry(ctx context.Context, op func() error) error {
	err := op()
	for _, d := range storeRetryDelays {
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(d):
		}
		err = op()
	}
	return err
}

// NewEngine creates a new alert engine.
func NewEngine(d EngineDeps) *Engine {
	if d.AlertStore == nil {
		panic("alert.NewEngine: AlertStore is required")
	}
	if d.ChannelStore == nil {
		panic("alert.NewEngine: ChannelStore is required")
	}
	if d.TriggerStore == nil {
		panic("alert.NewEngine: TriggerStore is required")
	}
	if d.SilenceStore == nil {
		panic("alert.NewEngine: SilenceStore is required")
	}
	if d.Logger == nil {
		panic("alert.NewEngine: Logger is required")
	}
	return &Engine{
		eventCh:      make(chan Event, engineChannelBuffer),
		alertStore:   d.AlertStore,
		channelStore: d.ChannelStore,
		triggerStore: d.TriggerStore,
		silenceStore: d.SilenceStore,
		notifier:     d.Notifier,
		broadcaster:  d.Broadcaster,
		logger:       d.Logger,
		escalator:    noopEscalator{},
		suppressor:   noopSuppressor{},
		activeAlerts: make(map[activeAlertKey]*Alert),
		silenced:     make(map[activeAlertKey]*Alert),
		recheckEvery: defaultSilenceRecheckInterval,
	}
}

// SetEscalator sets the escalation extension.
func (e *Engine) SetEscalator(esc Escalator) {
	e.escalator = esc
}

// SetMaintenanceSuppressor sets the maintenance suppression extension.
func (e *Engine) SetMaintenanceSuppressor(s MaintenanceSuppressor) {
	e.suppressor = s
}

// noopEscalator is the Engine-internal no-op default.
type noopEscalator struct{}

func (noopEscalator) EvaluateCycle(_ context.Context) error            { return nil }
func (noopEscalator) OnAlertCreated(_ context.Context, _ *Alert) error { return nil }
func (noopEscalator) OnAlertAcknowledged(_ context.Context, _ string, _ Acknowledgment) error {
	return nil
}
func (noopEscalator) OnAlertResolved(_ context.Context, _ string, _ time.Time) error { return nil }
func (noopEscalator) OnEditionDowngraded(_ context.Context) error                    { return nil }

// noopSuppressor is the Engine-internal no-op default.
type noopSuppressor struct{}

func (noopSuppressor) IsSuppressed(_ context.Context, _ string, _ string, _ string) (bool, error) {
	return false, nil
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

const defaultEscalationInterval = 60 * time.Second

// Start begins the engine's event processing loop. Call this in a goroutine.
func (e *Engine) Start(ctx context.Context) {
	// Reload active alerts from DB on startup
	if err := e.reloadActiveAlerts(ctx); err != nil {
		e.logger.Error("alert engine: failed to reload active alerts", "error", err)
	}
	if err := e.reloadSilencedAlerts(ctx); err != nil {
		e.logger.Error("alert engine: failed to reload silenced alerts", "error", err)
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

	go e.runSilenceRecheck(ctx)

	// Start escalation evaluator only when a real Escalator is injected
	if _, isNoop := e.escalator.(noopEscalator); !isNoop {
		go e.runEscalationEvaluator(ctx)
	}
}

func (e *Engine) runEscalationEvaluator(ctx context.Context) {
	ticker := time.NewTicker(defaultEscalationInterval)
	defer ticker.Stop()

	e.logger.Info("alert engine: escalation evaluator started", "interval", defaultEscalationInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.escalator.EvaluateCycle(ctx); err != nil {
				e.logger.ErrorContext(ctx, "alert engine: escalation cycle error", "error", err)
			}
		}
	}
}

func (e *Engine) reloadActiveAlerts(ctx context.Context) error {
	var active []*Alert
	if err := withRetry(ctx, func() (err error) {
		active, err = e.alertStore.ListActiveAlerts(ctx)
		return err
	}); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, a := range active {
		key := activeAlertKey{
			Source:     a.Source,
			AlertType:  a.AlertType,
			EntityType: a.EntityType,
			EntityID:   a.EntityID,
		}
		e.activeAlerts[key] = a
	}

	e.logger.Info("alert engine: reloaded active alerts", "count", len(active))
	return nil
}

// reloadSilencedAlerts holds back again the recent silenced alerts that a silence or a maintenance window still covers.
func (e *Engine) reloadSilencedAlerts(ctx context.Context) error {
	var held []*Alert
	var rules []*SilenceRule
	if err := withRetry(ctx, func() (err error) {
		if held, err = e.alertStore.ListSilencedAlertsSince(ctx, time.Now().Add(-silencedReloadMaxAge)); err != nil {
			return err
		}
		rules, err = e.silenceStore.GetActiveSilenceRules(ctx)
		return err
	}); err != nil {
		return err
	}

	e.procMu.Lock()
	defer e.procMu.Unlock()
	seen := make(map[activeAlertKey]bool)
	for _, a := range held {
		key := keyOf(a)
		if seen[key] {
			continue
		}
		seen[key] = true
		e.mu.RLock()
		_, active := e.activeAlerts[key]
		e.mu.RUnlock()
		if active {
			continue
		}
		if covered, err := e.covered(ctx, rules, eventOf(a)); err == nil && !covered {
			continue
		}
		e.silenced[key] = a
	}

	e.logger.Info("alert engine: reloaded silenced alerts", "count", len(e.silenced))
	return nil
}

func (e *Engine) runSilenceRecheck(ctx context.Context) {
	ticker := time.NewTicker(e.recheckEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.promoteUncovered(ctx)
		}
	}
}

// promoteUncovered promotes every held alert that no silence or maintenance window covers anymore.
func (e *Engine) promoteUncovered(ctx context.Context) {
	e.procMu.Lock()
	defer e.procMu.Unlock()
	if len(e.silenced) == 0 {
		return
	}
	rules, err := e.silenceStore.GetActiveSilenceRules(ctx)
	if err != nil {
		e.logger.Error("alert engine: get silence rules", "error", err)
		return
	}
	for key, a := range e.silenced {
		covered, err := e.covered(ctx, rules, eventOf(a))
		if err != nil {
			e.logger.Error("alert engine: maintenance suppressor error, alert kept silenced", "error", err, "alert_id", a.ID)
			continue
		}
		if !covered {
			e.promote(ctx, key, a)
		}
	}
}

// promote makes a held alert live, exactly as if it had just fired.
func (e *Engine) promote(ctx context.Context, key activeAlertKey, a *Alert) {
	if err := withRetry(ctx, func() error {
		return e.alertStore.UpdateAlertStatus(ctx, a.ID, StatusActive, nil, nil)
	}); err != nil {
		e.logger.Error("alert engine: promote silenced alert", "error", err, "alert_id", a.ID)
		return
	}
	delete(e.silenced, key)
	a.Status = StatusActive
	e.logger.Info("silenced alert promoted", "alert_id", a.ID, "entity", a.EntityName)
	e.fire(ctx, key, a)
}

// resolveSilenced closes a held alert without notifying anyone, since nothing was ever sent for it.
func (e *Engine) resolveSilenced(ctx context.Context, key activeAlertKey, a *Alert, at time.Time) {
	if err := withRetry(ctx, func() error {
		return e.alertStore.UpdateAlertStatus(ctx, a.ID, StatusResolved, &at, nil)
	}); err != nil {
		e.logger.Error("alert engine: resolve silenced alert", "error", err, "alert_id", a.ID)
		return
	}
	delete(e.silenced, key)
}

// SilencedAlerts returns copies of the alerts a silence or a maintenance window holds back.
func (e *Engine) SilencedAlerts() []*Alert {
	e.procMu.Lock()
	defer e.procMu.Unlock()
	held := make([]*Alert, 0, len(e.silenced))
	for _, a := range e.silenced {
		c := *a
		held = append(held, &c)
	}
	return held
}

func keyOf(a *Alert) activeAlertKey {
	return activeAlertKey{Source: a.Source, AlertType: a.AlertType, EntityType: a.EntityType, EntityID: a.EntityID}
}

func eventOf(a *Alert) Event {
	return Event{Source: a.Source, AlertType: a.AlertType, EntityType: a.EntityType, EntityID: a.EntityID}
}

func (e *Engine) processEvent(ctx context.Context, evt Event) {
	e.procMu.Lock()
	defer e.procMu.Unlock()

	if evt.IsRecover {
		e.processRecovery(ctx, evt)
		return
	}

	key := activeAlertKey{
		Source:     evt.Source,
		AlertType:  evt.AlertType,
		EntityType: evt.EntityType,
		EntityID:   evt.EntityID,
	}

	// Dedup: skip if there's already an active alert for this key,
	// unless the new event has a higher severity (escalation) or is a reminder.
	e.mu.RLock()
	existing, exists := e.activeAlerts[key]
	e.mu.RUnlock()

	if exists {
		// Collision guard: one dedup key must identify exactly one subject. If an
		// incoming event lands on an existing alert but names a DIFFERENT entity,
		// two distinct subjects are sharing a key — almost always a missing/blank
		// entity_id in the emitting source. Log loudly so it can never corrupt
		// silently (this is the "keycloak message on bitwarden entity" signature).
		if evt.EntityName != existing.EntityName {
			e.logger.Error("alert engine: dedup key collision — one key claimed by two entities (emitting source likely omits a unique entity_id)",
				"source", evt.Source,
				"alert_type", evt.AlertType,
				"entity_type", evt.EntityType,
				"entity_id", evt.EntityID,
				"existing_entity", existing.EntityName,
				"incoming_entity", evt.EntityName,
			)
		}
		if severityRank(evt.Severity) > severityRank(existing.Severity) || evt.Renotify {
			e.escalateAlert(ctx, existing, evt)
		} else {
			e.logger.Debug("alert: dedup, skipping duplicate",
				"source", evt.Source,
				"alert_type", evt.AlertType,
				"entity_type", evt.EntityType,
				"entity_id", evt.EntityID,
			)
		}
		return
	}

	// Check silence rules before persisting
	silenced := e.checkSilenceRules(ctx, evt)

	if held, ok := e.silenced[key]; ok {
		if severityRank(evt.Severity) > severityRank(held.Severity) || evt.Renotify {
			if err := e.refreshAlert(ctx, held, evt); err != nil {
				e.logger.Error("alert engine: refresh silenced alert", "error", err, "alert_id", held.ID)
			}
		}
		if !silenced {
			e.promote(ctx, key, held)
		}
		return
	}

	// Serialize details to JSON
	detailsJSON := "{}"
	if evt.Details != nil {
		if b, err := json.Marshal(evt.Details); err == nil {
			detailsJSON = string(b)
		}
	}

	a := &Alert{
		Source:     evt.Source,
		AlertType:  evt.AlertType,
		Severity:   evt.Severity,
		Status:     StatusActive,
		Message:    evt.Message,
		EntityType: evt.EntityType,
		EntityID:   evt.EntityID,
		EntityName: evt.EntityName,
		AgentID:    uid.Agent(evt.AgentID),
		Details:    detailsJSON,
		FiredAt:    evt.Timestamp,
	}

	if silenced {
		a.Status = StatusSilenced
		e.logger.Debug("alert: silenced by rule",
			"alert_id", a.ID,
			"source", evt.Source,
			"entity_name", evt.EntityName,
			"alert_type", evt.AlertType,
		)
	}

	// Persist the alert
	var id string
	if err := withRetry(ctx, func() (err error) {
		id, err = e.alertStore.InsertAlert(ctx, a)
		return err
	}); err != nil {
		e.logger.Error("alert engine: persist alert", "error", err)
		return
	}
	a.ID = id

	if silenced {
		e.silenced[key] = a
		e.broadcastAlert(a, true)
		return
	}
	e.fire(ctx, key, a)
}

// fire makes a stored alert live: tracked, broadcast, handed to the escalator and notified.
func (e *Engine) fire(ctx context.Context, key activeAlertKey, a *Alert) {
	e.mu.Lock()
	e.activeAlerts[key] = a
	e.mu.Unlock()

	// SSE broadcast
	e.broadcastAlert(a, false)

	// Notify the escalator that a new alert is live so it can start any
	// matching policy runs.
	if err := e.escalator.OnAlertCreated(ctx, a); err != nil {
		e.logger.ErrorContext(ctx, "alert engine: OnAlertCreated hook error", "error", err, "alert_id", a.ID)
	}

	if e.notifier != nil {
		e.dispatchNotifications(ctx, a, a)
	}
}

// refreshAlert rewrites a stored alert to the latest event, then its copy in memory.
func (e *Engine) refreshAlert(ctx context.Context, a *Alert, evt Event) error {
	// Refresh the record to the escalating event in full — severity, message AND
	// entity name/details — so an escalation can never leave a record mixing two
	// events' fields. EntityID/EntityType are part of the dedup key and therefore
	// already identical, so they are intentionally not touched.
	detailsJSON := "{}"
	if evt.Details != nil {
		if b, err := json.Marshal(evt.Details); err == nil {
			detailsJSON = string(b)
		}
	}
	if err := withRetry(ctx, func() error {
		return e.alertStore.UpdateAlertOnEscalation(ctx, a.ID, evt.Severity, evt.Message, evt.EntityName, detailsJSON)
	}); err != nil {
		return err
	}
	a.Severity = evt.Severity
	a.Message = evt.Message
	a.EntityName = evt.EntityName
	a.Details = detailsJSON
	return nil
}

func (e *Engine) escalateAlert(ctx context.Context, existing *Alert, evt Event) {
	oldSeverity := existing.Severity
	raised := severityRank(evt.Severity) > severityRank(oldSeverity)

	if err := e.refreshAlert(ctx, existing, evt); err != nil {
		e.logger.Error("alert engine: escalate", "error", err, "alert_id", existing.ID)
		return
	}

	if stored, err := e.alertStore.GetAlert(ctx, existing.ID); err != nil {
		e.logger.Error("alert engine: reload escalated alert", "error", err, "alert_id", existing.ID)
	} else if stored != nil {
		existing.AcknowledgedAt, existing.AcknowledgedBy = stored.AcknowledgedAt, stored.AcknowledgedBy
	}

	if raised {
		e.logger.Warn("alert escalated",
			"alert_id", existing.ID,
			"entity", existing.EntityName,
			"from", oldSeverity,
			"to", evt.Severity,
		)
	} else if existing.AcknowledgedAt != nil {
		return
	}

	// Broadcast the escalation and dispatch notifications at the new severity
	e.broadcastAlert(existing, false)

	// Re-evaluate escalation policies: a higher severity may match policies
	// that did not at the previous level. The escalator dedupes per
	// (alert, policy) so existing runs continue untouched.
	if raised && existing.AcknowledgedAt == nil {
		if err := e.escalator.OnAlertCreated(ctx, existing); err != nil {
			e.logger.ErrorContext(ctx, "alert engine: OnAlertCreated hook error", "error", err, "alert_id", existing.ID)
		}
	}

	if e.notifier != nil {
		payload := *existing
		if raised {
			payload.Message = fmt.Sprintf("Severity raised from %s to %s: %s", oldSeverity, existing.Severity, existing.Message)
		}
		e.dispatchNotifications(ctx, existing, &payload)
	}
}

// ErrAlertNotFound reports an alert id that names no alert.
var ErrAlertNotFound = errors.New("alert not found")

// ErrNotAcknowledgeable reports an alert that is not active or is already acknowledged.
var ErrNotAcknowledgeable = errors.New("alert is not active or already acknowledged")

// Acknowledge records who acknowledged an active alert, broadcasts it and stops its escalation.
func (e *Engine) Acknowledge(ctx context.Context, id, by string) (*Alert, error) {
	a, err := e.alertStore.GetAlert(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrAlertNotFound
	}
	if a.Status != StatusActive || a.AcknowledgedAt != nil {
		return nil, ErrNotAcknowledgeable
	}

	at := time.Now().UTC().Truncate(time.Second)
	acknowledged, err := e.alertStore.AcknowledgeAlert(ctx, id, by, at)
	if err != nil {
		return nil, err
	}
	if !acknowledged {
		return nil, ErrNotAcknowledgeable
	}
	a.AcknowledgedAt = &at
	a.AcknowledgedBy = by

	if e.broadcaster != nil {
		e.broadcaster.Broadcast(event.AlertAcknowledged, a)
	}
	if err := e.escalator.OnAlertAcknowledged(ctx, id, Acknowledgment{By: by, At: at}); err != nil {
		e.logger.ErrorContext(ctx, "alert engine: OnAlertAcknowledged hook error", "error", err, "alert_id", id)
	}
	return a, nil
}

// severityRank returns a numeric rank for severity comparison (higher = more severe).
func severityRank(s string) int {
	switch s {
	case SeverityInfo:
		return 0
	case SeverityWarning:
		return 1
	case SeverityCritical:
		return 2
	default:
		return -1
	}
}

func (e *Engine) processRecovery(ctx context.Context, evt Event) {
	key := activeAlertKey{
		Source:     evt.Source,
		AlertType:  evt.AlertType,
		EntityType: evt.EntityType,
		EntityID:   evt.EntityID,
	}

	e.mu.RLock()
	activeAlert, exists := e.activeAlerts[key]
	e.mu.RUnlock()

	if !exists {
		if held, ok := e.silenced[key]; ok {
			e.resolveSilenced(ctx, key, held, evt.Timestamp)
			return
		}
		e.logger.Debug("alert: recovery without active alert",
			"source", evt.Source,
			"alert_type", evt.AlertType,
			"entity_type", evt.EntityType,
			"entity_id", evt.EntityID,
		)
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
		AlertType:  evt.AlertType,
		Severity:   evt.Severity,
		Status:     StatusResolved,
		Message:    evt.Message,
		EntityType: evt.EntityType,
		EntityID:   evt.EntityID,
		EntityName: evt.EntityName,
		AgentID:    activeAlert.AgentID,
		Details:    detailsJSON,
		FiredAt:    evt.Timestamp,
	}

	var recoveryID string
	if err := withRetry(ctx, func() (err error) {
		recoveryID, err = e.alertStore.InsertAlert(ctx, recoveryAlert)
		return err
	}); err != nil {
		e.logger.Error("alert engine: persist recovery", "error", err, "alert_id", activeAlert.ID)
		return
	}
	recoveryAlert.ID = recoveryID

	// Resolve the original alert
	now := evt.Timestamp
	if err := withRetry(ctx, func() error {
		return e.alertStore.UpdateAlertStatus(ctx, activeAlert.ID, StatusResolved, &now, &recoveryID)
	}); err != nil {
		e.logger.Error("alert engine: resolve original alert", "error", err, "alert_id", activeAlert.ID)
		return
	}

	e.mu.Lock()
	delete(e.activeAlerts, key)
	e.mu.Unlock()

	if err := e.escalator.OnAlertResolved(ctx, activeAlert.ID, now); err != nil {
		e.logger.ErrorContext(ctx, "alert engine: OnAlertResolved hook error", "error", err, "alert_id", activeAlert.ID)
	}

	// Update the resolved alert fields for broadcasting
	activeAlert.Status = StatusResolved
	activeAlert.ResolvedAt = &now
	activeAlert.ResolvedByID = &recoveryID

	// SSE broadcast
	e.broadcastResolved(activeAlert)

	// Route by the original alert, but notify a payload that reads as a recovery.
	if e.notifier != nil {
		payload := *activeAlert
		payload.Message = evt.Message
		payload.Severity = evt.Severity
		e.dispatchNotifications(ctx, activeAlert, &payload)
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
	covered, err := e.covered(ctx, rules, evt)
	if err != nil {
		e.logger.Error("alert engine: maintenance suppressor error", "error", err)
	}
	return covered
}

// covered reports whether one of rules or a maintenance window covers the event's entity.
func (e *Engine) covered(ctx context.Context, rules []*SilenceRule, evt Event) (bool, error) {
	for _, rule := range rules {
		if matchesSilenceRule(rule, evt) {
			e.logger.Debug("alert: silence rule matched",
				"rule_id", rule.ID,
				"source", evt.Source,
				"entity_type", evt.EntityType,
			)
			return true, nil
		}
	}

	// Consult maintenance suppressor extension (Pro: calendar-based suppression)
	suppressed, err := e.suppressor.IsSuppressed(ctx, evt.Source, evt.EntityType, evt.EntityID)
	if err != nil {
		return false, err
	}
	if suppressed {
		e.logger.Debug("alert: suppressed by maintenance window",
			"source", evt.Source,
			"entity_type", evt.EntityType,
			"entity_id", evt.EntityID,
		)
	}
	return suppressed, nil
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

// dispatchNotifications matches triggers and the entity router against routeBy,
// then delivers payload (they differ only for recoveries).
func (e *Engine) dispatchNotifications(ctx context.Context, routeBy *Alert, payload *Alert) {
	if e.channelStore == nil || e.notifier == nil {
		return
	}

	dispatched := make(map[string]bool)

	// Resolve channels from active alert triggers.
	if e.triggerStore != nil {
		var triggers []*AlertTrigger
		if err := withRetry(ctx, func() (err error) {
			triggers, err = e.triggerStore.ListEnabledTriggers(ctx)
			return err
		}); err != nil {
			e.logger.Warn("alert engine: alert not notified, alert triggers unreadable", "error", err, "alert_id", routeBy.ID)
			return
		}
		for _, t := range triggers {
			if !matchesTrigger(t, routeBy) {
				continue
			}
			for _, chID := range t.ChannelIDs {
				if dispatched[chID] {
					continue
				}
				var ch *NotificationChannel
				if err := withRetry(ctx, func() (err error) {
					ch, err = e.channelStore.GetChannel(ctx, chID)
					return err
				}); err != nil {
					e.logger.Warn("alert engine: channel not notified, channel unreadable", "error", err, "alert_id", routeBy.ID, "channel_id", chID)
					continue
				}
				if ch == nil || !ch.Enabled {
					continue
				}
				dispatched[chID] = true
				e.enqueueDelivery(ctx, ch, payload)
			}
		}
	}
}

func (e *Engine) enqueueDelivery(ctx context.Context, ch *NotificationChannel, a *Alert) {
	delivery := &NotificationDelivery{
		AlertID:   a.ID,
		ChannelID: ch.ID,
		Status:    DeliveryPending,
	}
	deliveryID, err := e.channelStore.InsertDelivery(ctx, delivery)
	if err != nil {
		e.logger.Error("alert engine: create delivery", "error", err, "channel_id", ch.ID)
		return
	}
	delivery.ID = deliveryID

	e.logger.Debug("alert: dispatching notification",
		"channel_id", ch.ID,
		"channel_type", ch.Type,
		"alert_id", a.ID,
	)
	snapshot := *a // the engine keeps mutating a (escalation, recovery) while the job waits in the queue
	e.notifier.Enqueue(NotificationJob{
		Delivery: delivery,
		Channel:  ch,
		Alert:    &snapshot,
	})
}

// matchesTrigger reports whether an alert satisfies all of a trigger's
// non-empty filters (AND between fields, OR within a CSV field). An empty
// filter matches everything.
func matchesTrigger(t *AlertTrigger, a *Alert) bool {
	if a.Status == StatusResolved && !t.NotifyOnResolve {
		return false
	}
	if t.FilterSeverities != "" && !containsCSV(t.FilterSeverities, a.Severity) {
		return false
	}
	if t.FilterSources != "" && !containsCSV(t.FilterSources, a.Source) {
		return false
	}
	if t.FilterScopes != "" {
		scope := fmt.Sprintf("%s:%s", a.EntityType, a.EntityID)
		if !containsCSV(t.FilterScopes, scope) {
			return false
		}
	}
	return true
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

	eventType := event.AlertFired
	if silenced {
		eventType = event.AlertSilenced
	}

	e.broadcaster.Broadcast(eventType, alertToMap(a))
}

func (e *Engine) broadcastResolved(a *Alert) {
	if e.broadcaster == nil {
		return
	}
	e.broadcaster.Broadcast(event.AlertResolved, alertToMap(a))
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
		"agent_id":    a.AgentID,
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
	if a.AcknowledgedAt != nil {
		m["acknowledged_at"] = a.AcknowledgedAt.UTC().Format(time.RFC3339)
	}
	if a.AcknowledgedBy != "" {
		m["acknowledged_by"] = a.AcknowledgedBy
	}
	if a.EscalatedAt != nil {
		m["escalated_at"] = a.EscalatedAt.UTC().Format(time.RFC3339)
	}

	return m
}

// ResolveByEntity resolves all active alerts for a given entity (e.g. when a
// container is destroyed). This prevents stale alerts from accumulating when
// containers are recreated with new internal IDs.
func (e *Engine) ResolveByEntity(ctx context.Context, entityType string, entityID string) {
	e.procMu.Lock()
	defer e.procMu.Unlock()
	now := time.Now()

	e.mu.RLock()
	toResolve := make(map[activeAlertKey]*Alert)
	for key, a := range e.activeAlerts {
		if key.EntityType == entityType && key.EntityID == entityID {
			toResolve[key] = a
		}
	}
	e.mu.RUnlock()

	for key, a := range toResolve {
		if err := withRetry(ctx, func() error {
			return e.alertStore.UpdateAlertStatus(ctx, a.ID, StatusResolved, &now, nil)
		}); err != nil {
			e.logger.Error("alert engine: resolve on entity removal", "error", err, "alert_id", a.ID)
			continue
		}
		e.mu.Lock()
		delete(e.activeAlerts, key)
		e.mu.Unlock()
		if err := e.escalator.OnAlertResolved(ctx, a.ID, now); err != nil {
			e.logger.ErrorContext(ctx, "alert engine: OnAlertResolved hook error", "error", err, "alert_id", a.ID)
		}
		a.Status = StatusResolved
		a.ResolvedAt = &now
		e.broadcastResolved(a)
		e.logger.Info("resolved alert for removed entity",
			"alert_id", a.ID,
			"entity_type", entityType,
			"entity_id", entityID,
		)
	}

	for key, a := range e.silenced {
		if key.EntityType == entityType && key.EntityID == entityID {
			e.resolveSilenced(ctx, key, a, now)
		}
	}
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
	return fmt.Sprintf("%s/%s/%s/%s", k.Source, k.AlertType, k.EntityType, k.EntityID)
}
