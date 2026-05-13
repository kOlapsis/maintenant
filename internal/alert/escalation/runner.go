// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package escalation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
)

// Run statuses (mirror the SQL CHECK constraint on escalation_runs).
const (
	RunStatusActive                = "active"
	RunStatusPausedByMaintenance   = "paused_by_maintenance"
	RunStatusStoppedByAck          = "stopped_by_ack"
	RunStatusStoppedByResolution   = "stopped_by_resolution"
	RunStatusStoppedByPolicyDelete = "stopped_by_policy_deletion"
	RunStatusStoppedByPolicyDisabl = "stopped_by_policy_disabled"
	RunStatusStoppedByDowngrade    = "stopped_by_edition_downgrade"
	RunStatusExhausted             = "exhausted"
)

// Delivery statuses (mirror the SQL CHECK constraint on escalation_deliveries).
const (
	DeliveryStatusPending             = "pending"
	DeliveryStatusSent                = "sent"
	DeliveryStatusFailed              = "failed"
	DeliveryStatusAbandoned           = "abandoned"
	DeliveryStatusSkippedMaintenance  = "skipped_maintenance"
)

// Reserved level indices for non-step deliveries (negative values are intentional —
// they let us trace ack/exhausted notifications in the same table without a
// separate schema). -2 is reserved for resolve notifications (not dispatched
// today; the engine's recovery broadcast already covers user-visible feedback).
const (
	specialLevelAck       = -1
	specialLevelExhausted = -3
)

const (
	defaultOrphanTimeout = 2 * time.Minute
	maintenanceRecheck   = 60 * time.Second
)

// Sender abstracts the synchronous-send capability of *alert.Notifier so the
// Runner can be unit-tested without spinning up an HTTP server. *alert.Notifier
// satisfies it.
type Sender interface {
	SendNow(ctx context.Context, a *alert.Alert, ch *alert.NotificationChannel) error
}

// RunnerDeps bundles the dependencies of the concrete Pro escalator.
type RunnerDeps struct {
	Store         Store
	AlertStore    alert.AlertStore
	ChannelStore  alert.ChannelStore
	Notifier      Sender
	Suppressor    alert.MaintenanceSuppressor
	Service       *Service
	Logger        *slog.Logger
	Clock         func() time.Time
	OrphanTimeout time.Duration
}

// Runner is the concrete alert.Escalator. It owns the runtime state machine
// (active runs, paused runs, exhausted runs) and dispatches escalation
// notifications via the provided alert.Notifier — its delivery rows live in
// escalation_deliveries (separate from notification_deliveries).
//
// Idempotence (R4): InsertDelivery enforces a UNIQUE (run_id, level_index,
// channel_id) constraint, so a crash between InsertDelivery and the network
// send is recovered at the next cycle without duplicate notifications.
type Runner struct {
	store         Store
	alertStore    alert.AlertStore
	channelStore  alert.ChannelStore
	notifier      Sender
	suppressor    alert.MaintenanceSuppressor
	service       *Service
	logger        *slog.Logger
	clock         func() time.Time
	orphanTimeout time.Duration
}

// NewRunner wires a Runner. AlertStore, ChannelStore, Notifier, Service, Logger
// must be non-nil. Suppressor defaults to a no-op when omitted.
func NewRunner(d RunnerDeps) *Runner {
	if d.Store == nil || d.AlertStore == nil || d.ChannelStore == nil ||
		d.Notifier == nil || d.Service == nil || d.Logger == nil {
		panic("escalation.NewRunner: required dependency missing")
	}
	clock := d.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	timeout := d.OrphanTimeout
	if timeout <= 0 {
		timeout = defaultOrphanTimeout
	}
	suppressor := d.Suppressor
	if suppressor == nil {
		suppressor = noopRunnerSuppressor{}
	}
	return &Runner{
		store:         d.Store,
		alertStore:    d.AlertStore,
		channelStore:  d.ChannelStore,
		notifier:      d.Notifier,
		suppressor:    suppressor,
		service:       d.Service,
		logger:        d.Logger,
		clock:         clock,
		orphanTimeout: timeout,
	}
}

