// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package update

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ecosystemStaticMap maps well-known image short names to their CVE ecosystem.
// To add a new image: append an entry with {packageName, ecosystem}.
// The package name is used in the OSV query; ecosystem must match OSV format
// (e.g., "Debian:12", "Alpine:3.19", "Go").
var ecosystemStaticMap = map[string]struct{ pkg, eco string }{
	"nginx":      {"nginx", "Debian:12"},
	"postgres":   {"postgresql-16", "Debian:12"},
	"redis":      {"redis", "Debian:12"},
	"mysql":      {"mysql-server-8.0", "Debian:12"},
	"mariadb":    {"mariadb", "Debian:12"},
	"node":       {"nodejs", "Debian:12"},
	"python":     {"python3", "Debian:12"},
	"golang":     {"golang", "Debian:12"},
	"httpd":      {"apache2", "Debian:12"},
	"memcached":  {"memcached", "Debian:12"},
	"mongo":      {"mongodb", "Debian:12"},
	"rabbitmq":   {"rabbitmq-server", "Debian:12"},
	"traefik":    {"traefik", "Go"},
	"grafana":    {"grafana", "Go"},
	"prometheus": {"prometheus", "Go"},
}

// languageEcosystemImages lists images that use language-level ecosystems
// instead of OS-level ecosystems. These bypass OS detection entirely.
// To add a new entry: append the image short name with its language ecosystem.
var languageEcosystemImages = map[string]string{
	"traefik":    "Go",
	"grafana":    "Go",
	"prometheus": "Go",
	"consul":     "Go",
	"vault":      "Go",
	"caddy":      "Go",
	"minio":      "Go",
	"etcd":       "Go",
}

// knownScratchImages are images that should return nil (no ecosystem).
var knownScratchImages = map[string]bool{
	"scratch":    true,
	"gcr.io/distroless/static":        true,
	"gcr.io/distroless/base":          true,
	"gcr.io/distroless/cc":            true,
	"gcr.io/distroless/static-debian": true,
}

// ociBaseNameLabel is the OCI annotation for base image provenance.
const ociBaseNameLabel = "org.opencontainers.image.base.name"

// knownPublicRegistries are registries where unauthenticated access is allowed.
var knownPublicRegistries = map[string]bool{
	"docker.io":      true,
	"registry-1.docker.io": true,
	"ghcr.io":        true,
	"quay.io":        true,
	"gcr.io":         true,
	"public.ecr.aws": true,
}

// baseImageEcosystems maps base image names to their OSV ecosystem identifier.
var baseImageEcosystems = map[string]string{
	"debian":  "Debian",
	"ubuntu":  "Ubuntu",
	"alpine":  "Alpine",
	"centos":  "CentOS",
	"fedora":  "Fedora",
}

// baseImageVersions maps codenames/tags to version numbers for OSV.
var baseImageVersions = map[string]string{
	// Debian
	"bookworm":      "12",
	"bookworm-slim": "12",
	"bullseye":      "11",
	"bullseye-slim": "11",
	"buster":        "10",
	"buster-slim":   "10",
	// Ubuntu
	"noble":  "24.04",
	"jammy":  "22.04",
	"focal":  "20.04",
	"bionic": "18.04",
}

// EcosystemResolver resolves container images to CVE ecosystems
// using a fallback chain: cache → static → local OCI labels →
// remote registry labels → tag heuristics → image name fallback.
type EcosystemResolver struct {
	registry *RegistryClient
	logger   *slog.Logger

	mu    sync.RWMutex
	cache map[string]*EcosystemResult
}

// NewEcosystemResolver creates an ecosystem resolver.
func NewEcosystemResolver(registry *RegistryClient, logger *slog.Logger) *EcosystemResolver {
	return &EcosystemResolver{
		registry: registry,
		logger:   logger,
		cache:    make(map[string]*EcosystemResult),
	}
}

