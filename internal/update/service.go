// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/uid"
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

// EOLTableProvider serves the support table in force.
type EOLTableProvider interface {
	Current() eol.Table
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
	EOL           EOLTableProvider   // optional, nil-safe
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
	eolTables     EOLTableProvider
	interval      time.Duration

	mu           sync.RWMutex
	lastScanTime time.Time
	nextScanTime time.Time
	scanning     bool
	appCtx       context.Context // application-scoped context for background scans
	triggered    sync.WaitGroup
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
		eolTables:     d.EOL,
	}
}

// SetEventCallback sets the SSE broadcasting callback.
func (s *Service) SetEventCallback(fn EventCallback) {
	s.eventCallback = fn
}

// Start begins the periodic scan loop. Blocks until ctx is canceled.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.appCtx = ctx
	s.mu.Unlock()
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
	ctx, scanning := s.appCtx, s.scanning
	s.mu.RUnlock()
	if scanning {
		return "", fmt.Errorf("scan already in progress")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.triggered.Go(func() { s.runScan(ctx) })
	return "", nil
}

// Wait blocks until the scans started by TriggerScan have returned.
func (s *Service) Wait() {
	s.triggered.Wait()
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

// GetAgentImageUpdate returns the latest update for the container containerID runs on agentID.
func (s *Service) GetAgentImageUpdate(ctx context.Context, agentID, containerID string) (*ImageUpdate, error) {
	return s.store.GetAgentImageUpdate(ctx, agentID, containerID)
}

// ListImageUpdates returns filtered updates.
func (s *Service) ListImageUpdates(ctx context.Context, opts ListImageUpdatesOpts) ([]*ImageUpdate, error) {
	return s.store.ListImageUpdates(ctx, opts)
}

// GenerateUpdateCommand produces a shell command to update a container to latestTag, whose manifest is latestDigest.
func (s *Service) GenerateUpdateCommand(c ContainerInfo, latestTag, latestDigest string) string {
	if unsafeFields(c) {
		return ""
	}
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
		svc := shellQuote(c.OrchestrationUnit)
		// Compose pulls the tag written in the compose file, so a new tag has to be written there first.
		if latestTag != currentTag {
			return fmt.Sprintf("cd %s\n# Set the image of service %s to %s in the compose file, then:\ndocker compose pull %s\ndocker compose up -d %s",
				composeDir(c), svc, shellQuote(repo+":"+latestTag), svc, svc)
		}
		return fmt.Sprintf("cd %s\ndocker compose pull %s\ndocker compose up -d --force-recreate %s",
			composeDir(c), svc, svc)
	}

	// Standalone Docker container
	ref, name := shellQuote(repo+":"+latestTag), shellQuote(c.Name)
	return fmt.Sprintf("docker pull %s\ndocker stop %s && docker rm %s\ndocker run -d --name %s %s",
		ref, name, name, name, ref)
}

// GenerateRollbackCommand produces a shell command that puts a container back on the image it ran before the update, or "" when that image cannot be named.
func (s *Service) GenerateRollbackCommand(c ContainerInfo, u *ImageUpdate) string {
	if unsafeFields(c) {
		return ""
	}
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
		svc, qref := shellQuote(c.OrchestrationUnit), shellQuote(ref)
		if u.LatestTag == u.CurrentTag {
			// The compose file keeps the same tag: point that tag back at the previous image locally.
			return fmt.Sprintf("cd %s\ndocker pull %s\ndocker tag %s %s\ndocker compose up -d --pull never --force-recreate %s",
				composeDir(c), qref, qref, shellQuote(imageWithoutDigest(u.Image)), svc)
		}
		return fmt.Sprintf("cd %s\n# Set the image of service %s back to %s in the compose file, then:\ndocker compose up -d %s",
			composeDir(c), svc, qref, svc)
	}

	// Standalone Docker container
	ref, name := shellQuote(ref), shellQuote(c.Name)
	return fmt.Sprintf("docker pull %s\ndocker stop %s && docker rm %s\ndocker run -d --name %s %s",
		ref, name, name, name, ref)
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
		shellQuote(kind), shellQuote(c.OrchestrationUnit), shellQuote(podContainer(c)), shellQuote(ref), shellQuote(c.OrchestrationGroup))
}

// swarmServiceUpdate points the Swarm service that runs the task at ref.
func swarmServiceUpdate(c ContainerInfo, ref string) string {
	return fmt.Sprintf("docker service update --image %s %s", shellQuote(ref), shellQuote(c.SwarmService))
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
	d := c.ComposeWorkingDir
	if d == "" || !filepath.IsAbs(d) || filepath.Clean(d) != d || hasControl(d) {
		return "<compose-project-dir>"
	}
	return shellQuote(d)
}

