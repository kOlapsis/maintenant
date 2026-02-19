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
}

// Scanner checks containers for available updates by comparing tags and digests.
type Scanner struct {
	registry *RegistryClient
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
			results = append(results, *result)
		}
	}

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
		return nil, nil
	}

	// Check if pinned via label
	if cfg.Pin != "" {
		return nil, nil
	}

	// Check version pins in store
	pin, _ := sc.store.GetVersionPin(ctx, c.ExternalID)
	if pin != nil {
		return nil, nil
	}

	// Check exclusions
	if sc.isExcluded(c.Image, currentTag, exclusions) {
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

	// Find best update
	bestTag, updateType := FindBestUpdate(currentTag, tags)
	if bestTag == "" || bestTag == currentTag {
		// No update found — but check for digest-only updates
		if currentTag != "" {
			tagRef := fullRef + ":" + currentTag
			remoteDigest, err := sc.registry.GetDigest(ctx, tagRef)
			if err == nil && remoteDigest != "" {
				// We don't have the local digest here yet; the service will compare
			}
		}
		return nil, nil
	}

	// Get digest for the latest tag
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
