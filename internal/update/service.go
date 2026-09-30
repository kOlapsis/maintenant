// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

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

// Enricher enriches raw scan results with CVE, changelog and risk data, as far as the running edition opens them.
type Enricher interface {
	Enrich(ctx context.Context, results []UpdateResult) error
}

// noopUpdateEnricher is the default when no enricher is plugged in.
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

// SetEnricher sets the update enricher.
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

// GenerateUpdateCommand produces a shell command to update a container to latestTag, whose manifest is latestDigest.
func (s *Service) GenerateUpdateCommand(c ContainerInfo, latestTag, latestDigest string) string {
	repo, currentTag, _ := ParseImageRef(c.Image)

	if c.RuntimeType == "kubernetes" || c.SwarmService != "" {
		ref := repo + ":" + latestTag
		// An unchanged image reference can leave the workload as it is, so a republished tag is deployed by its digest.
		if latestTag == currentTag && latestDigest != "" {
			ref += "@" + latestDigest
		}
		if c.SwarmService != "" {
			return swarmServiceUpdate(c, ref)
		}
		return kubectlSetImage(c, ref)
	}

	// Docker Compose
	if isCompose(c) {
		// Compose pulls the tag written in the compose file, so a new tag has to be written there first.
		if latestTag != currentTag {
			return fmt.Sprintf("cd %s\n# Set the image of service %s to %s:%s in the compose file, then:\ndocker compose pull %s\ndocker compose up -d %s",
				composeDir(c), c.OrchestrationUnit, repo, latestTag, c.OrchestrationUnit, c.OrchestrationUnit)
		}
		return fmt.Sprintf("cd %s\ndocker compose pull %s\ndocker compose up -d --force-recreate %s",
			composeDir(c), c.OrchestrationUnit, c.OrchestrationUnit)
	}

	// Standalone Docker container
	return fmt.Sprintf("docker pull %s:%s\ndocker stop %s && docker rm %s\ndocker run -d --name %s %s:%s",
		repo, latestTag, c.Name, c.Name, c.Name, repo, latestTag)
}

// GenerateRollbackCommand produces a shell command that puts a container back on the image it ran before the update, or "" when that image cannot be named.
func (s *Service) GenerateRollbackCommand(c ContainerInfo, u *ImageUpdate) string {
	ref := previousImageRef(u.Image, u.CurrentTag, u.PreviousDigest)
	if ref == "" {
		return ""
	}

	if c.RuntimeType == "kubernetes" {
		return kubectlSetImage(c, ref)
	}
	if c.SwarmService != "" {
		return swarmServiceUpdate(c, ref)
	}

	// Docker Compose
	if isCompose(c) {
		svc := c.OrchestrationUnit
		if u.LatestTag == u.CurrentTag {
			// The compose file keeps the same tag: point that tag back at the previous image locally.
			return fmt.Sprintf("cd %s\ndocker pull %s\ndocker tag %s %s\ndocker compose up -d --pull never --force-recreate %s",
				composeDir(c), ref, ref, imageWithoutDigest(u.Image), svc)
		}
		return fmt.Sprintf("cd %s\n# Set the image of service %s back to %s in the compose file, then:\ndocker compose up -d %s",
			composeDir(c), svc, ref, svc)
	}

	// Standalone Docker container
	return fmt.Sprintf("docker pull %s\ndocker stop %s && docker rm %s\ndocker run -d --name %s %s",
		ref, c.Name, c.Name, c.Name, ref)
}

// previousImageRef names the image a container ran before an update: by digest when known,
// else by its tag when that tag is a fixed release; a moving tag without digest cannot name it.
func previousImageRef(image, currentTag, previousDigest string) string {
	repo, _, _ := ParseImageRef(image)
	if previousDigest != "" {
		return repo + "@" + previousDigest
	}
	if isFixedVersionTag(currentTag) {
		return repo + ":" + currentTag
	}
	return ""
}

func imageWithoutDigest(image string) string {
	if i := strings.Index(image, "@"); i > 0 {
		return image[:i]
	}
	return image
}

// kubectlSetImage points the workload's container at ref; a pod without a controller is updated in place.
func kubectlSetImage(c ContainerInfo, ref string) string {
	kind := strings.ToLower(c.ControllerKind)
	if kind == "" {
		kind = "pod"
	}
	return fmt.Sprintf("kubectl set image %s/%s %s=%s -n %s",
		kind, c.OrchestrationUnit, podContainer(c), ref, c.OrchestrationGroup)
}

// swarmServiceUpdate points the Swarm service that runs the task at ref.
func swarmServiceUpdate(c ContainerInfo, ref string) string {
	return fmt.Sprintf("docker service update --image %s %s", ref, c.SwarmService)
}

// podContainer names the container of a Kubernetes pod that runs the workload's image.
func podContainer(c ContainerInfo) string {
	if c.PodContainer != "" {
		return c.PodContainer
	}
	return c.Name
}

