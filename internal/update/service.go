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

package update

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
)

// Enricher enriches raw scan results with additional data.
// CE: no-op (returns nil). Pro: runs an enrichment pipeline (CVE, changelog, risk).
type Enricher interface {
	Enrich(ctx context.Context, results []UpdateResult) error
}

// noopUpdateEnricher is the CE default — skips enrichment silently.
type noopUpdateEnricher struct{}

func (noopUpdateEnricher) Enrich(_ context.Context, _ []UpdateResult) error {
	return nil
}

// EventCallback is the function signature for SSE event broadcasting.
type EventCallback func(eventType string, data interface{})

// ContainerLister provides the list of containers to scan.
type ContainerLister interface {
	ListContainerInfos(ctx context.Context) ([]ContainerInfo, error)
}

// Deps holds all dependencies for the update Service.
type Deps struct {
	Store         UpdateStore        // required
	Scanner       *Scanner           // required
	Containers    ContainerLister    // required
	Logger        *slog.Logger       // required
	Enricher      Enricher           // optional — defaults to no-op
	EventCallback EventCallback      // optional — nil-safe
	AlertChan     chan<- interface{} // optional — nil-safe
}

// Service orchestrates update detection and notification.
type Service struct {
	store         UpdateStore
	scanner       *Scanner
	containers    ContainerLister
	enricher      Enricher
	logger        *slog.Logger
	eventCallback EventCallback
	alertChan     chan<- interface{}
	interval      time.Duration

	mu           sync.RWMutex
	lastScanTime time.Time
	nextScanTime time.Time
	scanning     bool
	appCtx       context.Context // application-scoped context for background scans
}

// NewService creates the update intelligence service.
func NewService(d Deps) *Service {
	if d.Store == nil {
		panic("update.NewService: Store is required")
	}
	if d.Scanner == nil {
		panic("update.NewService: Scanner is required")
	}
	if d.Containers == nil {
		panic("update.NewService: Containers is required")
	}
	if d.Logger == nil {
		panic("update.NewService: Logger is required")
	}
	enricher := d.Enricher
	if enricher == nil {
		enricher = noopUpdateEnricher{}
	}
	interval := 24 * time.Hour
	if v := os.Getenv("MAINTENANT_UPDATE_INTERVAL"); v != "" {
		if dd, err := time.ParseDuration(v); err == nil && dd > 0 {
			interval = dd
		}
	}
	return &Service{
		store:         d.Store,
		scanner:       d.Scanner,
		containers:    d.Containers,
		logger:        d.Logger,
		interval:      interval,
		enricher:      enricher,
		eventCallback: d.EventCallback,
		alertChan:     d.AlertChan,
	}
}

// SetEnricher sets the update enricher (no-op in CE, CVE/changelog/risk in Pro).
func (s *Service) SetEnricher(e Enricher) {
	s.enricher = e
}

// SetAlertChannel sets the alert engine's event channel for critical notifications.
func (s *Service) SetAlertChannel(ch chan<- interface{}) {
	s.alertChan = ch
}

// SetEventCallback sets the SSE broadcasting callback.
func (s *Service) SetEventCallback(fn EventCallback) {
	s.eventCallback = fn
}

// Start begins the periodic scan loop. Blocks until ctx is canceled.
func (s *Service) Start(ctx context.Context) {
	s.appCtx = ctx
	s.logger.Info("starting update intelligence service", "interval", s.interval)

	// Run the first scan after a short delay to let containers start
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}

	s.runScan(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runScan(ctx)
		}
	}
}

// TriggerScan starts an immediate scan. Returns the scan ID.
// The scan runs with the application-scoped context (not the HTTP request context),
// so it survives after the triggering request completes.
func (s *Service) TriggerScan(_ context.Context) (string, error) {
	s.mu.RLock()
	if s.scanning {
		s.mu.RUnlock()
		return "", fmt.Errorf("scan already in progress")
	}
	s.mu.RUnlock()

	ctx := s.appCtx
	if ctx == nil {
		ctx = context.Background()
	}
	go s.runScan(ctx)
	return "", nil
}

// GetLastScanTime returns when the last scan completed.
func (s *Service) GetLastScanTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastScanTime
}

// GetNextScanTime returns when the next scan is scheduled.
func (s *Service) GetNextScanTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nextScanTime
}

// IsScanning returns whether a scan is currently in progress.
func (s *Service) IsScanning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scanning
}

// GetUpdateSummary returns the aggregated update counts.
func (s *Service) GetUpdateSummary(ctx context.Context) (*UpdateSummary, error) {
	return s.store.GetUpdateSummary(ctx)
}

// GetImageUpdateByContainer returns the latest update for a container.
func (s *Service) GetImageUpdateByContainer(ctx context.Context, containerID string) (*ImageUpdate, error) {
	return s.store.GetImageUpdateByContainer(ctx, containerID)
}

// ListImageUpdates returns filtered updates.
func (s *Service) ListImageUpdates(ctx context.Context, opts ListImageUpdatesOpts) ([]*ImageUpdate, error) {
	return s.store.ListImageUpdates(ctx, opts)
}

