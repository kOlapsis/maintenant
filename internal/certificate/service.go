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

package certificate

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
)

var (
	ErrMonitorNotFound     = errors.New("certificate monitor not found")
	ErrDuplicateMonitor    = errors.New("hostname:port already monitored")
	ErrAutoDetectedMonitor = errors.New("hostname:port already monitored via endpoint auto-detection")
	ErrInvalidInput        = errors.New("invalid input")
	ErrCannotDeleteAuto    = errors.New("cannot delete auto-detected certificate monitors")
	ErrLimitReached        = errors.New("certificate monitor limit reached")
)

// LicenseChecker determines license-gated capabilities for certificate monitors.
type LicenseChecker interface {
	CanCreateCertificate(currentCount int) bool
}

// DefaultLicenseChecker implements Community edition limits.
type DefaultLicenseChecker struct {
	MaxCertificates int
}

func (c *DefaultLicenseChecker) CanCreateCertificate(currentCount int) bool {
	return currentCount < c.MaxCertificates
}

// EventCallback is called when a certificate event occurs (for SSE broadcasting).
type EventCallback func(eventType string, data interface{})

// Deps holds all dependencies for the certificate Service.
type Deps struct {
	Store          CertificateStore // required
	Logger         *slog.Logger     // required
	LicenseChecker LicenseChecker   // optional — defaults to community limits
	EventCallback  EventCallback    // optional — nil-safe
}

// Service orchestrates certificate monitoring.
type Service struct {
	store          CertificateStore
	logger         *slog.Logger
	licenseChecker LicenseChecker
	onEvent        EventCallback
	mu             sync.Mutex
}

// NewService creates a new certificate service.
func NewService(d Deps) *Service {
	if d.Store == nil {
		panic("certificate.NewService: Store is required")
	}
	if d.Logger == nil {
		panic("certificate.NewService: Logger is required")
	}
	lc := d.LicenseChecker
	if lc == nil {
		lc = &DefaultLicenseChecker{MaxCertificates: 5}
	}
	return &Service{
		store:          d.Store,
		logger:         d.Logger,
		licenseChecker: lc,
		onEvent:        d.EventCallback,
	}
}

// SetEventCallback sets the callback for broadcasting certificate events.
func (s *Service) SetEventCallback(cb EventCallback) {
	s.onEvent = cb
}

func (s *Service) emit(eventType string, data interface{}) {
	if s.onEvent != nil {
		s.onEvent(eventType, data)
	}
}

// --- US1: Auto-detection ---

