// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ContainerInfo holds the minimal container data needed for scanning.
type ContainerInfo struct {
	UID                string // canonical store PK (uid.Container) — used as alert EntityID
	AgentID            string
	ExternalID         string
	Name               string
	Image              string
	Labels             map[string]string
	OrchestrationGroup string
	OrchestrationUnit  string
	RuntimeType        string
	ControllerKind     string
	ComposeWorkingDir  string
	RepoDigests        []string // "repo@sha256:..." of the running image, as its runtime reports them
	LocallyBuilt       bool     // the runtime reports an image never pulled from nor pushed to a registry
	PodContainer       string   // Kubernetes: the pod container that runs Image
	SwarmService       string   // Swarm: the service the task belongs to
}

// Scanner checks containers for available updates by comparing tags and digests.
// registryQuerier abstracts the registry operations needed by Scanner.
// Satisfied by *RegistryClient in production; replaced by stubs in tests.
type registryQuerier interface {
	ListTags(ctx context.Context, imageRef string) ([]string, error)
	ResolveDigests(ctx context.Context, imageRef string) (RemoteDigests, error)
}

type Scanner struct {
	registry registryQuerier
	store    UpdateStore
	logger   *slog.Logger
	delay    time.Duration
}

// NewScanner creates a new registry scanner.
func NewScanner(registry *RegistryClient, store UpdateStore, logger *slog.Logger) *Scanner {
	return &Scanner{
		registry: registry,
		store:    store,
		logger:   logger,
		delay:    1 * time.Second,
	}
}

// Scan checks all provided containers for available updates.
func (sc *Scanner) Scan(ctx context.Context, containers []ContainerInfo) ([]UpdateResult, []ScanError) {
	var results []UpdateResult
	var scanErrors []ScanError

	// Load exclusions
	exclusions, err := sc.store.ListExclusions(ctx)
	if err != nil {
		sc.logger.Error("scanner: load exclusions", "error", err)
	}

	sc.logger.Info("scanner: starting", "containers", len(containers))

	for i, c := range containers {
		if ctx.Err() != nil {
			break
		}

		// Throttle between images
		if i > 0 {
			select {
			case <-ctx.Done():
				return results, scanErrors
			case <-time.After(sc.delay):
			}
		}

		sc.logger.Debug("scanner: checking container",
			"container", c.Name, "image", c.Image,
			"index", fmt.Sprintf("%d/%d", i+1, len(containers)))

		result, err := sc.scanContainer(ctx, c, exclusions)
		if err != nil {
			scanErrors = append(scanErrors, ScanError{
				ContainerID:   c.ExternalID,
				ContainerName: c.Name,
				Image:         c.Image,
				Error:         err,
			})
			sc.logger.Warn("scanner: failed to scan container",
				"container", c.Name, "image", c.Image, "error", err)
			continue
		}
		if result != nil {
			sc.logger.Info("scanner: update available",
				"container", c.Name,
				"current", result.CurrentTag, "latest", result.LatestTag,
				"type", result.UpdateType)
			results = append(results, *result)
		} else {
			sc.logger.Debug("scanner: up to date", "container", c.Name)
		}
	}

	sc.logger.Info("scanner: finished",
		"scanned", len(containers), "updates", len(results), "errors", len(scanErrors))

	return results, scanErrors
}