// shellQuote leaves a plain word as is and wraps anything else in POSIX single quotes.
func shellQuote(s string) string {
	plainRune := func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_./:@-", r)
	}
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !plainRune(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

// unsafeFields reports a control character in a value written into a command, which quoting cannot neutralise.
func unsafeFields(c ContainerInfo) bool {
	return slices.ContainsFunc([]string{c.Name, c.OrchestrationUnit, c.OrchestrationGroup, c.SwarmService, c.PodContainer, c.ControllerKind}, hasControl)
}

var plainRelease = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)

// GenerateFixCommand produces a shell command to update a container to the release that fixes a CVE, or "" when fixedInVersion is not a plain release above currentTag.
func (s *Service) GenerateFixCommand(c ContainerInfo, currentTag, fixedInVersion string) string {
	fixed := strings.TrimPrefix(fixedInVersion, "v")
	if !plainRelease.MatchString(fixed) {
		return ""
	}
	versionPart, variant := splitVariant(currentTag)
	fixVer, err := ParseTag(fixed)
	if err != nil {
		return ""
	}
	currentVer, err := ParseTag(versionPart)
	if err != nil || !fixVer.GreaterThan(currentVer) {
		return ""
	}
	if hasVPrefix(versionPart) {
		fixed = "v" + fixed
	}
	return s.GenerateUpdateCommand(c, fixed+variant, "")
}