// Resolve determines the CVE ecosystem for a container image using a fallback chain.
// Returns nil if no ecosystem can be determined.
func (r *EcosystemResolver) Resolve(ctx context.Context, image, tag, digest string, localLabels map[string]string) *EcosystemResult {
	if ctx.Err() != nil {
		return nil
	}

	// Extract short name for lookups
	shortName := imageShortName(image)

	// 1. Cache lookup
	cacheKey := fmt.Sprintf("%s:%s@%s", image, tag, digest)
	r.mu.RLock()
	if cached, ok := r.cache[cacheKey]; ok {
		r.mu.RUnlock()
		r.logger.Debug("ecosystem resolved",
			"image", shortName, "ecosystem", cached.Ecosystem,
			"method", cached.DetectionMethod, "cache", "hit")
		return cached
	}
	r.mu.RUnlock()

	// 2. Static mapping (preserves backward compatibility)
	if result := r.resolveStatic(shortName); result != nil {
		r.cacheResult(cacheKey, result)
		r.logger.Debug("ecosystem resolved",
			"image", shortName, "ecosystem", result.Ecosystem,
			"method", result.DetectionMethod, "cache", "miss")
		return result
	}

	// 3. Local runtime OCI labels
	if localLabels != nil {
		if result, ok := parseBaseImageLabel(localLabels, shortName); ok {
			r.cacheResult(cacheKey, result)
			r.logger.Debug("ecosystem resolved",
				"image", shortName, "ecosystem", result.Ecosystem,
				"method", result.DetectionMethod, "cache", "miss")
			return result
		}
	}

	// 4. Remote registry labels (public registries only)
	if r.registry != nil && isPublicRegistry(image) {
		if result := r.resolveRemoteLabels(ctx, image, tag, shortName); result != nil {
			r.cacheResult(cacheKey, result)
			r.logger.Debug("ecosystem resolved",
				"image", shortName, "ecosystem", result.Ecosystem,
				"method", result.DetectionMethod, "cache", "miss")
			return result
		}
	}

	// 5. Tag heuristics
	if eco, ok := ParseTagOSVariant(tag); ok {
		result := &EcosystemResult{
			PackageName:     shortName,
			Ecosystem:       eco,
			DetectionMethod: "tag-heuristic",
		}
		r.cacheResult(cacheKey, result)
		r.logger.Debug("ecosystem resolved",
			"image", shortName, "ecosystem", result.Ecosystem,
			"method", result.DetectionMethod, "cache", "miss")
		return result
	}

	// 6. Image name fallback
	if result := r.resolveImageNameFallback(shortName); result != nil {
		r.cacheResult(cacheKey, result)
		r.logger.Debug("ecosystem resolved",
			"image", shortName, "ecosystem", result.Ecosystem,
			"method", result.DetectionMethod, "cache", "miss")
		return result
	}

	r.logger.Debug("ecosystem unresolved", "image", shortName)
	return nil
}

// resolveStatic checks the static mapping for known images.
func (r *EcosystemResolver) resolveStatic(shortName string) *EcosystemResult {
	if mapping, ok := ecosystemStaticMap[shortName]; ok {
		return &EcosystemResult{
			PackageName:     mapping.pkg,
			Ecosystem:       mapping.eco,
			DetectionMethod: "static",
		}
	}
	return nil
}

// resolveRemoteLabels fetches OCI labels from a public registry.
func (r *EcosystemResolver) resolveRemoteLabels(ctx context.Context, image, tag, shortName string) *EcosystemResult {
	imageRef := image + ":" + tag
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	labels, err := r.registry.GetConfigLabels(timeoutCtx, imageRef)
	if err != nil {
		r.logger.Warn("ecosystem: remote label fetch failed",
			"image", shortName, "error", err)
		return nil
	}
	if labels == nil {
		return nil
	}

	result, ok := parseBaseImageLabel(labels, shortName)
	if !ok {
		return nil
	}
	return result
}

