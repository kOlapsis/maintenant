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
	"path/filepath"
	"strings"
	"time"
)

// ContainerInfo holds the minimal container data needed for scanning.
type ContainerInfo struct {
	ExternalID         string
	Name               string
	Image              string
	Labels             map[string]string
	OrchestrationGroup string
	OrchestrationUnit  string
	RuntimeType        string
	ControllerKind     string
	ComposeWorkingDir  string
}

// Scanner checks containers for available updates by comparing tags and digests.
// registryQuerier abstracts the registry operations needed by Scanner.
// Satisfied by *RegistryClient in production; replaced by stubs in tests.
type registryQuerier interface {
	ListTags(ctx context.Context, imageRef string) ([]string, error)
	GetDigest(ctx context.Context, imageRef string) (string, error)
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
	imageRef, currentTag, registry := parseImageRef(c.Image)
	if imageRef == "" {
		return nil, fmt.Errorf("cannot parse image reference: %s", c.Image)
	}

	// Skip local/private images that have no registry and no slash (locally built)
	if !strings.Contains(imageRef, "/") && currentTag == "latest" && registry == "registry-1.docker.io" {
		// Likely a locally-built image (e.g. "myapp" or "myapp:latest") — skip silently
		sc.logger.Debug("scanner: skipping likely local image", "image", c.Image)
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

	// Override registry if specified in labels
	if cfg.Registry != "" {
		registry = cfg.Registry
	}

	// Build full ref for registry queries
	fullRef := imageRef
	if registry != "" && !strings.Contains(imageRef, "/") {
		fullRef = "library/" + imageRef
	}

	// List all tags from registry
	tags, err := sc.registry.ListTags(ctx, fullRef)
	if err != nil {
		// Skip images that fail auth (private/local images not on any registry)
		if strings.Contains(err.Error(), "UNAUTHORIZED") || strings.Contains(err.Error(), "NAME_UNKNOWN") || strings.Contains(err.Error(), "denied") {
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
		tags = tf.Filter(tags)
		if len(tags) == 0 {
			sc.logger.Warn("scanner: tag filter produced no candidates, skipping update check",
				"container", c.Name, "image", c.Image)
			return nil, nil
		}
	}

	// Find best update
	bestTag, updateType := FindBestUpdate(currentTag, tags)
	if bestTag == "" {
		return nil, nil
	}

	// Digest-only mode: non-semver tags like "lts", "alpine", "stable", "latest".
	// Compare the current remote digest against the stored baseline to detect rebuilds.
	if bestTag == currentTag && updateType == UpdateTypeDigestOnly {
		tagRef := fullRef + ":" + currentTag
		remoteDigest, err := sc.registry.GetDigest(ctx, tagRef)
		if err != nil || remoteDigest == "" {
			sc.logger.Debug("scanner: cannot fetch digest for channel tag",
				"container", c.Name, "tag", currentTag, "error", err)
			return nil, nil
		}

		baseline, _ := sc.store.GetDigestBaseline(ctx, c.ExternalID)

		// Store/update the baseline for next scan comparison
		now := time.Now()
		if err := sc.store.UpsertDigestBaseline(ctx, &DigestBaseline{
			ContainerID:  c.ExternalID,
			Image:        c.Image,
			Tag:          currentTag,
			RemoteDigest: remoteDigest,
			CheckedAt:    now,
		}); err != nil {
			sc.logger.Warn("scanner: failed to store digest baseline",
				"container", c.Name, "error", err)
		}

		if baseline == nil || baseline.RemoteDigest == remoteDigest {
			// First scan or digest unchanged — no update
			return nil, nil
		}

		// Digest changed — the tag was republished with a new build
		sc.logger.Info("scanner: digest change detected for channel tag",
			"container", c.Name, "tag", currentTag,
			"old_digest", baseline.RemoteDigest[:19], "new_digest", remoteDigest[:19])

		return &UpdateResult{
			ContainerID:    c.ExternalID,
			ContainerName:  c.Name,
			Image:          c.Image,
			CurrentTag:     currentTag,
			CurrentDigest:  baseline.RemoteDigest,
			PreviousDigest: baseline.RemoteDigest,
			Registry:       registry,
			LatestTag:      currentTag,
			LatestDigest:   remoteDigest,
			UpdateType:     UpdateTypeDigestOnly,
			HasUpdate:      true,
		}, nil
	}

	// Semver update: a newer version tag exists
	latestRef := fullRef + ":" + bestTag
	latestDigest, err := sc.registry.GetDigest(ctx, latestRef)
	if err != nil {
		sc.logger.Warn("scanner: failed to get digest for latest tag",
			"image", fullRef, "tag", bestTag, "error", err)
	}

	result := &UpdateResult{
		ContainerID:   c.ExternalID,
		ContainerName: c.Name,
		Image:         c.Image,
		CurrentTag:    currentTag,
		Registry:      registry,
		LatestTag:     bestTag,
		LatestDigest:  latestDigest,
		UpdateType:    updateType,
		HasUpdate:     true,
	}

	return result, nil
}

func (sc *Scanner) isExcluded(image, tag string, exclusions []*UpdateExclusion) bool {
	for _, e := range exclusions {
		switch e.PatternType {
		case ExclusionTypeImage:
			if matched, _ := filepath.Match(e.Pattern, image); matched {
				return true
			}
		case ExclusionTypeTag:
			if matched, _ := filepath.Match(e.Pattern, tag); matched {
				return true
			}
		}
	}
	return false
}

// parseImageRef splits an image string into (repository, tag, registry).
// Examples:
//   - "nginx:1.25" -> ("nginx", "1.25", "registry-1.docker.io")
//   - "ghcr.io/org/repo:v1.0" -> ("ghcr.io/org/repo", "v1.0", "ghcr.io")
//   - "myapp:latest" -> ("myapp", "latest", "registry-1.docker.io")
func parseImageRef(image string) (repo, tag, registry string) {
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
