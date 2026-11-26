// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package license

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/retry"
	"github.com/kolapsis/maintenant/internal/trust"
)

const (
	checkInterval = 24 * time.Hour

	// Graceful degradation windows: how long Pro stays active when the
	// license server is unreachable, based on cache age.
	graceDegradationWarn     = 7 * 24 * time.Hour  // 7 days
	graceDegradationError    = 30 * 24 * time.Hour // 30 days
	graceDegradationDisabled = 60 * 24 * time.Hour // 60 days

	maxClockSkew = 24 * time.Hour
)

var (
	checkRetryMin = time.Minute
	checkRetryMax = time.Hour
)

// State is the licence status the manager maintains.
type State = extension.LicenseState

// EditionChangeCallback is invoked from a dedicated goroutine with a 30-second timeout and must not block.
type EditionChangeCallback = extension.EditionChangeCallback

// Manager handles license verification, caching, and state management.
type Manager struct {
	licenseKey string
	dataDir    string
	version    string
	logger     *slog.Logger
	publicKey  ed25519.PublicKey
	client     *http.Client
	state      atomic.Value // *State
	stop       chan struct{}
	running    sync.WaitGroup

	// buildDate is when this binary was released. buildDateKnown is false when
	// ldflags left the default "unknown", or stamped something unparseable; the
	// update window is then never enforced.
	buildDate      time.Time
	buildDateKnown bool

	signedTime     time.Time
	signedTimeSeen time.Time

	callbacksMu sync.Mutex
	callbacks   []EditionChangeCallback
	lastEdition extension.Edition
	baselineSet bool
}

// NewManager creates a new license manager and synchronously loads the
// disk cache so IsProEnabled() reflects the persisted Pro/CE state immediately.
// This matters because edition-gated wiring runs before Start (e.g.
// extension.CurrentEdition() checks during app construction). Call Start() to
// begin periodic verification.
func NewManager(licenseKey, dataDir, version, buildDate string, logger *slog.Logger) (*Manager, error) {
	pubKey, err := getPublicKey()
	if err != nil {
		return nil, err
	}

	m := &Manager{
		licenseKey: licenseKey,
		dataDir:    dataDir,
		version:    version,
		logger:     logger.With("component", "license"),
		publicKey:  pubKey,
		client:     &http.Client{Timeout: 10 * time.Second, Transport: trust.HTTPTransport()},
		stop:       make(chan struct{}),
	}

	// Parsed once here, and the failure reported once here: repeating it on
	// every 24-hour cycle would be noise.
	m.buildDate, m.buildDateKnown = parseBuildDate(buildDate)
	if !m.buildDateKnown {
		m.logger.Warn("build date is unreadable, the update window will not be enforced",
			"build_date", buildDate,
		)
	}

	// Default community mode; loadCache may upgrade us if a valid cache exists.
	// This is the one write that bypasses setStateAndNotify on purpose: it seeds
	// the zero value, there is nothing to transition from yet.
	m.state.Store(&State{Status: "unknown", Edition: extension.Community})

	// No callbacks are registered yet, so this never dispatches edition-change
	// notifications — it only establishes the baseline.
	m.loadCache(context.Background())

	return m, nil
}

// Start performs an initial license check, then starts a background ticker.
// Non-blocking: errors during the initial check are logged, not fatal. The
// disk cache has already been loaded by NewManager.
func (m *Manager) Start(ctx context.Context) {
	ok := m.check(ctx)

	m.running.Go(func() { m.ticker(ctx, ok) })
}

// loadCache populates state from the disk cache, if a valid one exists.
func (m *Manager) loadCache(ctx context.Context) {
	cached, err := readCache(m.dataDir, m.publicKey)
	if err != nil || cached == nil {
		return
	}
	m.recordSignedTime(cached.VerifiedAt)
	if fallback := offlineFallback(cached.VerifiedAt, cached.ExpiresAt, m.now()); fallback != nil {
		m.logger.Warn("cached license no longer grants an edition",
			"status", fallback.Status,
			"verified_at", cached.VerifiedAt,
			"expires_at", cached.ExpiresAt,
		)
		m.setStateAndNotify(ctx, fallback)
		return
	}
	m.applyPayload(ctx, cached)
	m.logger.Info("license loaded from cache",
		"status", cached.Status,
		"plan", cached.Plan,
		"verified_at", cached.VerifiedAt,
	)
}

// Stop stops the background ticker and waits for it and for the edition change callbacks in flight.
func (m *Manager) Stop() {
	close(m.stop)
	m.running.Wait()
}