// resolveImageNameFallback uses the image short name as package name with
// Debian:12 as default ecosystem. Returns nil for scratch/distroless/unrecognizable images.
func (r *EcosystemResolver) resolveImageNameFallback(shortName string) *EcosystemResult {
	// Skip scratch-based and distroless images
	if knownScratchImages[shortName] {
		return nil
	}

	// Skip unrecognizable names (hex hashes, single chars)
	if len(shortName) <= 2 || looksLikeHash(shortName) {
		return nil
	}

	// Check if it's a language ecosystem image
	if eco, ok := languageEcosystemImages[shortName]; ok {
		return &EcosystemResult{
			PackageName:     shortName,
			Ecosystem:       eco,
			DetectionMethod: "image-name-fallback",
		}
	}

	return &EcosystemResult{
		PackageName:     shortName,
		Ecosystem:       "Debian:12",
		DetectionMethod: "image-name-fallback",
	}
}

// cacheResult stores a resolved ecosystem in the cache.
func (r *EcosystemResolver) cacheResult(key string, result *EcosystemResult) {
	r.mu.Lock()
	r.cache[key] = result
	r.mu.Unlock()
}

// parseBaseImageLabel extracts ecosystem info from OCI base image labels.
func parseBaseImageLabel(labels map[string]string, shortName string) (*EcosystemResult, bool) {
	baseName, ok := labels[ociBaseNameLabel]
	if !ok || baseName == "" {
		return nil, false
	}

	// Parse base image reference: "docker.io/library/debian:bookworm-slim"
	// Extract the image name and tag parts
	baseImage := baseName
	// Strip registry prefix
	if idx := strings.Index(baseImage, "/library/"); idx >= 0 {
		baseImage = baseImage[idx+len("/library/"):]
	} else if idx := strings.LastIndex(baseImage, "/"); idx >= 0 {
		baseImage = baseImage[idx+1:]
	}

	// Split name:tag
	baseImageName := baseImage
	baseTag := ""
	if idx := strings.LastIndex(baseImage, ":"); idx >= 0 {
		baseImageName = baseImage[:idx]
		baseTag = baseImage[idx+1:]
	}

	// Look up the base image ecosystem
	ecoPrefix, ok := baseImageEcosystems[baseImageName]
	if !ok {
		return nil, false
	}

	// Determine version
	version := ""
	if baseTag != "" {
		// Check codename mapping first
		// Strip -slim and similar suffixes for codename lookup
		cleanTag := baseTag
		for _, suffix := range []string{"-slim", "-backports"} {
			cleanTag = strings.TrimSuffix(cleanTag, suffix)
		}
		if v, ok := baseImageVersions[cleanTag]; ok {
			version = v
		} else if baseImageName == "alpine" {
			// Alpine uses major.minor from tag (e.g., "3.19", "3.20.1" → "3.20")
			parts := strings.SplitN(baseTag, ".", 3)
			if len(parts) >= 2 {
				version = parts[0] + "." + parts[1]
			}
		} else {
			// Try using the tag directly as version if it looks numeric
			if len(baseTag) > 0 && baseTag[0] >= '0' && baseTag[0] <= '9' {
				// Use major version only
				parts := strings.SplitN(baseTag, ".", 2)
				version = parts[0]
			}
		}
	}

	if version == "" {
		return nil, false
	}

	ecosystem := ecoPrefix + ":" + version

	return &EcosystemResult{
		PackageName:     shortName,
		Ecosystem:       ecosystem,
		DetectionMethod: "oci-labels",
	}, true
}

// imageShortName extracts the short name from a full image reference.
// "docker.io/library/nginx" → "nginx", "ghcr.io/org/app" → "app"
func imageShortName(image string) string {
	repo, _, _ := parseImageRef(image)
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		return repo[idx+1:]
	}
	return repo
}

// isPublicRegistry checks if the image is from a known public registry.
func isPublicRegistry(image string) bool {
	// Images without a registry prefix are from Docker Hub (public)
	if !strings.Contains(image, "/") || strings.HasPrefix(image, "library/") {
		return true
	}

	// Check first segment for known registries
	parts := strings.SplitN(image, "/", 2)
	if len(parts) < 2 {
		return true
	}

	host := parts[0]
	// If first segment has no dot, it's a Docker Hub user/org (public)
	if !strings.Contains(host, ".") {
		return true
	}

	return knownPublicRegistries[host]
}

// looksLikeHash returns true if the name looks like a hex hash.
func looksLikeHash(s string) bool {
	if len(s) < 8 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