func isCompose(c ContainerInfo) bool {
	return c.RuntimeType != "kubernetes" && c.OrchestrationGroup != "" && c.OrchestrationUnit != ""
}

func composeDir(c ContainerInfo) string {
	if c.ComposeWorkingDir == "" {
		return "<compose-project-dir>"
	}
	return c.ComposeWorkingDir
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

	return s.GenerateUpdateCommand(c, fixedInVersion, "")
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

	// A container whose scan failed keeps its pending update until a scan reaches its registry.
	failed := make(map[string]bool, len(scanErrors))
	for _, se := range scanErrors {
		failed[se.ContainerName] = true
	}
	scannedNames := make([]string, 0, len(containers))
	for _, c := range containers {
		if !failed[c.Name] {
			scannedNames = append(scannedNames, c.Name)
		}
	}

	// Persist results
	updatesFound := 0
	for _, r := range results {
		if !r.HasUpdate {
			continue
		}

		riskScore := BaseRiskScore(r.UpdateType)
		u := &ImageUpdate{
			ScanID:         scanID,
			ContainerID:    r.ContainerID,
			ContainerName:  r.ContainerName,
			Image:          r.Image,
			CurrentTag:     r.CurrentTag,
			CurrentDigest:  r.CurrentDigest,
			Registry:       r.Registry,
			LatestTag:      r.LatestTag,
			LatestDigest:   r.LatestDigest,
			UpdateType:     r.UpdateType,
			RiskScore:      riskScore,
			PreviousDigest: r.PreviousDigest,
			Status:         StatusAvailable,
			DetectedAt:     time.Now(),
		}

		if _, err := s.store.InsertImageUpdate(ctx, u); err != nil {
			s.logger.Error("update scan: persist update", "container", r.ContainerName, "error", err)
			continue
		}

		updatesFound++

		eventData := map[string]interface{}{
			"container_id":   r.ContainerID,
			"container_uid":  containerByID[r.ContainerID].UID,
			"container_name": r.ContainerName,
			"image":          r.Image,
			"current_tag":    r.CurrentTag,
			"latest_tag":     r.LatestTag,
			"update_type":    string(r.UpdateType),
			"risk_score":     riskScore,
			"alert_on":       r.AlertOn,
		}

		if ci, ok := containerByID[r.ContainerID]; ok {
			eventData["update_command"] = s.GenerateUpdateCommand(ci, r.LatestTag, r.LatestDigest)
			if cmd := s.GenerateRollbackCommand(ci, u); cmd != "" {
				eventData["rollback_command"] = cmd
			}
		}

		s.emitEvent(event.UpdateDetected, eventData)
	}

	// Enrichment pipeline, as far as the edition opens it: a CVE pass on every container,
	// then changelog and risk on those with an update.
	if targets := enrichmentTargets(containers, results); len(targets) > 0 {
		s.logger.Info("starting enrichment pipeline", "containers", len(targets), "updates", updatesFound)
		if err := s.enricher.Enrich(ctx, targets); err != nil {
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

	// Same for the findings whose container is gone entirely: matching on the
	// name never sheds those, because Swarm renames the task container on every
	// deploy. Skipped when the scan saw nothing, so a runtime that is momentarily
	// unreachable doesn't wipe the whole page.
	if len(containers) > 0 {
		// Deleting without the list would strand the matching alerts, with
		// nothing left to announce their recovery — so leave them to the next
		// scan. The Updates page already hides them in the meantime.
		orphans, err := s.store.ListOrphanImageUpdates(ctx)
		if err != nil {
			s.logger.Warn("update scan: list orphan updates", "error", err)
		} else {
			if deleted, err := s.store.DeleteOrphanImageUpdates(ctx); err != nil {
				s.logger.Warn("update scan: cleanup orphan updates", "error", err)
			} else if deleted > 0 {
				s.logger.Info("update scan: removed updates for containers that no longer exist", "deleted", deleted)
			}
			staleUpdates = append(staleUpdates, orphans...)
		}
	}

	for _, su := range staleUpdates {
		containerUID := su.ContainerUID
		if containerUID == "" {
			containerUID = containerByID[su.ContainerID].UID
		}
		s.emitEvent(event.UpdateResolved, map[string]interface{}{
			"container_id":   su.ContainerID,
			"container_uid":  containerUID,
			"container_name": su.ContainerName,
		})
	}

	s.completeScan(ctx, scanRecord, ScanStatusCompleted, len(containers), updatesFound, len(scanErrors))
}

// enrichmentTargets returns the scan results, followed by the running image of every container they do not cover.
func enrichmentTargets(containers []ContainerInfo, results []UpdateResult) []UpdateResult {
	covered := make(map[string]bool, len(results))
	for _, r := range results {
		covered[r.ContainerID] = true
	}
	targets := append(make([]UpdateResult, 0, len(containers)), results...)
	for _, c := range containers {
		if !covered[c.ExternalID] {
			targets = append(targets, runningImage(c))
		}
	}
	return targets
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