// EnsureAutoDetected creates or returns the existing auto-detected cert monitor
// for the given HTTPS endpoint.
func (s *Service) EnsureAutoDetected(ctx context.Context, endpointID int64, targetURL string) (*CertMonitor, error) {
	hostname, port, err := extractHostPort(targetURL)
	if err != nil {
		return nil, err
	}

	// Check if monitor already exists for this endpoint
	existing, err := s.store.GetMonitorByEndpointID(ctx, endpointID)
	if err != nil {
		return nil, fmt.Errorf("get monitor by endpoint: %w", err)
	}
	if existing != nil {
		s.logger.Debug("certificate: auto-detection skipped, monitor exists", "hostname", hostname, "port", port)
		return existing, nil
	}

	// Check if there's already a monitor for this hostname:port (including inactive)
	existing, err = s.store.GetMonitorByHostPort(ctx, hostname, port)
	if err != nil {
		return nil, fmt.Errorf("get monitor by host:port: %w", err)
	}
	if existing != nil {
		s.logger.Debug("certificate: auto-detection skipped, monitor exists", "hostname", hostname, "port", port)
		return existing, nil
	}

	// Create new auto-detected monitor
	monitor := &CertMonitor{
		Hostname:             hostname,
		Port:                 port,
		Source:               SourceAuto,
		EndpointID:           &endpointID,
		Status:               StatusUnknown,
		CheckIntervalSeconds: 43200,
		WarningThresholds:    DefaultWarningThresholds(),
	}

	_, err = s.store.CreateMonitor(ctx, monitor)
	if err != nil {
		// Race condition: another goroutine created the monitor concurrently.
		// Fall back to reading the existing one.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			existing, rerr := s.store.GetMonitorByHostPort(ctx, hostname, port)
			if rerr != nil {
				return nil, fmt.Errorf("create auto monitor: %w (fallback: %w)", err, rerr)
			}
			if existing != nil {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("create auto monitor: %w", err)
	}

	s.emit(event.CertificateCreated, map[string]interface{}{
		"monitor_id": monitor.ID,
		"hostname":   monitor.Hostname,
		"port":       monitor.Port,
		"source":     "auto",
	})

	return monitor, nil
}

// ProcessAutoDetectedCerts processes TLS certificates from an HTTP endpoint check.
func (s *Service) ProcessAutoDetectedCerts(ctx context.Context, endpointID int64, targetURL string, certs []*x509.Certificate) {
	if len(certs) == 0 {
		return
	}

	hostname, _, err := extractHostPort(targetURL)
	if err != nil {
		s.logger.Error("extract host:port from target", "error", err, "target", targetURL)
		return
	}

	monitor, err := s.EnsureAutoDetected(ctx, endpointID, targetURL)
	if err != nil {
		s.logger.Error("ensure auto-detected monitor", "error", err, "endpoint_id", endpointID)
		return
	}

	result := CheckCertificateFromPeerCerts(certs, hostname)
	s.processCheckResult(ctx, monitor, result)
}

// --- US2: Standalone ---

// CreateStandalone creates a standalone certificate monitor and runs the first check.
func (s *Service) CreateStandalone(ctx context.Context, input CreateCertificateInput) (*CertMonitor, *CertCheckResult, error) {
	if input.Hostname == "" {
		return nil, nil, fmt.Errorf("%w: hostname is required", ErrInvalidInput)
	}
	if input.Port <= 0 {
		input.Port = 443
	}
	if input.Port > 65535 {
		return nil, nil, fmt.Errorf("%w: port must be 1-65535", ErrInvalidInput)
	}
	if input.CheckIntervalSeconds <= 0 {
		input.CheckIntervalSeconds = 43200
	}
	if input.CheckIntervalSeconds < 3600 {
		return nil, nil, fmt.Errorf("%w: minimum check interval is 3600 seconds (1 hour)", ErrInvalidInput)
	}
	if input.CheckIntervalSeconds > 604800 {
		return nil, nil, fmt.Errorf("%w: maximum check interval is 604800 seconds (7 days)", ErrInvalidInput)
	}
	if len(input.WarningThresholds) == 0 {
		input.WarningThresholds = DefaultWarningThresholds()
	}

	// Check quota for standalone monitors
	count, err := s.store.CountStandaloneMonitors(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("count standalone monitors: %w", err)
	}

	// Check for existing monitor (FR-018)
	existing, err := s.store.GetMonitorByHostPort(ctx, input.Hostname, input.Port)
	if err != nil {
		return nil, nil, fmt.Errorf("check existing: %w", err)
	}
	monitor := &CertMonitor{
		Hostname:             input.Hostname,
		Port:                 input.Port,
		Source:               SourceStandalone,
		Status:               StatusUnknown,
		CheckIntervalSeconds: input.CheckIntervalSeconds,
		WarningThresholds:    input.WarningThresholds,
	}

	if existing != nil {
		if existing.Active {
			if existing.Source == SourceAuto {
				return nil, nil, ErrAutoDetectedMonitor
			}
			return nil, nil, ErrDuplicateMonitor
		}
		// Reactivate the soft-deleted monitor with the new settings.
		monitor.ID = existing.ID
		if err := s.store.ReactivateMonitor(ctx, existing.ID, monitor); err != nil {
			return nil, nil, fmt.Errorf("reactivate monitor: %w", err)
		}
	} else {
		// Check quota for new standalone monitors
		if !s.licenseChecker.CanCreateCertificate(count) {
			return nil, nil, ErrLimitReached
		}
		_, err = s.store.CreateMonitor(ctx, monitor)
		if err != nil {
			return nil, nil, fmt.Errorf("create standalone monitor: %w", err)
		}
	}

	s.emit(event.CertificateCreated, map[string]interface{}{
		"monitor_id": monitor.ID,
		"hostname":   monitor.Hostname,
		"port":       monitor.Port,
		"source":     "standalone",
	})

	// Run first check immediately
	checkResult := CheckCertificate(monitor.Hostname, monitor.Port, 10*time.Second)
	result := s.processCheckResult(ctx, monitor, checkResult)

	return monitor, result, nil
}

// UpdateMonitor updates a certificate monitor's settings.
func (s *Service) UpdateMonitor(ctx context.Context, id int64, input UpdateCertificateInput) (*CertMonitor, error) {
	monitor, err := s.store.GetMonitorByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get monitor: %w", err)
	}
	if monitor == nil {
		return nil, ErrMonitorNotFound
	}

	if input.CheckIntervalSeconds != nil {
		v := *input.CheckIntervalSeconds
		if v < 3600 || v > 604800 {
			return nil, fmt.Errorf("%w: check interval must be 3600-604800 seconds", ErrInvalidInput)
		}
		monitor.CheckIntervalSeconds = v
	}
	if len(input.WarningThresholds) > 0 {
		monitor.WarningThresholds = input.WarningThresholds
	}

	if err := s.store.UpdateMonitor(ctx, monitor); err != nil {
		return nil, fmt.Errorf("update monitor: %w", err)
	}

	return monitor, nil
}