// State returns the current license state (thread-safe).
func (m *Manager) State() *State {
	return m.state.Load().(*State)
}

// Edition returns the edition the current license grants.
func (m *Manager) Edition() extension.Edition {
	if e := m.State().Edition; e != "" {
		return e
	}
	return extension.Community
}

// RegisterEditionChangeCallback registers a callback invoked on edition
// transitions. Must be called before Start to avoid missing initial transitions.
func (m *Manager) RegisterEditionChangeCallback(cb EditionChangeCallback) {
	m.callbacksMu.Lock()
	defer m.callbacksMu.Unlock()
	m.callbacks = append(m.callbacks, cb)
}

func (m *Manager) dispatchEditionChange(ctx context.Context, prev, next extension.Edition) {
	m.callbacksMu.Lock()
	cbs := make([]EditionChangeCallback, len(m.callbacks))
	copy(cbs, m.callbacks)
	m.callbacksMu.Unlock()

	for _, f := range cbs {
		m.running.Go(func() {
			cbCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("license: callback panic", "recover", r)
				}
			}()
			f(cbCtx, prev, next)
		})
	}
}

// setStateAndNotify is the only way state is written. Routing every write
// through it is what makes a transition observable: the HTTP 401, HTTP 403 and
// network-degradation paths used to store directly, so a revocation seen on
// those paths never disabled anything.
func (m *Manager) setStateAndNotify(ctx context.Context, newState *State) {
	if newState.Edition == "" {
		newState.Edition = extension.Community
	}
	prev := m.lastEdition
	m.state.Store(newState)
	next := newState.Edition
	if m.baselineSet && prev != next {
		m.dispatchEditionChange(ctx, prev, next)
	}
	m.lastEdition = next
	m.baselineSet = true
}

func (m *Manager) ticker(ctx context.Context, ok bool) {
	backoff := retry.New(checkRetryMin, checkRetryMax, 0.25)
	t := time.NewTimer(checkInterval)
	defer t.Stop()
	for {
		wait := checkInterval
		if ok {
			backoff.Reset()
		} else {
			wait = backoff.Next()
		}
		t.Reset(wait)
		select {
		case <-ctx.Done():
			return
		case <-m.stop:
			return
		case <-t.C:
			ok = m.check(ctx)
		}
	}
}

// check asks the license server for the current verdict and reports whether a signed one arrived.
func (m *Manager) check(ctx context.Context) bool {
	serverURL := getLicenseServerURL()
	resp, statusCode, err := fetchLicense(ctx, m.client, serverURL, m.licenseKey, m.version)

	if err != nil {
		if (statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden) && m.applyRefusal(ctx, statusCode, resp, err) {
			return true
		}

		// Network error or other non-200: graceful degradation
		m.handleNetworkError(ctx, err)
		return false
	}

	// Verify signature
	payload, err := verify(m.publicKey, *resp)
	if err != nil {
		// Invalid signature = treat as network error (keep current state)
		m.logger.Error("license signature verification failed", "error", err)
		m.handleNetworkError(ctx, err)
		return false
	}

	skew := m.now().Sub(payload.VerifiedAt)
	if (payload.Status == "active" || payload.Status == "grace") && (skew > maxClockSkew || skew < -maxClockSkew) {
		err := fmt.Errorf("license answer verified at %s is more than a day away from this host's clock", payload.VerifiedAt.Format(time.RFC3339))
		m.logger.Error("license answer ignored, check this host's clock", "error", err)
		m.handleNetworkError(ctx, err)
		return false
	}

	switch payload.Status {
	case "active":
		m.applyPayload(ctx, payload)
		if err := writeCache(m.dataDir, resp); err != nil {
			m.logger.Error("failed to write license cache", "error", err)
		}
		m.logger.Info("license verified", "plan", payload.Plan, "expires_at", payload.ExpiresAt)

	case "grace":
		m.applyPayload(ctx, payload)
		if err := writeCache(m.dataDir, resp); err != nil {
			m.logger.Error("failed to write license cache", "error", err)
		}
		m.logger.Warn("license in grace period",
			"plan", payload.Plan,
			"expires_at", payload.ExpiresAt,
		)
		// Update message for frontend banner without mutating the shared pointer.
		stateCopy := *m.State()
		stateCopy.Message = "Your license is in a grace period. Please renew to avoid service interruption."
		m.setStateAndNotify(ctx, &stateCopy)

	case "expired":
		m.logger.Warn("license expired", "expires_at", payload.ExpiresAt)
		deleteCache(m.dataDir)
		m.setStateAndNotify(ctx, &State{
			Status:    "expired",
			ExpiresAt: payload.ExpiresAt,
			Message:   "Your license has expired. This instance has fallen back to the Community edition.",
		})

	case "revoked":
		m.logger.Error("license revoked")
		deleteCache(m.dataDir)
		m.setStateAndNotify(ctx, &State{
			Status:  "revoked",
			Message: "Your license has been revoked.",
		})

	default:
		m.logger.Warn("unknown license status from server", "status", payload.Status)
		m.setStateAndNotify(ctx, &State{
			Status:  payload.Status,
			Message: "Unexpected license status: " + payload.Status,
		})
	}
	return true
}