func (sc *Scanner) scanContainer(ctx context.Context, c ContainerInfo, exclusions []*UpdateExclusion) (*UpdateResult, error) {
	// Parse image reference
	imageRef, currentTag, registry := ParseImageRef(c.Image)
	if imageRef == "" {
		return nil, fmt.Errorf("cannot parse image reference: %s", c.Image)
	}

	if c.LocallyBuilt {
		sc.logger.Debug("scanner: skipping locally built image", "image", c.Image)
		return nil, nil
	}

	// Parse labels
	cfg := ParseUpdateLabels(c.Labels, sc.logger)
	if !cfg.Enabled {
		sc.logger.Debug("scanner: update tracking disabled", "container", c.Name)
		return nil, nil
	}

	// Check if pinned via label
	if cfg.Pin != "" {
		sc.logger.Debug("scanner: pinned via label", "container", c.Name, "pin", cfg.Pin)
		return nil, nil
	}

	// Check version pins in store
	pin, _ := sc.store.GetVersionPin(ctx, c.ExternalID)
	if pin != nil {
		sc.logger.Debug("scanner: pinned via store", "container", c.Name, "pin", pin.PinnedTag)
		return nil, nil
	}

	// Check exclusions
	if sc.isExcluded(c.Image, currentTag, exclusions) {
		sc.logger.Debug("scanner: excluded by rule", "container", c.Name, "image", c.Image)
		return nil, nil
	}

	// Build full ref for registry queries
	fullRef := imageRef
	if registry != "" && !strings.Contains(imageRef, "/") {
		fullRef = "library/" + imageRef
	}

	// The registry label points the queries at a mirror serving the same repository.
	if cfg.Registry != "" {
		registry = cfg.Registry
		fullRef = registry + "/" + repositoryPath(fullRef)
	}

	target := scanTarget{
		container:     c,
		fullRef:       fullRef,
		currentTag:    currentTag,
		registry:      registry,
		runningDigest: runningDigest(c, imageRef),
		alertOn:       cfg.AlertOn,
	}

	level := cfg.TrackLevel()
	if level == TrackDigest {
		return sc.checkDigest(ctx, target)
	}

	// List all tags from registry
	tags, err := sc.registry.ListTags(ctx, fullRef)
	if err != nil {
		if isUnreachableImage(err) {
			sc.logger.Debug("scanner: skipping unreachable image", "image", c.Image, "reason", err.Error())
			return nil, nil
		}
		return nil, fmt.Errorf("list tags: %w", err)
	}

	// Apply tag filter (include/exclude labels + variant detection).
	// Tag filters are skipped for non-semver channel tags (digest-only mode) because
	// the tag filter operates on version tags and would remove channel tags like
	// "latest", "lts", "stable" from the list, preventing digest comparison.
	versionPart, variant := splitVariant(currentTag)
	if _, parseErr := ParseTag(versionPart); parseErr == nil {
		// Semver tag: apply user-configured tag filters
		tf := NewTagFilter(cfg.TagInclude, cfg.TagExclude, variant)
		filtered := tf.Filter(tags)
		if len(filtered) == 0 {
			sc.logger.Warn("scanner: tag filter produced no candidates",
				"container", c.Name, "image", c.Image)
		}
		// The current tag stays the reference for digest comparison even when the filter rejects it.
		if slices.Contains(tags, currentTag) && !slices.Contains(filtered, currentTag) {
			filtered = append(filtered, currentTag)
		}
		tags = filtered
		if len(tags) == 0 {
			return nil, nil
		}
	}

	// Find best update
	bestTag, updateType := findBestUpdate(currentTag, tags, level)
	if bestTag == "" {
		return nil, nil
	}

	// Floating tags, i.e. non-semver channels like "lts", "alpine", "stable", "latest",
	// plus partial versions like "v3" or "1.2" that the registry moves.
	if bestTag == currentTag && updateType == UpdateTypeDigestOnly {
		return sc.checkDigest(ctx, target)
	}

	// Semver update: a newer version tag exists
	latest, err := sc.registry.ResolveDigests(ctx, fullRef+":"+bestTag)
	if err != nil {
		sc.logger.Warn("scanner: failed to get digest for latest tag",
			"image", fullRef, "tag", bestTag, "error", err)
	}

	result := &UpdateResult{
		ContainerID:    c.ExternalID,
		ContainerUID:   c.UID,
		ContainerName:  c.Name,
		Image:          c.Image,
		CurrentTag:     currentTag,
		CurrentDigest:  target.runningDigest,
		Registry:       registry,
		LatestTag:      bestTag,
		LatestDigest:   latest.Digest,
		UpdateType:     updateType,
		HasUpdate:      true,
		PreviousDigest: target.runningDigest,
		AlertOn:        cfg.AlertOn,
	}

	return result, nil
}