// GenerateUpdateCommand produces a shell command to update a container.
func (s *Service) GenerateUpdateCommand(c ContainerInfo, latestTag string) string {
	repo, _, _ := parseImageRef(c.Image)

	// Kubernetes workloads
	if c.RuntimeType == "kubernetes" && c.ControllerKind != "" {
		kind := strings.ToLower(c.ControllerKind)
		return fmt.Sprintf("kubectl set image %s/%s %s=%s:%s -n %s",
			kind, c.OrchestrationUnit, c.Name, repo, latestTag, c.OrchestrationGroup)
	}

	// Docker Compose
	if c.RuntimeType != "kubernetes" && c.OrchestrationGroup != "" && c.OrchestrationUnit != "" {
		dir := c.ComposeWorkingDir
		if dir == "" {
			dir = "<compose-project-dir>"
		}
		return fmt.Sprintf("cd %s\ndocker compose pull %s\ndocker compose up -d --force-recreate %s",
			dir, c.OrchestrationUnit, c.OrchestrationUnit)
	}

	// Standalone Docker container
	return fmt.Sprintf("docker pull %s:%s\ndocker stop %s && docker rm %s\ndocker run -d --name %s %s:%s",
		repo, latestTag, c.Name, c.Name, c.Name, repo, latestTag)
}

// GenerateRollbackCommand produces a shell command to revert a container to its previous image digest.
func (s *Service) GenerateRollbackCommand(c ContainerInfo, previousDigest string) string {
	if previousDigest == "" {
		return ""
	}

	repo, _, _ := parseImageRef(c.Image)

	// Kubernetes workloads — use rollout undo
	if c.RuntimeType == "kubernetes" && c.ControllerKind != "" {
		kind := strings.ToLower(c.ControllerKind)
		return fmt.Sprintf("kubectl rollout undo %s/%s -n %s",
			kind, c.OrchestrationUnit, c.OrchestrationGroup)
	}

	// Docker Compose — recreate with previous digest
	if c.RuntimeType != "kubernetes" && c.OrchestrationGroup != "" && c.OrchestrationUnit != "" {
		dir := c.ComposeWorkingDir
		if dir == "" {
			dir = "<compose-project-dir>"
		}
		return fmt.Sprintf("cd %s\ndocker compose pull %s\ndocker compose up -d --force-recreate %s",
			dir, c.OrchestrationUnit, c.OrchestrationUnit)
	}

	// Standalone Docker container — stop/rm/run with digest reference
	return fmt.Sprintf("docker stop %s && docker rm %s\ndocker run -d --name %s %s@%s",
		c.Name, c.Name, c.Name, repo, previousDigest)
}

// GenerateFixCommand produces a shell command to update a container to a specific CVE fix version.
// Returns empty string if fixedInVersion is not a valid semver or is <= currentTag (prevents downgrades).
func (s *Service) GenerateFixCommand(c ContainerInfo, currentTag, fixedInVersion string) string {
	if fixedInVersion == "" {
		return ""
	}

	fixVer, err := ParseTag(fixedInVersion)
	if err != nil {
		return ""
	}

	currentVer, err := ParseTag(currentTag)
	if err != nil {
		return ""
	}

	// Prevent downgrades: only generate a command if a fix version > current
	if !fixVer.GreaterThan(currentVer) {
		return ""
	}

	return s.GenerateUpdateCommand(c, fixedInVersion)
}

// IsFixedByUpdate returns true when the latest available tag already covers the CVE fix version.
func (s *Service) IsFixedByUpdate(latestTag, fixedInVersion string) bool {
	if fixedInVersion == "" || latestTag == "" {
		return false
	}

	latestVer, err := ParseTag(latestTag)
	if err != nil {
		return false
	}

	fixVer, err := ParseTag(fixedInVersion)
	if err != nil {
		return false
	}

	return !latestVer.LessThan(fixVer) // latest >= fixedIn
}