// DeleteMonitor soft-deletes a standalone certificate monitor.
func (s *Service) DeleteMonitor(ctx context.Context, id int64) error {
	monitor, err := s.store.GetMonitorByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get monitor: %w", err)
	}
	if monitor == nil {
		return ErrMonitorNotFound
	}
	if monitor.Source == SourceAuto {
		return ErrCannotDeleteAuto
	}

	if err := s.store.SoftDeleteMonitor(ctx, id); err != nil {
		return fmt.Errorf("soft delete: %w", err)
	}

	s.emit(event.CertificateDeleted, map[string]interface{}{
		"monitor_id": id,
		"hostname":   monitor.Hostname,
	})

	return nil
}

// DeactivateByEndpointID soft-deletes the auto-detected cert monitor linked to an endpoint.
func (s *Service) DeactivateByEndpointID(ctx context.Context, endpointID int64) {
	monitor, err := s.store.GetMonitorByEndpointID(ctx, endpointID)
	if err != nil || monitor == nil {
		return
	}
	if err := s.store.SoftDeleteMonitor(ctx, monitor.ID); err != nil {
		s.logger.Error("deactivate cert monitor for removed endpoint", "monitor_id", monitor.ID, "endpoint_id", endpointID, "error", err)
		return
	}
	s.emit(event.CertificateDeleted, map[string]interface{}{
		"monitor_id": monitor.ID,
		"hostname":   monitor.Hostname,
	})
}

// --- Label-discovered monitors ---