// applyRefusal acts on a 401 or 403 only when its body is signed by the license server and does not grant the license, and reports whether it did.
func (m *Manager) applyRefusal(ctx context.Context, statusCode int, resp *SignedResponse, httpErr error) bool {
	if resp == nil {
		m.logger.Error("license refusal is not signed, ignoring it", "http_status", statusCode)
		return false
	}
	payload, err := verify(m.publicKey, *resp)
	if err != nil {
		m.logger.Error("license refusal signature verification failed, ignoring it", "http_status", statusCode, "error", err)
		return false
	}
	if payload.Status == "active" || payload.Status == "grace" {
		m.logger.Error("license refusal carries a granting payload, ignoring it", "http_status", statusCode, "status", payload.Status)
		return false
	}

	message := payload.Message
	if message == "" {
		message = httpErr.Error()
	}

	if statusCode == http.StatusUnauthorized {
		m.logger.Error("license key not recognized by server", "message", message)
		deleteCache(m.dataDir)
		m.setStateAndNotify(ctx, &State{Status: "unknown", Message: message})
		return true
	}

	status := payload.Status
	if status == "" {
		status = "expired"
	}
	m.logger.Warn("license no longer active", "status", status, "message", message)
	deleteCache(m.dataDir)
	m.setStateAndNotify(ctx, &State{Status: status, Message: message})
	return true
}

// applyPayload sets the license state from a verified payload.
//
// The state is built, bridled, then written exactly once: a second
// setStateAndNotify would dispatch a phantom edition change.
func (m *Manager) applyPayload(ctx context.Context, payload *LicensePayload) {
	state := &State{
		Edition:    ResolveEdition(payload, m.logger),
		Plan:       payload.Plan,
		Features:   payload.Features,
		Status:     payload.Status,
		VerifiedAt: payload.VerifiedAt,
		ExpiresAt:  payload.ExpiresAt,
	}
	m.recordSignedTime(payload.VerifiedAt)
	m.enforceUpdateWindow(state, payload, m.now())
	m.setStateAndNotify(ctx, state)
}

// recordSignedTime keeps the latest server time a verified payload carried, with the local monotonic reading taken when it was seen.
func (m *Manager) recordSignedTime(t time.Time) {
	if m.signedTimeSeen.IsZero() || t.After(m.signedTime.Add(time.Since(m.signedTimeSeen))) {
		m.signedTime, m.signedTimeSeen = t, time.Now()
	}
}

// now reads the wall clock, but never earlier than the latest signed server time plus the monotonic time elapsed since it was seen.
func (m *Manager) now() time.Time {
	now := time.Now()
	if m.signedTimeSeen.IsZero() {
		return now
	}
	if floor := m.signedTime.Add(time.Since(m.signedTimeSeen)); floor.After(now) {
		return floor
	}
	return now
}

// enforceUpdateWindow bridles state when the running build was released after
// the license's update window closed. A Personal license never expires; only
// the right to new versions is bounded, and it is bounded per binary, which is
// why this is decided here and not in ResolveEdition. That function is pure,
// and the grace clock is persistent state.
//
// It is the only writer of the window record.
func (m *Manager) enforceUpdateWindow(state *State, payload *LicensePayload, now time.Time) {
	windowEnd, ok := payload.updateWindowEnd()
	if !ok {
		return
	}
	state.UpdatesUntil = windowEnd

	// Nothing to bridle: the license grants no edition to take away.
	if state.Edition == "" || state.Edition == extension.Community {
		return
	}
	if !m.buildDateKnown {
		return
	}

	// The covered case, and the reason going back is free: it returns before
	// reading the record, so a rollback reads nothing, writes nothing and
	// consumes nothing. The record is not deleted either, so alternating
	// rollback and update harvests no grace.
	if !m.buildDate.After(windowEnd) {
		return
	}

	record, mustWrite := reconcileWindow(readWindowRecord(m.dataDir), windowEnd, now, m.version, m.buildDate.Format(time.RFC3339))
	if mustWrite {
		if err := writeWindowRecord(m.dataDir, record); err != nil {
			m.logger.Error("failed to write the update window record", "error", err)
		}
	}

	graceEnd := updateGraceEnd(record)
	state.UpdateGraceUntil = graceEnd

	if now.Before(graceEnd) {
		state.Status = StatusUpdateWindowGrace
		state.Message = updateWindowGraceMessage(windowEnd, graceEnd, now)
		m.logger.Warn("this build was released after the update window closed",
			"updates_until", windowEnd,
			"build_date", m.buildDate,
			"community_from", graceEnd,
		)
		return
	}

	state.Status = StatusUpdateWindowEnded
	state.Edition = extension.Community
	state.Message = updateWindowEndedMessage(windowEnd)
	m.logger.Error("the update window grace is spent, falling back to the Community edition",
		"updates_until", windowEnd,
		"build_date", m.buildDate,
	)
}