type noopRunnerSuppressor struct{}

func (noopRunnerSuppressor) IsSuppressed(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

// --- alert.Escalator implementation ---

// OnAlertCreated starts a Run for every active policy whose filters match the
// alert and that does not already have an active run for this alert. Idempotent
// per (alertID, policyID) tuple — safe to call multiple times (e.g. on severity
// escalation, when new policies become applicable).
func (r *Runner) OnAlertCreated(ctx context.Context, a *alert.Alert) error {
	if a == nil || a.ID == 0 {
		return nil
	}

	existing, err := r.store.SelectActiveRunsByAlert(ctx, a.ID)
	if err != nil {
		return fmt.Errorf("escalation: list existing runs: %w", err)
	}
	covered := make(map[int64]struct{}, len(existing))
	for _, run := range existing {
		if run.PolicyID != nil {
			covered[*run.PolicyID] = struct{}{}
		}
	}

	policies, err := r.store.SelectPolicies(ctx, true)
	if err != nil {
		return fmt.Errorf("escalation: list policies: %w", err)
	}

	now := r.clock()
	for _, p := range policies {
		if _, already := covered[p.ID]; already {
			continue
		}
		if !matchPolicyFilters(a, p) {
			continue
		}
		if len(p.Levels) == 0 {
			continue
		}

		snapshot, mErr := json.Marshal(p)
		if mErr != nil {
			r.logger.ErrorContext(ctx, "escalation: snapshot marshal", "error", mErr, "policy_id", p.ID)
			continue
		}

		nextAt := now.Add(time.Duration(p.Levels[0].DelaySeconds) * time.Second)
		policyID := p.ID
		run := &Run{
			PolicyID:               &policyID,
			PolicySnapshotJSON:     string(snapshot),
			AlertID:                a.ID,
			Status:                 RunStatusActive,
			LastExecutedLevelIndex: -1,
			StartedAt:              now,
			NextActionAt:           &nextAt,
		}
		if _, err := r.store.InsertRun(ctx, run); err != nil {
			r.logger.ErrorContext(ctx, "escalation: insert run", "error", err, "alert_id", a.ID, "policy_id", p.ID)
			continue
		}
		r.logger.InfoContext(ctx, "escalation: run started",
			"run_id", run.ID, "alert_id", a.ID, "policy_id", p.ID,
			"next_action_at", nextAt.Format(time.RFC3339))
	}
	return nil
}

// OnAlertAcknowledged terminates every active run attached to the alert and
// dispatches an ack notification on the channels of the last executed level.
func (r *Runner) OnAlertAcknowledged(ctx context.Context, alertID int64, ack alert.Acknowledgment) error {
	runs, err := r.store.SelectActiveRunsByAlert(ctx, alertID)
	if err != nil {
		return fmt.Errorf("escalation: list runs for ack: %w", err)
	}
	if len(runs) == 0 {
		return nil
	}

	a, err := r.alertStore.GetAlert(ctx, alertID)
	if err != nil {
		r.logger.ErrorContext(ctx, "escalation: load alert for ack notif", "error", err, "alert_id", alertID)
	}

	now := r.clock()
	for _, run := range runs {
		if termErr := r.store.TerminateRun(ctx, run.ID, RunStatusStoppedByAck, now); termErr != nil {
			r.logger.ErrorContext(ctx, "escalation: terminate on ack", "error", termErr, "run_id", run.ID)
			continue
		}
		r.logger.InfoContext(ctx, "escalation: run stopped by ack",
			"run_id", run.ID, "alert_id", alertID, "by", ack.By)

		if a == nil || run.LastExecutedLevelIndex < 0 {
			continue
		}
		policy, perr := unmarshalPolicySnapshot(run.PolicySnapshotJSON)
		if perr != nil || policy == nil {
			continue
		}
		idx := run.LastExecutedLevelIndex
		if idx >= len(policy.Levels) {
			continue
		}
		ackAlert := formatAckAlert(a, ack)
		r.dispatchSpecial(ctx, run.ID, specialLevelAck, policy.Levels[idx].ChannelIDs, ackAlert)
	}
	return nil
}

// OnAlertResolved terminates every active run attached to the alert. The
// recovery broadcast already covers user-visible feedback; we do not send a
// per-channel "resolved" notification (avoids dedup work — channels usually
// receive the recovery via the standard alert pipeline).
func (r *Runner) OnAlertResolved(ctx context.Context, alertID int64, resolvedAt time.Time) error {
	runs, err := r.store.SelectActiveRunsByAlert(ctx, alertID)
	if err != nil {
		return fmt.Errorf("escalation: list runs for resolve: %w", err)
	}
	for _, run := range runs {
		if err := r.store.TerminateRun(ctx, run.ID, RunStatusStoppedByResolution, resolvedAt); err != nil {
			r.logger.ErrorContext(ctx, "escalation: terminate on resolve", "error", err, "run_id", run.ID)
			continue
		}
		r.logger.InfoContext(ctx, "escalation: run stopped by resolution",
			"run_id", run.ID, "alert_id", alertID)
	}
	return nil
}

// OnEditionDowngraded delegates to the CE service's bulk-deactivate routine.
func (r *Runner) OnEditionDowngraded(ctx context.Context) error {
	return r.service.OnEditionDowngraded(ctx)
}

// EvaluateCycle is invoked periodically by the alert engine (default 60s). It
// recovers stuck deliveries, then steps every due run forward.
func (r *Runner) EvaluateCycle(ctx context.Context) error {
	now := r.clock()

	if err := r.recoverOrphans(ctx, now); err != nil {
		r.logger.ErrorContext(ctx, "escalation: orphan recovery", "error", err)
	}

	runs, err := r.store.SelectDueRuns(ctx, now)
	if err != nil {
		return fmt.Errorf("escalation: select due runs: %w", err)
	}
	for _, run := range runs {
		if err := r.processRun(ctx, run, now); err != nil {
			r.logger.ErrorContext(ctx, "escalation: process run", "error", err, "run_id", run.ID)
		}
	}
	return nil
}

// --- internals ---

func (r *Runner) processRun(ctx context.Context, run *Run, now time.Time) error {
	a, err := r.alertStore.GetAlert(ctx, run.AlertID)
	if err != nil {
		return fmt.Errorf("load alert: %w", err)
	}
	if a == nil {
		// Alert was deleted; close the run silently.
		return r.store.TerminateRun(ctx, run.ID, RunStatusStoppedByResolution, now)
	}
	// Defensive: alert may have transitioned to resolved/silenced via a path
	// that bypassed our hooks (e.g. retention purge in flight). Stop the run.
	if a.Status != alert.StatusActive {
		return r.store.TerminateRun(ctx, run.ID, RunStatusStoppedByResolution, now)
	}

	suppressed, sErr := r.suppressor.IsSuppressed(ctx, a.Source, a.EntityType, fmt.Sprint(a.EntityID))
	if sErr != nil {
		r.logger.ErrorContext(ctx, "escalation: suppressor", "error", sErr, "run_id", run.ID)
		// Fail-open on suppressor errors to avoid stalling escalations.
	}
	if suppressed {
		recheckAt := now.Add(maintenanceRecheck)
		if err := r.store.PauseRunForMaintenance(ctx, run.ID, recheckAt); err != nil {
			return fmt.Errorf("pause run: %w", err)
		}
		r.logger.DebugContext(ctx, "escalation: run paused by maintenance", "run_id", run.ID)
		return nil
	}

	if run.Status == RunStatusPausedByMaintenance {
		// Maintenance window cleared — back to active. Don't consume a level
		// this tick; the next call to EvaluateCycle will pick it up via the
		// standard active path.
		return r.store.ResumeRunFromMaintenance(ctx, run.ID, now)
	}

	policy, err := unmarshalPolicySnapshot(run.PolicySnapshotJSON)
	if err != nil {
		r.logger.ErrorContext(ctx, "escalation: bad snapshot — terminating run",
			"run_id", run.ID, "error", err)
		return r.store.TerminateRun(ctx, run.ID, RunStatusStoppedByPolicyDelete, now)
	}

	nextLevel := run.LastExecutedLevelIndex + 1
	if nextLevel >= len(policy.Levels) {
		if run.LastExecutedLevelIndex >= 0 && run.LastExecutedLevelIndex < len(policy.Levels) {
			exhAlert := formatExhaustedAlert(a, len(policy.Levels))
			r.dispatchSpecial(ctx, run.ID, specialLevelExhausted,
				policy.Levels[run.LastExecutedLevelIndex].ChannelIDs, exhAlert)
		}
		return r.store.TerminateRun(ctx, run.ID, RunStatusExhausted, now)
	}

	level := policy.Levels[nextLevel]
	r.executeLevel(ctx, run, nextLevel, level.ChannelIDs, a)

	// Schedule the next tick. After the last level we still want the run to
	// surface once more so the next cycle can dispatch the "exhausted" notif
	// and terminate the run — set next_action_at = now to make that happen.
	var nextAt time.Time
	if nextLevel+1 < len(policy.Levels) {
		nextAt = now.Add(time.Duration(policy.Levels[nextLevel+1].DelaySeconds) * time.Second)
	} else {
		nextAt = now
	}
	if err := r.store.UpdateRunProgress(ctx, run.ID, nextLevel, &nextAt, RunStatusActive); err != nil {
		return fmt.Errorf("update run progress: %w", err)
	}
	return nil
}

func (r *Runner) executeLevel(ctx context.Context, run *Run, levelIndex int, channelIDs []int64, a *alert.Alert) {
	now := r.clock()
	for _, chID := range channelIDs {
		r.dispatchToChannel(ctx, run.ID, levelIndex, chID, a, now)
	}
}

func (r *Runner) dispatchSpecial(ctx context.Context, runID int64, levelIndex int, channelIDs []int64, a *alert.Alert) {
	now := r.clock()
	for _, chID := range channelIDs {
		r.dispatchToChannel(ctx, runID, levelIndex, chID, a, now)
	}
}

// dispatchToChannel reserves a delivery row (UNIQUE constraint catches crash-recovery
// duplicates, FR-022) and dispatches the send in a goroutine if the channel is enabled.
// One channel's failure must not block the others.
func (r *Runner) dispatchToChannel(ctx context.Context, runID int64, levelIndex int, chID int64, a *alert.Alert, now time.Time) {
	ch, err := r.channelStore.GetChannel(ctx, chID)
	if err != nil {
		r.logger.ErrorContext(ctx, "escalation: get channel", "error", err, "channel_id", chID, "run_id", runID)
		return
	}
	if ch == nil {
		r.logger.WarnContext(ctx, "escalation: channel not found", "channel_id", chID, "run_id", runID)
		return
	}
	chIDCopy := chID
	delivery := &Delivery{
		RunID:            runID,
		LevelIndex:       levelIndex,
		ChannelID:        &chIDCopy,
		Status:           DeliveryStatusPending,
		AttemptStartedAt: now,
	}
	if _, err := r.store.InsertDelivery(ctx, delivery); err != nil {
		if !errors.Is(err, ErrDeliveryDuplicate) {
			r.logger.ErrorContext(ctx, "escalation: insert delivery", "error", err,
				"run_id", runID, "level_index", levelIndex, "channel_id", chID)
		}
		return
	}
	if !ch.Enabled {
		delivery.Status = DeliveryStatusFailed
		delivery.Error = "channel disabled"
		if uErr := r.store.UpdateDelivery(ctx, delivery); uErr != nil {
			r.logger.ErrorContext(ctx, "escalation: update disabled delivery", "error", uErr, "delivery_id", delivery.ID)
		}
		return
	}
	go r.deliverAndUpdate(ctx, delivery, a, ch)
}

// deliverAndUpdate performs the actual network send and persists the outcome.
func (r *Runner) deliverAndUpdate(ctx context.Context, delivery *Delivery, a *alert.Alert, ch *alert.NotificationChannel) {
	err := r.notifier.SendNow(ctx, a, ch)
	if err != nil {
		delivery.Status = DeliveryStatusFailed
		delivery.Error = truncateErr(err.Error())
	} else {
		delivery.Status = DeliveryStatusSent
		t := r.clock()
		delivery.SentAt = &t
	}
	if uErr := r.store.UpdateDelivery(ctx, delivery); uErr != nil {
		r.logger.ErrorContext(ctx, "escalation: update delivery", "error", uErr, "delivery_id", delivery.ID)
	}
}

// recoverOrphans handles deliveries stuck in 'pending' beyond the orphan timeout.
// If the alert is still firing, the delivery is retried; otherwise it is abandoned (R4).
func (r *Runner) recoverOrphans(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-r.orphanTimeout)
	orphans, err := r.store.SelectOrphanPendingDeliveries(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("select orphans: %w", err)
	}
	for _, d := range orphans {
		run, err := r.store.SelectRun(ctx, d.RunID)
		if err != nil || run == nil {
			r.abandonDelivery(ctx, d, "run vanished")
			continue
		}
		// Run already terminal → abandon the orphan.
		if run.Status != RunStatusActive && run.Status != RunStatusPausedByMaintenance {
			r.abandonDelivery(ctx, d, "run terminated")
			continue
		}
		a, err := r.alertStore.GetAlert(ctx, run.AlertID)
		if err != nil || a == nil || a.Status != alert.StatusActive {
			r.abandonDelivery(ctx, d, "alert no longer active")
			continue
		}
		if d.ChannelID == nil {
			r.abandonDelivery(ctx, d, "channel removed")
			continue
		}
		ch, err := r.channelStore.GetChannel(ctx, *d.ChannelID)
		if err != nil || ch == nil {
			r.abandonDelivery(ctx, d, "channel unavailable")
			continue
		}
		go r.deliverAndUpdate(ctx, d, a, ch)
	}
	return nil
}