// scanTarget carries what the digest comparison needs about one container.
type scanTarget struct {
	container     ContainerInfo
	fullRef       string
	currentTag    string
	registry      string
	runningDigest string
	alertOn       string
}

// checkDigest reports an update for as long as the container does not run what its tag
// points at now, which detects a republished tag without looking at other tags.
func (sc *Scanner) checkDigest(ctx context.Context, t scanTarget) (*UpdateResult, error) {
	c := t.container
	remote, err := sc.registry.ResolveDigests(ctx, t.fullRef+":"+t.currentTag)
	if err != nil {
		if isUnreachableImage(err) {
			sc.logger.Debug("scanner: skipping unreachable tag",
				"container", c.Name, "tag", t.currentTag, "reason", err.Error())
			return nil, nil
		}
		return nil, fmt.Errorf("get digest: %w", err)
	}
	if remote.Digest == "" {
		return nil, nil
	}

	current, err := sc.currentDigest(ctx, t, remote)
	if err != nil {
		return nil, err
	}
	if current == "" || remote.Contains(current) {
		return nil, nil
	}

	sc.logger.Info("scanner: tag now points at another digest than the running one",
		"container", c.Name, "tag", t.currentTag,
		"running_digest", shortDigest(current), "tag_digest", shortDigest(remote.Digest))

	return &UpdateResult{
		ContainerID:    c.ExternalID,
		ContainerUID:   c.UID,
		ContainerName:  c.Name,
		Image:          c.Image,
		CurrentTag:     t.currentTag,
		CurrentDigest:  current,
		PreviousDigest: current,
		Registry:       t.registry,
		LatestTag:      t.currentTag,
		LatestDigest:   remote.Digest,
		UpdateType:     UpdateTypeDigestOnly,
		HasUpdate:      true,
		AlertOn:        t.alertOn,
	}, nil
}

// currentDigest returns the digest the container runs and records it as the container's baseline,
// under the tag's multi-platform digest when the two name the same release. When the runtime does
// not report it, the baseline stands in: the last digest the container was seen running, else the
// one its tag pointed at on first sight, recorded then with nothing to compare.
func (sc *Scanner) currentDigest(ctx context.Context, t scanTarget, remote RemoteDigests) (string, error) {
	c := t.container
	current := t.runningDigest
	if current == "" {
		baseline, err := sc.store.GetDigestBaseline(ctx, c.ExternalID)
		if err != nil {
			return "", fmt.Errorf("get digest baseline: %w", err)
		}
		if baseline != nil && baseline.Tag == t.currentTag {
			current = baseline.RemoteDigest
			if current == remote.Digest || !remote.Contains(current) {
				return current, nil
			}
		}
	}

	recorded := current
	if recorded == "" || remote.Contains(recorded) {
		recorded = remote.Digest
	}
	if err := sc.store.UpsertDigestBaseline(ctx, &DigestBaseline{
		ContainerID:  c.ExternalID,
		Image:        c.Image,
		Tag:          t.currentTag,
		RemoteDigest: recorded,
		CheckedAt:    time.Now(),
	}); err != nil {
		sc.logger.Warn("scanner: failed to store digest baseline",
			"container", c.Name, "error", err)
	}
	return current, nil
}

// isUnreachableImage reports a registry answer meaning the image or tag cannot be
// checked there at all (private, unknown or never published), as opposed to a failure.
func isUnreachableImage(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "UNAUTHORIZED") || strings.Contains(msg, "NAME_UNKNOWN") ||
		strings.Contains(msg, "MANIFEST_UNKNOWN") || strings.Contains(msg, "denied")
}