// handleNetworkError applies graceful degradation based on cache age.
// It takes ctx because every write goes through setStateAndNotify: degradation
// past the 60-day window drops the edition, and that transition must be
// dispatched like any other.
func (m *Manager) handleNetworkError(ctx context.Context, err error) {
	current := m.State()

	// If we have no valid state, just log and remain in community mode
	if current.VerifiedAt.IsZero() {
		m.logger.Error("license server unreachable and no cached license", "error", err)
		m.setStateAndNotify(ctx, &State{
			Status:  "unreachable",
			Message: "Cannot reach license server. Please check your network connection.",
		})
		return
	}

	// The cache stays: only the server can tell a renewal from an end.
	now := m.now()
	if fallback := offlineFallback(current.VerifiedAt, current.ExpiresAt, now); fallback != nil {
		m.logger.Error("license server unreachable and the cached license no longer grants an edition",
			"error", err,
			"status", fallback.Status,
			"verified_at", current.VerifiedAt,
			"expires_at", current.ExpiresAt,
		)
		m.setStateAndNotify(ctx, fallback)
		return
	}

	age := now.Sub(current.VerifiedAt)

	switch {
	case age > graceDegradationError:
		daysLeft := int((graceDegradationDisabled - age).Hours() / 24)
		m.logger.Error("license server unreachable for over 30 days",
			"error", err,
			"verified_at", current.VerifiedAt,
			"days_until_disabled", daysLeft,
		)
		degraded := *current
		degraded.Message = "License server unreachable. This instance falls back to the Community edition in " +
			formatDays(daysLeft) + "."
		m.replayUpdateWindow(&degraded)
		m.setStateAndNotify(ctx, &degraded)

	case age > graceDegradationWarn:
		m.logger.Warn("license server unreachable for over 7 days",
			"error", err,
			"verified_at", current.VerifiedAt,
		)
		degraded := *current
		degraded.Message = "License server unreachable. Your edition remains active from cache."
		m.replayUpdateWindow(&degraded)
		m.setStateAndNotify(ctx, &degraded)

	default:
		m.logger.Info("license server unreachable, using cached license",
			"error", err,
			"verified_at", current.VerifiedAt,
		)
		degraded := *current
		m.replayUpdateWindow(&degraded)
		m.setStateAndNotify(ctx, &degraded)
	}
}

// replayUpdateWindow re-applies the window verdict to a state built offline. It
// runs after the degradation message is set, so the window has the last word on
// both status and message: it is the more urgent of the two.
//
// readCache re-checks the signature, so the date it yields is the one the online
// path would have used. Without this the grace clock stops the moment the
// network does, and an operator who updates then unplugs keeps the edition until
// the next restart.
func (m *Manager) replayUpdateWindow(state *State) {
	cached, err := readCache(m.dataDir, m.publicKey)
	if err != nil || cached == nil {
		return
	}
	m.enforceUpdateWindow(state, cached, m.now())
}

// offlineFallback returns the Community state a cached license falls to at now, or nil while it still grants its edition.
func offlineFallback(verifiedAt, expiresAt, now time.Time) *State {
	switch {
	case !expiresAt.IsZero() && now.After(expiresAt):
		return &State{
			Status:     "expired",
			VerifiedAt: verifiedAt,
			ExpiresAt:  expiresAt,
			Message:    "Your license has expired. This instance has fallen back to the Community edition.",
		}
	case now.Sub(verifiedAt) > graceDegradationDisabled:
		return &State{
			Status:     "unreachable",
			VerifiedAt: verifiedAt,
			Message:    "License server unreachable for over 60 days. This instance has fallen back to the Community edition.",
		}
	}
	return nil
}

func formatDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}