// IsFixedByUpdate returns true when the latest available tag already covers the CVE fix version.
func IsFixedByUpdate(latestTag, fixedInVersion string) bool {
	if fixedInVersion == "" || latestTag == "" {
		return false
	}

	versionPart, _ := splitVariant(latestTag)
	latestVer, err := ParseTag(versionPart)
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
	results, scanErrors, unreadable := s.scanner.scan(ctx, containers)

	// Build container lookup for command generation
	containerByID := make(map[string]ContainerInfo, len(containers))
	for _, c := range containers {
		containerByID[uid.Container(uid.Agent(c.AgentID), c.ExternalID)] = c
	}

	// A container whose scan failed or whose image the registry would not show keeps its pending update until a scan reads its registry.
	type hostName struct{ host, name string }
	errored := make(map[string]bool, len(scanErrors))
	for _, se := range scanErrors {
		errored[se.ContainerID] = true
	}
	failed := make(map[hostName]bool, len(scanErrors)+len(unreadable))
	for _, c := range unreadable {
		failed[hostName{uid.Agent(c.AgentID), c.Name}] = true
	}
	for _, c := range containers {
		if errored[c.ExternalID] {
			failed[hostName{uid.Agent(c.AgentID), c.Name}] = true
		}
	}
	scannedNames := make([]string, 0, len(containers))
	var coveredHosts []string
	for _, c := range containers {
		host := uid.Agent(c.AgentID)
		if !slices.Contains(coveredHosts, host) {
			coveredHosts = append(coveredHosts, host)
		}
		if !failed[hostName{host, c.Name}] {
			scannedNames = append(scannedNames, c.Name)
		}
	}

	// Persist results
	type persisted struct {
		index  int
		stored *ImageUpdate
	}
	var detected []persisted
	for i := range results {
		r := &results[i]
		if !r.HasUpdate {
			continue
		}

		r.RiskScore = s.scanScore(*r)
		u := &ImageUpdate{
			ScanID:         scanID,
			ContainerID:    r.ContainerID,
			AgentID:        r.AgentID,
			ContainerName:  r.ContainerName,
			Image:          r.Image,
			CurrentTag:     r.CurrentTag,
			CurrentDigest:  r.CurrentDigest,
			Registry:       r.Registry,
			LatestTag:      r.LatestTag,
			LatestDigest:   r.LatestDigest,
			MajorTag:       r.MajorTag,
			UpdateType:     r.UpdateType,
			RiskScore:      r.RiskScore,
			PreviousDigest: r.PreviousDigest,
			Status:         StatusAvailable,
			DetectedAt:     time.Now(),
		}

		if _, err := s.store.InsertImageUpdate(ctx, u); err != nil {
			s.logger.Error("update scan: persist update", "container", r.ContainerName, "error", err)
			continue
		}
		detected = append(detected, persisted{index: i, stored: u})
	}
	updatesFound := len(detected)

	// Enrichment pipeline, as far as the edition opens it: a CVE pass on every container,
	// then changelog and risk on those with an update.
	targets := enrichmentTargets(containers, results)
	if len(targets) > 0 {
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
	// Only the hosts this scan covered are cleaned, so an absent host keeps its entries.
	var staleUpdates []StaleImageUpdate
	for _, host := range coveredHosts {
		// A name read on another host also cleans the entry of a container stopped on this one.
		names := slices.DeleteFunc(slices.Clone(scannedNames), func(name string) bool { return failed[hostName{host, name}] })
		stale, err := s.store.ListStaleImageUpdates(ctx, scanID, []string{host}, names)
		if err != nil {
			s.logger.Warn("update scan: list stale updates", "agent_id", host, "error", err)
			continue
		}
		staleUpdates = append(staleUpdates, stale...)
		if deleted, err := s.store.DeleteStaleImageUpdates(ctx, scanID, []string{host}, names); err != nil {
			s.logger.Warn("update scan: cleanup stale updates", "agent_id", host, "error", err)
		} else if deleted > 0 {
			s.logger.Info("update scan: removed stale updates", "agent_id", host, "deleted", deleted)
		}
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
		agentID := uid.Agent(su.AgentID)
		containerUID := su.ContainerUID
		if containerUID == "" {
			containerUID = containerByID[uid.Container(agentID, su.ContainerID)].UID
		}
		if containerUID == "" {
			containerUID = uid.Container(agentID, su.ContainerID)
		}
		s.emitEvent(event.UpdateResolved, map[string]interface{}{
			"container_id":   su.ContainerID,
			"container_uid":  containerUID,
			"container_name": su.ContainerName,
			"agent_id":       agentID,
		})
	}

	for _, d := range detected {
		r := targets[d.index]
		eventData := map[string]interface{}{
			"container_id":   r.ContainerID,
			"container_uid":  r.ContainerUID,
			"container_name": r.ContainerName,
			"agent_id":       r.AgentID,
			"image":          r.Image,
			"current_tag":    r.CurrentTag,
			"latest_tag":     r.LatestTag,
			"update_type":    string(r.UpdateType),
			"risk_score":     r.RiskScore,
			"alert_on":       r.AlertOn,
		}
		if r.MajorTag != "" {
			eventData["major_tag"] = r.MajorTag
		}

		if ci, ok := containerByID[uid.Container(r.AgentID, r.ContainerID)]; ok {
			eventData["update_command"] = s.GenerateUpdateCommand(ci, r.LatestTag, r.LatestDigest)
			if cmd := s.GenerateRollbackCommand(ci, d.stored); cmd != "" {
				eventData["rollback_command"] = cmd
			}
		}

		s.emitEvent(event.UpdateDetected, eventData)
	}

	s.completeScan(ctx, scanRecord, ScanStatusCompleted, len(containers), updatesFound, len(scanErrors))
}

// scanScore rates the urgency of an update by its type, raised when support for the running version has ended or ends soon.
func (s *Service) scanScore(r UpdateResult) int {
	score := BaseRiskScore(r.UpdateType)
	if s.eolTables == nil {
		return score
	}
	repo, _, _ := ParseImageRef(r.Image)
	switch eol.EvaluateImage(repo, r.CurrentTag, s.eolTables.Current(), time.Now()).State {
	case eol.StateEnded:
		return max(score, 85)
	case eol.StateEndingSoon:
		return max(score, 65)
	}
	return score
}

// ImageSupport is the support state of the release line one running container's image names.
type ImageSupport struct {
	ContainerID   string      `json:"container_id"`
	ContainerName string      `json:"container_name"`
	AgentID       string      `json:"agent_id"`
	Image         string      `json:"image"`
	Support       eol.Support `json:"support"`
}

// ListImageSupport returns the support state of every running image the support table tracks, whether or not an update is pending.
func (s *Service) ListImageSupport(ctx context.Context) ([]ImageSupport, error) {
	out := []ImageSupport{}
	if s.eolTables == nil {
		return out, nil
	}
	containers, err := s.containers.ListContainerInfos(ctx)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	table, today := s.eolTables.Current(), time.Now()
	for _, c := range containers {
		if c.LocallyBuilt {
			continue
		}
		repo, tag, _ := ParseImageRef(c.Image)
		support := eol.EvaluateImage(repo, tag, table, today)
		if support.State == eol.StateUntracked {
			continue
		}
		out = append(out, ImageSupport{
			ContainerID:   c.ExternalID,
			ContainerName: c.Name,
			AgentID:       uid.Agent(c.AgentID),
			Image:         c.Image,
			Support:       support,
		})
	}
	slices.SortFunc(out, func(a, b ImageSupport) int {
		return cmp.Or(strings.Compare(a.ContainerName, b.ContainerName), strings.Compare(a.AgentID, b.AgentID))
	})
	return out, nil
}

// enrichmentTargets returns the scan results in their order, followed by the running image of every container they do not cover.
func enrichmentTargets(containers []ContainerInfo, results []UpdateResult) []UpdateResult {
	covered := make(map[string]bool, len(results))
	for _, r := range results {
		covered[uid.Container(uid.Agent(r.AgentID), r.ContainerID)] = true
	}
	targets := append(make([]UpdateResult, 0, len(containers)), results...)
	for _, c := range containers {
		if !covered[uid.Container(uid.Agent(c.AgentID), c.ExternalID)] {
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
