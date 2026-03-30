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
	"sync/atomic"
	"time"
)

const (
	checkInterval = 24 * time.Hour

	// Graceful degradation windows: how long Pro stays active when the
	// license server is unreachable, based on cache age.
	graceDegradationWarn     = 7 * 24 * time.Hour  // 7 days
	graceDegradationError    = 30 * 24 * time.Hour // 30 days
	graceDegradationDisabled = 60 * 24 * time.Hour // 60 days
)

// LicenseState represents the current license status, safe to read concurrently.
type LicenseState struct {
	IsProEnabled bool      `json:"is_pro_enabled"`
	Plan         string    `json:"plan,omitempty"`
	Features     []string  `json:"features,omitempty"`
	Status       string    `json:"status"`
	VerifiedAt   time.Time `json:"verified_at,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Message      string    `json:"message,omitempty"`
}

// LicenseManager handles license verification, caching, and state management.
type LicenseManager struct {
	licenseKey string
	dataDir    string
	version    string
	logger     *slog.Logger
	publicKey  ed25519.PublicKey
	client     *http.Client
	state      atomic.Value // *LicenseState
	stop       chan struct{}
}

// NewLicenseManager creates a new license manager. Call Start() to begin
// periodic verification.
func NewLicenseManager(licenseKey, dataDir, version string, logger *slog.Logger) (*LicenseManager, error) {
	pubKey, err := getPublicKey()
	if err != nil {
		return nil, err
	}

	m := &LicenseManager{
		licenseKey: licenseKey,
		dataDir:    dataDir,
		version:    version,
		logger:     logger.With("component", "license"),
		publicKey:  pubKey,
		client:     &http.Client{Timeout: 10 * time.Second},
		stop:       make(chan struct{}),
	}

	// Start in community mode
	m.state.Store(&LicenseState{Status: "unknown"})

	return m, nil
}

// Start performs an initial license check, then starts a background ticker.
// Non-blocking: errors during the initial check are logged, not fatal.
func (m *LicenseManager) Start(ctx context.Context) {
	// Try loading from cache first for fast startup.
	// Discard the cache if the licence has already passed its expiry date.
	if cached, err := readCache(m.dataDir, m.publicKey); err == nil && cached != nil {
		if !cached.ExpiresAt.IsZero() && time.Now().After(cached.ExpiresAt) {
			m.logger.Warn("cached license has expired, ignoring cache",
				"expires_at", cached.ExpiresAt,
			)
			deleteCache(m.dataDir)
			m.state.Store(&LicenseState{
				Status:    "expired",
				ExpiresAt: cached.ExpiresAt,
				Message:   "Your license has expired. Pro features have been disabled.",
			})
		} else {
			m.applyPayload(cached)
			m.logger.Info("license loaded from cache",
				"status", cached.Status,
				"plan", cached.Plan,
				"verified_at", cached.VerifiedAt,
			)
		}
	}

	// Run initial check (non-blocking on failure)
	m.check(ctx)

	go m.ticker(ctx)
}

// Stop stops the background ticker.
func (m *LicenseManager) Stop() {
	close(m.stop)
}

// State returns the current license state (thread-safe).
func (m *LicenseManager) State() *LicenseState {
	return m.state.Load().(*LicenseState)
}

// IsProEnabled returns true if the current license enables Pro features.
func (m *LicenseManager) IsProEnabled() bool {
	return m.State().IsProEnabled
}

func (m *LicenseManager) ticker(ctx context.Context) {
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

func (m *LicenseManager) check(ctx context.Context) {
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
			m.state.Store(&LicenseState{
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
			m.state.Store(&LicenseState{
				Status:  srvErr.Status,
				Message: srvErr.Message,
			})
			return
		}

		// Network error or other non-200: graceful degradation
		m.handleNetworkError(err)
		return
	}

	// Verify signature
	payload, err := verify(m.publicKey, *resp)
	if err != nil {
		// Invalid signature = treat as network error (keep current state)
		m.logger.Error("license signature verification failed", "error", err)
		m.handleNetworkError(err)
		return
	}

	switch payload.Status {
	case "active":
		m.applyPayload(payload)
		if err := writeCache(m.dataDir, resp); err != nil {
			m.logger.Error("failed to write license cache", "error", err)
		}
		m.logger.Info("license verified", "plan", payload.Plan, "expires_at", payload.ExpiresAt)

	case "grace":
		m.applyPayload(payload)
		if err := writeCache(m.dataDir, resp); err != nil {
			m.logger.Error("failed to write license cache", "error", err)
		}
		m.logger.Warn("license in grace period",
			"plan", payload.Plan,
			"expires_at", payload.ExpiresAt,
		)
		// Update message for frontend banner
		state := m.State()
		state.Message = "Your license is in a grace period. Please renew to avoid service interruption."
		m.state.Store(state)

	case "expired":
		m.logger.Warn("license expired", "expires_at", payload.ExpiresAt)
		deleteCache(m.dataDir)
		m.state.Store(&LicenseState{
			Status:    "expired",
			ExpiresAt: payload.ExpiresAt,
			Message:   "Your license has expired. Pro features have been disabled.",
		})

	case "revoked":
		m.logger.Error("license revoked")
		deleteCache(m.dataDir)
		m.state.Store(&LicenseState{
			Status:  "revoked",
			Message: "Your license has been revoked.",
		})

	default:
		m.logger.Warn("unknown license status from server", "status", payload.Status)
		m.state.Store(&LicenseState{
			Status:  payload.Status,
			Message: "Unexpected license status: " + payload.Status,
		})
	}
}

// applyPayload sets the license state from a verified payload.
func (m *LicenseManager) applyPayload(payload *LicensePayload) {
	m.state.Store(&LicenseState{
		IsProEnabled: true,
		Plan:         payload.Plan,
		Features:     payload.Features,
		Status:       payload.Status,
		VerifiedAt:   payload.VerifiedAt,
		ExpiresAt:    payload.ExpiresAt,
	})
}

// handleNetworkError applies graceful degradation based on cache age.
func (m *LicenseManager) handleNetworkError(err error) {
	current := m.State()

	// If we have no valid state, just log and remain in community mode
	if current.VerifiedAt.IsZero() {
		m.logger.Error("license server unreachable and no cached license", "error", err)
		m.state.Store(&LicenseState{
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
		m.state.Store(&LicenseState{
			Status:    "expired",
			ExpiresAt: current.ExpiresAt,
			Message:   "Your license has expired. Pro features have been disabled.",
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
		m.state.Store(&LicenseState{
			Status:     "unreachable",
			VerifiedAt: current.VerifiedAt,
			Message:    "License server unreachable for over 60 days. Pro features have been disabled.",
		})

	case age > graceDegradationError:
		daysLeft := int((graceDegradationDisabled - age).Hours() / 24)
		m.logger.Error("license server unreachable for over 30 days",
			"error", err,
			"verified_at", current.VerifiedAt,
			"days_until_disabled", daysLeft,
		)
		current.Message = "License server unreachable. Pro features will be disabled in " +
			formatDays(daysLeft) + "."
		m.state.Store(current)

	case age > graceDegradationWarn:
		m.logger.Warn("license server unreachable for over 7 days",
			"error", err,
			"verified_at", current.VerifiedAt,
		)
		current.Message = "License server unreachable. Pro features remain active from cache."
		m.state.Store(current)

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
