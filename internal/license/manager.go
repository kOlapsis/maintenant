// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package license

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
)

const (
	checkInterval = 24 * time.Hour

	// Graceful degradation windows: how long Pro stays active when the
	// license server is unreachable, based on cache age.
	graceDegradationWarn     = 7 * 24 * time.Hour  // 7 days
	graceDegradationError    = 30 * 24 * time.Hour // 30 days
	graceDegradationDisabled = 60 * 24 * time.Hour // 60 days
)

// State represents the current license status, safe to read concurrently.
// A zero Edition means Community: no usable license.
type State struct {
	Edition    extension.Edition `json:"edition"`
	Plan       string            `json:"plan,omitempty"`
	Features   []string          `json:"features,omitempty"`
	Status     string            `json:"status"`
	VerifiedAt time.Time         `json:"verified_at,omitempty"`
	ExpiresAt  time.Time         `json:"expires_at,omitempty"`
	Message    string            `json:"message,omitempty"`
}

// EditionChangeCallback is invoked after the manager observes a transition
// between two distinct editions. prev and next are the old and new editions.
// The callback is invoked from a dedicated goroutine with a 30-second timeout;
// it must not block. RegisterEditionChangeCallback must be called before Start.
type EditionChangeCallback func(ctx context.Context, prev, next extension.Edition)

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
func NewManager(licenseKey, dataDir, version string, logger *slog.Logger) (*Manager, error) {
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
		client:     &http.Client{Timeout: 10 * time.Second},
		stop:       make(chan struct{}),
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
	// Run initial check (non-blocking on failure)
	m.check(ctx)

	go m.ticker(ctx)
}

// loadCache populates state from the disk cache, if a valid one exists.
func (m *Manager) loadCache(ctx context.Context) {
	cached, err := readCache(m.dataDir, m.publicKey)
	if err != nil || cached == nil {
		return
	}
	if !cached.ExpiresAt.IsZero() && time.Now().After(cached.ExpiresAt) {
		m.logger.Warn("cached license has expired, ignoring cache",
			"expires_at", cached.ExpiresAt,
		)
		deleteCache(m.dataDir)
		m.setStateAndNotify(ctx, &State{
			Status:    "expired",
			ExpiresAt: cached.ExpiresAt,
			Message:   "Your license has expired. This instance has fallen back to the Community edition.",
		})
		return
	}
	m.applyPayload(ctx, cached)
	m.logger.Info("license loaded from cache",
		"status", cached.Status,
		"plan", cached.Plan,
		"verified_at", cached.VerifiedAt,
	)
}

// Stop stops the background ticker.
func (m *Manager) Stop() {
	close(m.stop)
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

	for _, cb := range cbs {
		go func(f EditionChangeCallback) {
			cbCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("license: callback panic", "recover", r)
				}
			}()
			f(cbCtx, prev, next)
		}(cb)
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

func (m *Manager) ticker(ctx context.Context) {
	t := time.NewTicker(checkInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stop:
			return
		case <-t.C:
			m.check(ctx)
		}
	}
}

func (m *Manager) check(ctx context.Context) {
	serverURL := getLicenseServerURL()
	resp, statusCode, err := fetchLicense(ctx, m.client, serverURL, m.licenseKey, m.version)

	if err != nil {
		// HTTP 401 = unknown/invalid key — use server message verbatim
		if statusCode == http.StatusUnauthorized {
			var srvErr *ServerError
			if !errors.As(err, &srvErr) {
				srvErr = &ServerError{Message: err.Error()}
			}
			m.logger.Error("license key not recognized by server", "error", err)
			deleteCache(m.dataDir)
			m.setStateAndNotify(ctx, &State{
				Status:  "unknown",
				Message: srvErr.Message,
			})
			return
		}

		// HTTP 403 = license no longer active — use server status and message verbatim
		if statusCode == http.StatusForbidden {
			var srvErr *ServerError
			if !errors.As(err, &srvErr) {
				srvErr = &ServerError{Status: "expired", Message: err.Error()}
			}
			m.logger.Warn("license no longer active", "status", srvErr.Status, "error", err)
			deleteCache(m.dataDir)
			m.setStateAndNotify(ctx, &State{
				Status:  srvErr.Status,
				Message: srvErr.Message,
			})
			return
		}

		// Network error or other non-200: graceful degradation
		m.handleNetworkError(ctx, err)
		return
	}

	// Verify signature
	payload, err := verify(m.publicKey, *resp)
	if err != nil {
		// Invalid signature = treat as network error (keep current state)
		m.logger.Error("license signature verification failed", "error", err)
		m.handleNetworkError(ctx, err)
		return
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
}

// applyPayload sets the license state from a verified payload.
func (m *Manager) applyPayload(ctx context.Context, payload *LicensePayload) {
	m.setStateAndNotify(ctx, &State{
		Edition:    ResolveEdition(payload, m.logger),
		Plan:       payload.Plan,
		Features:   payload.Features,
		Status:     payload.Status,
		VerifiedAt: payload.VerifiedAt,
		ExpiresAt:  payload.ExpiresAt,
	})
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

	// If the cached license has already passed its expiry date, report it as
	// expired — not as unreachable. Graceful degradation must not hide expiry.
	if !current.ExpiresAt.IsZero() && time.Now().After(current.ExpiresAt) {
		m.logger.Warn("cached license has expired, disabling Pro",
			"expires_at", current.ExpiresAt,
		)
		deleteCache(m.dataDir)
		m.setStateAndNotify(ctx, &State{
			Status:    "expired",
			ExpiresAt: current.ExpiresAt,
			Message:   "Your license has expired. This instance has fallen back to the Community edition.",
		})
		return
	}

	age := time.Since(current.VerifiedAt)

	switch {
	case age > graceDegradationDisabled:
		m.logger.Error("license server unreachable for over 60 days, disabling Pro",
			"error", err,
			"verified_at", current.VerifiedAt,
		)
		deleteCache(m.dataDir)
		m.setStateAndNotify(ctx, &State{
			Status:     "unreachable",
			VerifiedAt: current.VerifiedAt,
			Message:    "License server unreachable for over 60 days. This instance has fallen back to the Community edition.",
		})

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
		m.setStateAndNotify(ctx, &degraded)

	case age > graceDegradationWarn:
		m.logger.Warn("license server unreachable for over 7 days",
			"error", err,
			"verified_at", current.VerifiedAt,
		)
		degraded := *current
		degraded.Message = "License server unreachable. Your edition remains active from cache."
		m.setStateAndNotify(ctx, &degraded)

	default:
		m.logger.Info("license server unreachable, using cached license",
			"error", err,
			"verified_at", current.VerifiedAt,
		)
	}
}

func formatDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}