func shortDigest(d string) string {
	if len(d) > 19 {
		return d[:19]
	}
	return d
}

// runningImage describes the image a container runs, as a result without an update.
func runningImage(c ContainerInfo) UpdateResult {
	repo, tag, registry := ParseImageRef(c.Image)
	return UpdateResult{
		ContainerID:   c.ExternalID,
		ContainerUID:  c.UID,
		ContainerName: c.Name,
		Image:         c.Image,
		CurrentTag:    tag,
		CurrentDigest: runningDigest(c, repo),
		Registry:      registry,
	}
}

// runningDigest names the image a container runs by the digest its runtime pulled for repo, else by the digest pinned in its reference.
func runningDigest(c ContainerInfo, repo string) string {
	if d := repoDigestFor(c.RepoDigests, repo); d != "" {
		return d
	}
	if _, d, ok := strings.Cut(c.Image, "@"); ok {
		return d
	}
	return ""
}

// repoDigestFor returns the digest recorded for repo among an image's repo digests.
func repoDigestFor(repoDigests []string, repo string) string {
	want := familiarRepo(repo)
	for _, rd := range repoDigests {
		name, digest, ok := strings.Cut(rd, "@")
		if ok && familiarRepo(name) == want {
			return digest
		}
	}
	return ""
}

// familiarRepo reduces a Docker Hub repository to the short form Docker prints ("library/nginx" -> "nginx").
func familiarRepo(repo string) string {
	repo = strings.TrimPrefix(repo, "docker.io/")
	repo = strings.TrimPrefix(repo, "index.docker.io/")
	if rest, ok := strings.CutPrefix(repo, "library/"); ok && !strings.Contains(rest, "/") {
		return rest
	}
	return repo
}

func (sc *Scanner) isExcluded(image, tag string, exclusions []*UpdateExclusion) bool {
	repo, _, _ := ParseImageRef(image)
	for _, e := range exclusions {
		switch e.PatternType {
		case ExclusionTypeImage:
			// The full reference is still tried so that patterns written with a tag keep matching.
			for _, candidate := range []string{repo, familiarRepo(repo), image} {
				if matched, _ := filepath.Match(e.Pattern, candidate); matched {
					return true
				}
			}
		case ExclusionTypeTag:
			if matched, _ := filepath.Match(e.Pattern, tag); matched {
				return true
			}
		}
	}
	return false
}

// ParseImageRef splits an image string into (repository, tag, registry).
// Examples:
//   - "nginx:1.25" -> ("nginx", "1.25", "registry-1.docker.io")
//   - "ghcr.io/org/repo:v1.0" -> ("ghcr.io/org/repo", "v1.0", "ghcr.io")
//   - "myapp:latest" -> ("myapp", "latest", "registry-1.docker.io")
func ParseImageRef(image string) (repo, tag, registry string) {
	// Strip digest (@sha256:...) — we only need the repository and tag
	if idx := strings.Index(image, "@sha256:"); idx > 0 {
		image = image[:idx]
	}

	// Strip "docker.io/" prefix
	image = strings.TrimPrefix(image, "docker.io/")

	// Split tag
	tag = "latest"
	if idx := strings.LastIndex(image, ":"); idx > 0 {
		// Make sure this isn't a port number by checking if there's a slash after it
		possibleTag := image[idx+1:]
		if !strings.Contains(possibleTag, "/") {
			tag = possibleTag
			image = image[:idx]
		}
	}

	// Determine registry
	registry = "registry-1.docker.io"
	parts := strings.SplitN(image, "/", 2)
	if len(parts) >= 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		registry = parts[0]
	}

	return image, tag, registry
}

// repositoryPath strips the registry host from a repository ("ghcr.io/org/repo" -> "org/repo").
func repositoryPath(repo string) string {
	if host, path, ok := strings.Cut(repo, "/"); ok && strings.ContainsAny(host, ".:") {
		return path
	}
	return repo
}
