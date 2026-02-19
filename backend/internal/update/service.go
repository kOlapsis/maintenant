package update

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UpdateEnricher enriches raw scan results with additional data.
// CE: no-op (returns nil). Pro: runs enrichment pipeline (CVE, changelog, risk).
type UpdateEnricher interface {
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

// Service orchestrates update detection and notification.
type Service struct {
	store         UpdateStore
	scanner       *Scanner
	containers    ContainerLister
	enricher      UpdateEnricher
	logger        *slog.Logger
	eventCallback EventCallback
	alertChan     chan<- interface{}
	interval      time.Duration

	mu           sync.RWMutex
	lastScanTime time.Time
	nextScanTime time.Time
	scanning     bool
}

// NewService creates the update intelligence service.
func NewService(store UpdateStore, scanner *Scanner, containers ContainerLister, logger *slog.Logger) *Service {
	interval := 24 * time.Hour
	if v := os.Getenv("PULSEBOARD_UPDATE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	return &Service{
		store:      store,
		scanner:    scanner,
		containers: containers,
		logger:     logger,
		interval:   interval,
		enricher:   noopUpdateEnricher{},
	}
}

// SetEnricher sets the update enricher (no-op in CE, CVE/changelog/risk in Pro).
func (s *Service) SetEnricher(e UpdateEnricher) {
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

// Start begins the periodic scan loop. Blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	s.logger.Info("starting update intelligence service", "interval", s.interval)

	// Run first scan after a short delay to let containers start
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
func (s *Service) TriggerScan(ctx context.Context) (int64, error) {
	s.mu.RLock()
	if s.scanning {
		s.mu.RUnlock()
		return 0, fmt.Errorf("scan already in progress")
	}
	s.mu.RUnlock()

	go s.runScan(ctx)
	// Return a placeholder; the scan record will be created in runScan
	return 0, nil
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
		return fmt.Sprintf("docker compose pull %s && docker compose up -d %s", c.OrchestrationUnit, c.OrchestrationUnit)
	}

	// Standalone Docker container
	return fmt.Sprintf("docker pull %s:%s && docker stop %s && docker rm %s && docker run -d --name %s %s:%s",
		repo, latestTag, c.Name, c.Name, c.Name, repo, latestTag)
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

	// Create scan record
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
	s.emitEvent("update.scan_started", map[string]interface{}{
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

	// Persist results
	updatesFound := 0
	for _, r := range results {
		if !r.HasUpdate {
			continue
		}

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
			RiskScore:     BaseRiskScore(r.UpdateType),
			Status:        UpdateStatusAvailable,
			DetectedAt:    time.Now(),
		}

		if _, err := s.store.InsertImageUpdate(ctx, u); err != nil {
			s.logger.Error("update scan: persist update", "container", r.ContainerName, "error", err)
			continue
		}

		updatesFound++
		s.emitEvent("update.detected", map[string]interface{}{
			"container_id":   r.ContainerID,
			"container_name": r.ContainerName,
			"image":          r.Image,
			"current_tag":    r.CurrentTag,
			"latest_tag":     r.LatestTag,
			"update_type":    string(r.UpdateType),
		})
	}

	// Enrichment pipeline (no-op in CE, runs CVE/changelog/risk in Pro)
	if err := s.enricher.Enrich(ctx, results); err != nil {
		s.logger.Warn("update enrichment failed", "error", err)
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

	s.emitEvent("update.scan_completed", map[string]interface{}{
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
func (s *Service) GetScanRecord(ctx context.Context, id int64) (*ScanRecord, error) {
	return s.store.GetScanRecord(ctx, id)
}

// GetLatestScanRecord returns the most recent scan record.
func (s *Service) GetLatestScanRecord(ctx context.Context) (*ScanRecord, error) {
	return s.store.GetLatestScanRecord(ctx)
}

// ParseScanID parses a scan ID from a string.
func ParseScanID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