func (s *Service) runScan(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	s.mu.Lock()
	if s.scanning {
		s.mu.Unlock()
		return
	}
	s.scanning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.scanning = false
		s.lastScanTime = time.Now()
		s.nextScanTime = s.lastScanTime.Add(s.interval)
		s.mu.Unlock()
	}()

	s.logger.Info("starting update scan")

	// Create a scan record
	scanRecord := &ScanRecord{
		StartedAt: time.Now(),
		Status:    ScanStatusRunning,
	}
	scanID, err := s.store.InsertScanRecord(ctx, scanRecord)
	if err != nil {
		s.logger.Error("update scan: create record", "error", err)
		return
	}
	scanRecord.ID = scanID

	// Broadcast scan started
	s.emitEvent(event.UpdateScanStarted, map[string]interface{}{
		"scan_id":    scanID,
		"started_at": scanRecord.StartedAt,
	})

	// Get container list
	containers, err := s.containers.ListContainerInfos(ctx)
	if err != nil {
		s.logger.Error("update scan: list containers", "error", err)
		s.completeScan(ctx, scanRecord, ScanStatusFailed, 0, 0, 1)
		return
	}

	// Run scan
	results, scanErrors := s.scanner.Scan(ctx, containers)

	// Build container lookup for command generation
	containerByID := make(map[string]ContainerInfo, len(containers))
	for _, c := range containers {
		containerByID[c.ExternalID] = c
	}

	// Collect scanned container names for stale update cleanup
	scannedNames := make([]string, 0, len(containers))
	for _, c := range containers {
		scannedNames = append(scannedNames, c.Name)
	}

	// Persist results
	updatesFound := 0
	for _, r := range results {
		if !r.HasUpdate {
			continue
		}

		riskScore := BaseRiskScore(r.UpdateType)
		u := &ImageUpdate{
			ScanID:        scanID,
			ContainerID:   r.ContainerID,
			ContainerName: r.ContainerName,
			Image:         r.Image,
			CurrentTag:    r.CurrentTag,
			CurrentDigest: r.CurrentDigest,
			Registry:      r.Registry,
			LatestTag:     r.LatestTag,
			LatestDigest:  r.LatestDigest,
			UpdateType:    r.UpdateType,
			RiskScore:     riskScore,
			Status:        StatusAvailable,
			DetectedAt:    time.Now(),
		}

		if _, err := s.store.InsertImageUpdate(ctx, u); err != nil {
			s.logger.Error("update scan: persist update", "container", r.ContainerName, "error", err)
			continue
		}

		updatesFound++

		eventData := map[string]interface{}{
			"container_id":   r.ContainerID,
			"container_name": r.ContainerName,
			"image":          r.Image,
			"current_tag":    r.CurrentTag,
			"latest_tag":     r.LatestTag,
			"update_type":    string(r.UpdateType),
			"risk_score":     riskScore,
		}

		if ci, ok := containerByID[r.ContainerID]; ok {
			eventData["update_command"] = s.GenerateUpdateCommand(ci, r.LatestTag)
			if r.CurrentDigest != "" {
				eventData["rollback_command"] = s.GenerateRollbackCommand(ci, r.CurrentDigest)
			}
		}

		s.emitEvent(event.UpdateDetected, eventData)
	}

	// Enrichment pipeline (no-op in CE, runs CVE/changelog/risk in Pro)
	if updatesFound > 0 {
		s.logger.Info("starting enrichment pipeline", "updates", updatesFound)
		if err := s.enricher.Enrich(ctx, results); err != nil {
			s.logger.Warn("update enrichment failed", "error", err)
		}
		s.logger.Info("enrichment pipeline completed")
	}

	// Remove stale updates: entries for scanned containers that were not refreshed
	// by this scan (container was upgraded and no longer has a pending update).
	// Emit a recovery event for each so listening consumers (alert engine) can
	// resolve the matching active alert.
	staleUpdates, err := s.store.ListStaleImageUpdates(ctx, scanID, scannedNames)
	if err != nil {
		s.logger.Warn("update scan: list stale updates", "error", err)
	}
	if deleted, err := s.store.DeleteStaleImageUpdates(ctx, scanID, scannedNames); err != nil {
		s.logger.Warn("update scan: cleanup stale updates", "error", err)
	} else if deleted > 0 {
		s.logger.Info("update scan: removed stale updates", "deleted", deleted)
	}
	for _, su := range staleUpdates {
		s.emitEvent(event.UpdateResolved, map[string]interface{}{
			"container_id":   su.ContainerID,
			"container_name": su.ContainerName,
		})
	}

	s.completeScan(ctx, scanRecord, ScanStatusCompleted, len(containers), updatesFound, len(scanErrors))
}

func (s *Service) completeScan(ctx context.Context, record *ScanRecord, status ScanStatus, scanned, found, errors int) {
	now := time.Now()
	record.CompletedAt = &now
	record.Status = status
	record.ContainersScanned = scanned
	record.UpdatesFound = found
	record.Errors = errors

	if err := s.store.UpdateScanRecord(ctx, record); err != nil {
		s.logger.Error("update scan: update record", "error", err)
	}

	s.emitEvent(event.UpdateScanCompleted, map[string]interface{}{
		"scan_id":       record.ID,
		"updates_found": found,
		"errors":        errors,
	})

	s.logger.Info("update scan completed",
		"containers_scanned", scanned,
		"updates_found", found,
		"errors", errors,
	)
}

func (s *Service) emitEvent(eventType string, data interface{}) {
	if s.eventCallback != nil {
		s.eventCallback(eventType, data)
	}
}

// GetScanRecord returns a scan record by ID.
func (s *Service) GetScanRecord(ctx context.Context, id string) (*ScanRecord, error) {
	return s.store.GetScanRecord(ctx, id)
}

// GetLatestScanRecord returns the most recent scan record.
func (s *Service) GetLatestScanRecord(ctx context.Context) (*ScanRecord, error) {
	return s.store.GetLatestScanRecord(ctx)
}