// SyncFromLabels reconciles label-discovered certificate monitors for a container.
// It creates new monitors for hostnames present in labels, and deactivates monitors
// for hostnames that were removed from labels.
func (s *Service) SyncFromLabels(ctx context.Context, containerExternalID string, labels map[string]string) {
	parsed := ParseCertificateLabels(labels)
	if len(parsed) == 0 {
		// No TLS labels — deactivate any existing label monitors for this container
		s.deactivateLabelMonitors(ctx, containerExternalID, nil)
		return
	}

	// Track which hostname:port combos are desired
	desired := make(map[string]bool)
	for _, p := range parsed {
		key := p.Hostname + ":" + strconv.Itoa(p.Port)
		desired[key] = true

		existing, err := s.store.GetMonitorByHostPort(ctx, p.Hostname, p.Port)
		if err != nil {
			s.logger.Error("check existing cert monitor", "error", err, "hostname", p.Hostname)
			continue
		}

		monitor := &CertMonitor{
			Hostname:             p.Hostname,
			Port:                 p.Port,
			Source:               SourceLabel,
			ExternalID:           containerExternalID,
			Status:               StatusUnknown,
			CheckIntervalSeconds: 43200,
			WarningThresholds:    DefaultWarningThresholds(),
		}

		if existing != nil {
			if existing.Active {
				s.logger.Debug("certificate: label monitor exists, skipping", "hostname", p.Hostname, "port", p.Port)
				continue
			}
			// Reactivate the soft-deleted monitor.
			if err := s.store.ReactivateMonitor(ctx, existing.ID, monitor); err != nil {
				s.logger.Error("reactivate label cert monitor", "error", err, "hostname", p.Hostname, "port", p.Port)
			}
			continue
		}

		if _, err := s.store.CreateMonitor(ctx, monitor); err != nil {
			s.logger.Error("create label cert monitor", "error", err, "hostname", p.Hostname, "port", p.Port)
			continue
		}

		s.logger.Info("created label-discovered cert monitor", "hostname", p.Hostname, "port", p.Port, "container", containerExternalID)
		s.emit(event.CertificateCreated, map[string]interface{}{
			"monitor_id": monitor.ID,
			"hostname":   monitor.Hostname,
			"port":       monitor.Port,
			"source":     "label",
		})
	}

	// Deactivate label monitors for this container that are no longer in labels
	s.deactivateLabelMonitors(ctx, containerExternalID, desired)
}

// HandleContainerDestroy deactivates all label-discovered monitors for a destroyed container.
func (s *Service) HandleContainerDestroy(ctx context.Context, externalID string) {
	s.deactivateLabelMonitors(ctx, externalID, nil)
}

func (s *Service) deactivateLabelMonitors(ctx context.Context, externalID string, keep map[string]bool) {
	monitors, err := s.store.ListMonitorsByExternalID(ctx, externalID)
	if err != nil {
		s.logger.Error("list label monitors for container", "error", err, "external_id", externalID)
		return
	}

	for _, m := range monitors {
		key := m.Hostname + ":" + strconv.Itoa(m.Port)
		if keep != nil && keep[key] {
			continue
		}

		if err := s.store.DeactivateMonitor(ctx, m.ID); err != nil {
			s.logger.Error("deactivate label cert monitor", "error", err, "monitor_id", m.ID)
			continue
		}

		s.logger.Info("deactivated label cert monitor", "hostname", m.Hostname, "port", m.Port, "container", externalID)
		s.emit(event.CertificateDeleted, map[string]interface{}{
			"monitor_id": m.ID,
			"hostname":   m.Hostname,
		})
	}
}

// --- Query methods ---

// ListMonitors returns all active certificate monitors.
func (s *Service) ListMonitors(ctx context.Context, opts ListCertificatesOpts) ([]*CertMonitor, error) {
	return s.store.ListMonitors(ctx, opts)
}

// GetMonitor returns a certificate monitor by ID.
func (s *Service) GetMonitor(ctx context.Context, id int64) (*CertMonitor, error) {
	m, err := s.store.GetMonitorByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrMonitorNotFound
	}
	return m, nil
}

// GetLatestCheckResult returns the latest check result for a monitor.
func (s *Service) GetLatestCheckResult(ctx context.Context, monitorID int64) (*CertCheckResult, error) {
	return s.store.GetLatestCheckResult(ctx, monitorID)
}

// GetChainEntries returns the chain entries for a check result.
func (s *Service) GetChainEntries(ctx context.Context, checkResultID int64) ([]*CertChainEntry, error) {
	return s.store.GetChainEntries(ctx, checkResultID)
}