func (r *Runner) abandonDelivery(ctx context.Context, d *Delivery, reason string) {
	d.Status = DeliveryStatusAbandoned
	d.Error = reason
	_ = r.store.UpdateDelivery(ctx, d)
}

// --- helpers ---

// matchPolicyFilters reports whether an alert satisfies a policy's filters.
// Empty filter buckets match everything (universe). Tags are not yet exposed
// on the Alert entity — treated as no-op (consistent with engine.matchesTrigger).
func matchPolicyFilters(a *alert.Alert, p *Policy) bool {
	if len(p.Filters.Severities) > 0 && !slices.Contains(p.Filters.Severities, a.Severity) {
		return false
	}
	if len(p.Filters.Scopes) > 0 {
		matched := false
		for _, sc := range p.Filters.Scopes {
			if sc.Kind == a.EntityType && sc.RefID == a.EntityID {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func unmarshalPolicySnapshot(s string) (*Policy, error) {
	if s == "" {
		return nil, errors.New("empty snapshot")
	}
	var p Policy
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return &p, nil
}

// formatAckAlert produces a synthetic *Alert reusing the notifier's formatting
// pipeline. Cloning lets us override Message/Status without mutating the
// upstream alert tracked by the engine.
func formatAckAlert(a *alert.Alert, ack alert.Acknowledgment) *alert.Alert {
	cp := *a
	cp.Status = alert.StatusResolved // routes the notifier through the "resolved" copy
	at := ack.At.Format(time.RFC3339)
	by := ack.By
	if by == "" {
		by = "—"
	}
	cp.Message = fmt.Sprintf("✓ Acquittée par %s à %s — %s", by, at, a.Message)
	return &cp
}

// formatExhaustedAlert produces a synthetic *Alert for the "escalation
// exhausted — human intervention required" notification (FR-013).
func formatExhaustedAlert(a *alert.Alert, totalLevels int) *alert.Alert {
	cp := *a
	// Keep severity/status of the original alert so the notifier still routes
	// it as a critical/warning event but with an explicit message.
	cp.Message = fmt.Sprintf(
		"⚠ Escalation épuisée — action humaine requise après %d palier(s) sans acquittement. %s",
		totalLevels, a.Message,
	)
	return &cp
}

// truncateErr keeps the persisted error column compact (DB-friendly).
func truncateErr(s string) string {
	const max = 500
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max]
	}
	return s
}