// ListCheckResults returns check result history for a monitor.
func (s *Service) ListCheckResults(ctx context.Context, monitorID int64, opts ListChecksOpts) ([]*CertCheckResult, int, error) {
	return s.store.ListCheckResults(ctx, monitorID, opts)
}

// --- US2: Scheduler ---

// Start starts a background goroutine that periodically checks
// standalone certificate monitors that are due for a check.
func (s *Service) Start(ctx context.Context) {
	s.logger.Info("certificate: scheduler started", "interval", "60s")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runScheduledChecks(ctx)
		}
	}
}

func (s *Service) runScheduledChecks(ctx context.Context) {
	monitors, err := s.store.ListDueScheduledMonitors(ctx, time.Now())
	if err != nil {
		s.logger.Error("list due standalone monitors", "error", err)
		return
	}

	s.logger.Debug("certificate: scheduled check run", "due_count", len(monitors))

	if len(monitors) == 0 {
		return
	}

	// Semaphore to limit concurrent checks
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, m := range monitors {
		sem <- struct{}{}
		wg.Add(1)
		go func(monitor *CertMonitor) {
			defer wg.Done()
			defer func() { <-sem }()

			s.logger.Debug("certificate: checking", "hostname", monitor.Hostname, "port", monitor.Port, "source", string(monitor.Source))
			result := CheckCertificate(monitor.Hostname, monitor.Port, 10*time.Second)
			s.processCheckResult(ctx, monitor, result)
		}(m)
	}

	wg.Wait()
}

// --- Core: Process check result + alerts ---

func (s *Service) processCheckResult(ctx context.Context, monitor *CertMonitor, raw *CheckCertificateResult) *CertCheckResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	result := &CertCheckResult{
		MonitorID: monitor.ID,
		CheckedAt: now,
	}

	if raw.Error != "" {
		// TLS connection failure
		result.ErrorMessage = raw.Error
		monitor.Status = StatusError
		monitor.LastError = raw.Error
	} else {
		result.SubjectCN = raw.SubjectCN
		result.IssuerCN = raw.IssuerCN
		result.IssuerOrg = raw.IssuerOrg
		result.SANs = raw.SANs
		result.SerialNumber = raw.SerialNumber
		result.SignatureAlgorithm = raw.SignatureAlgorithm
		result.NotBefore = &raw.NotBefore
		result.NotAfter = &raw.NotAfter
		result.ChainValid = &raw.ChainValid
		result.ChainError = raw.ChainError
		result.HostnameMatch = &raw.HostnameMatch

		// Determine status from cert data
		monitor.Status = s.computeStatus(raw, monitor.WarningThresholds)
		monitor.LastError = ""
	}

	monitor.LastCheckAt = &now

	// Update next_check_at for scheduled monitors (standalone + label)
	if monitor.Source == SourceStandalone || monitor.Source == SourceLabel {
		next := now.Add(time.Duration(monitor.CheckIntervalSeconds) * time.Second)
		monitor.NextCheckAt = &next
	}

	// Store check result
	checkID, err := s.store.InsertCheckResult(ctx, result)
	if err != nil {
		s.logger.Error("insert cert check result", "error", err, "monitor_id", monitor.ID)
	}

	// Store chain entries
	if len(raw.Chain) > 0 && checkID > 0 {
		entries := make([]*CertChainEntry, len(raw.Chain))
		for i, c := range raw.Chain {
			entries[i] = &CertChainEntry{
				CheckResultID: checkID,
				Position:      i,
				SubjectCN:     c.SubjectCN,
				IssuerCN:      c.IssuerCN,
				NotBefore:     c.NotBefore,
				NotAfter:      c.NotAfter,
			}
		}
		if err := s.store.InsertChainEntries(ctx, entries); err != nil {
			s.logger.Error("insert chain entries", "error", err, "monitor_id", monitor.ID)
		}
	}

	// Evaluate alerts (US3)
	previousStatus := monitor.Status
	s.evaluateAlerts(ctx, monitor, result)

	// Update monitor in DB
	if err := s.store.UpdateMonitor(ctx, monitor); err != nil {
		s.logger.Error("update cert monitor", "error", err, "monitor_id", monitor.ID)
	}

	s.logger.Debug("certificate: check result", "monitor_id", monitor.ID, "hostname", monitor.Hostname, "status", string(monitor.Status), "days_remaining", result.DaysRemaining())

	// Broadcast SSE events
	s.emitCheckCompleted(monitor, result)
	if previousStatus != monitor.Status {
		s.logger.Debug("certificate: status changed", "monitor_id", monitor.ID, "hostname", monitor.Hostname, "previous_status", string(previousStatus), "new_status", string(monitor.Status))
		s.emit(event.CertificateStatusChanged, map[string]interface{}{
			"monitor_id":      monitor.ID,
			"hostname":        monitor.Hostname,
			"previous_status": string(previousStatus),
			"new_status":      string(monitor.Status),
			"days_remaining":  result.DaysRemaining(),
			"timestamp":       now.Format(time.RFC3339),
		})
	}

	return result
}

func (s *Service) computeStatus(raw *CheckCertificateResult, thresholds []int) CertStatus {
	if raw.NotAfter.Before(time.Now()) {
		return StatusExpired
	}

	daysRemaining := int(time.Until(raw.NotAfter).Hours() / 24)

	// Sort thresholds descending
	sorted := make([]int, len(thresholds))
	copy(sorted, thresholds)
	sort.Sort(sort.Reverse(sort.IntSlice(sorted)))

	for _, threshold := range sorted {
		if daysRemaining <= threshold {
			return StatusExpiring
		}
	}

	return StatusValid
}

// --- US3: Alert evaluation ---

func (s *Service) evaluateAlerts(ctx context.Context, monitor *CertMonitor, result *CertCheckResult) {
	if result.ErrorMessage != "" {
		// Connection error — no cert data to evaluate
		return
	}

	// Check chain validation alerts
	if result.ChainValid != nil && !*result.ChainValid {
		s.emit(event.CertificateAlert, map[string]interface{}{
			"monitor_id":  monitor.ID,
			"hostname":    monitor.Hostname,
			"port":        monitor.Port,
			"alert_type":  "chain_invalid",
			"severity":    "critical",
			"chain_error": result.ChainError,
			"timestamp":   result.CheckedAt.Format(time.RFC3339),
		})
	}

	// Check hostname mismatch alerts
	if result.HostnameMatch != nil && !*result.HostnameMatch {
		s.emit(event.CertificateAlert, map[string]interface{}{
			"monitor_id": monitor.ID,
			"hostname":   monitor.Hostname,
			"port":       monitor.Port,
			"alert_type": "hostname_mismatch",
			"severity":   "critical",
			"timestamp":  result.CheckedAt.Format(time.RFC3339),
		})
	}

	if result.NotAfter == nil {
		return
	}

	daysRemaining := result.DaysRemaining()

	// Check if certificate has expired
	if result.NotAfter.Before(time.Now()) {
		s.emit(event.CertificateAlert, map[string]interface{}{
			"monitor_id":     monitor.ID,
			"hostname":       monitor.Hostname,
			"port":           monitor.Port,
			"alert_type":     "expired",
			"severity":       "critical",
			"not_after":      result.NotAfter.Format(time.RFC3339),
			"days_remaining": daysRemaining,
			"timestamp":      result.CheckedAt.Format(time.RFC3339),
		})
		return
	}

	// Sort thresholds descending (30, 14, 7, 3, 1)
	sorted := make([]int, len(monitor.WarningThresholds))
	copy(sorted, monitor.WarningThresholds)
	sort.Sort(sort.Reverse(sort.IntSlice(sorted)))

	// Find the highest threshold that's been crossed
	var crossedThreshold *int
	for _, threshold := range sorted {
		if daysRemaining <= threshold {
			crossedThreshold = &threshold
		}
	}

	if crossedThreshold == nil {
		s.logger.Debug("certificate: alert evaluation, no action", "monitor_id", monitor.ID, "hostname", monitor.Hostname, "days_remaining", daysRemaining)
		// No threshold crossed — check if we need a recovery alert
		if monitor.LastAlertedThreshold != nil {
			// Certificate renewed, past all thresholds
			s.emit(event.CertificateRecovery, map[string]interface{}{
				"monitor_id":          monitor.ID,
				"hostname":            monitor.Hostname,
				"port":                monitor.Port,
				"previous_alert_type": "expiring",
				"new_not_after":       result.NotAfter.Format(time.RFC3339),
				"days_remaining":      daysRemaining,
				"timestamp":           result.CheckedAt.Format(time.RFC3339),
			})
			monitor.LastAlertedThreshold = nil
		}
		return
	}

	s.logger.Debug("certificate: alert threshold crossed", "monitor_id", monitor.ID, "hostname", monitor.Hostname, "days_remaining", daysRemaining, "threshold", *crossedThreshold)

	// Check if we need to escalate (fire alert at a new, lower threshold)
	if monitor.LastAlertedThreshold == nil || *crossedThreshold < *monitor.LastAlertedThreshold {
		s.emit(event.CertificateAlert, map[string]interface{}{
			"monitor_id":     monitor.ID,
			"hostname":       monitor.Hostname,
			"port":           monitor.Port,
			"alert_type":     "expiring",
			"severity":       ExpiringSeverity(daysRemaining, result.IssuerOrg),
			"threshold_days": *crossedThreshold,
			"days_remaining": daysRemaining,
			"not_after":      result.NotAfter.Format(time.RFC3339),
			"issuer_org":     result.IssuerOrg,
			"auto_renewable": IsAutoRenewable(result.IssuerOrg),
			"timestamp":      result.CheckedAt.Format(time.RFC3339),
		})
		monitor.LastAlertedThreshold = crossedThreshold
	}
}

func (s *Service) emitCheckCompleted(monitor *CertMonitor, result *CertCheckResult) {
	data := map[string]interface{}{
		"monitor_id": monitor.ID,
		"hostname":   monitor.Hostname,
		"status":     string(monitor.Status),
		"checked_at": result.CheckedAt.Format(time.RFC3339),
	}

	if result.SubjectCN != "" {
		data["subject_cn"] = result.SubjectCN
	}
	if result.IssuerCN != "" {
		data["issuer_cn"] = result.IssuerCN
	}
	if result.NotAfter != nil {
		data["not_after"] = result.NotAfter.Format(time.RFC3339)
		data["days_remaining"] = result.DaysRemaining()
	}
	if result.ChainValid != nil {
		data["chain_valid"] = *result.ChainValid
	}
	if result.HostnameMatch != nil {
		data["hostname_match"] = *result.HostnameMatch
	}

	s.emit(event.CertificateCheckCompleted, data)
}

// --- Helpers ---

func extractHostPort(targetURL string) (string, int, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return "", 0, fmt.Errorf("parse URL: %w", err)
	}

	if u.Scheme != "https" {
		return "", 0, fmt.Errorf("not an HTTPS URL: %s", targetURL)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return "", 0, fmt.Errorf("no hostname in URL: %s", targetURL)
	}

	port := 443
	if p := u.Port(); p != "" {
		port, err = strconv.Atoi(p)
		if err != nil {
			return "", 0, fmt.Errorf("invalid port: %w", err)
		}
	}

	return hostname, port, nil
}

// IsHTTPS isHTTPS checks if a target URL uses the HTTPS scheme.
func IsHTTPS(target string) bool {
	return strings.HasPrefix(target, "https://")
}
