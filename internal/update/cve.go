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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const osvBatchURL = "https://api.osv.dev/v1/querybatch"

// CVEClient queries OSV.dev for known vulnerabilities.
type CVEClient struct {
	store  UpdateStore
	client *http.Client
	logger *slog.Logger
	delay  time.Duration
}

// NewCVEClient creates a CVE lookup client.
func NewCVEClient(store UpdateStore, logger *slog.Logger) *CVEClient {
	return &CVEClient{
		store:  store,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
		delay:  500 * time.Millisecond,
	}
}

// osvQuery is a single query in the OSV batch request.
type osvQuery struct {
	Package osvPackage `json:"package"`
	Version string     `json:"version,omitempty"`
}

type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

// osvBatchRequest is the batch request body.
type osvBatchRequest struct {
	Queries []osvQuery `json:"queries"`
}

// osvBatchResponse is the batch response.
type osvBatchResponse struct {
	Results []osvResult `json:"results"`
}

type osvResult struct {
	Vulns []osvVuln `json:"vulns"`
}

type osvVuln struct {
	ID       string        `json:"id"`
	Summary  string        `json:"summary"`
	Severity []osvSeverity `json:"severity"`
	Affected []osvAffected `json:"affected"`
}

type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type osvAffected struct {
	Package  osvPackage `json:"package"`
	Ranges   []osvRange `json:"ranges"`
	Versions []string   `json:"versions"`
}

type osvRange struct {
	Type   string     `json:"type"`
	Events []osvEvent `json:"events"`
}

type osvEvent struct {
	Introduced string `json:"introduced,omitempty"`
	Fixed      string `json:"fixed,omitempty"`
}

// ImageCVEQuery holds parameters for querying CVEs for an image.
type ImageCVEQuery struct {
	ContainerID string
	PackageName string
	Ecosystem   string
	Version     string
}

// QueryCVEs queries OSV.dev for a batch of images and returns CVEs.
func (c *CVEClient) QueryCVEs(ctx context.Context, queries []ImageCVEQuery) (map[string][]*CVECacheEntry, error) {
	results := make(map[string][]*CVECacheEntry)

	// Check cache first, build list of uncached queries
	var uncached []ImageCVEQuery

	for _, q := range queries {
		fresh, err := c.store.IsCVECacheFresh(ctx, q.Ecosystem, q.PackageName, q.Version)
		if err != nil {
			c.logger.Warn("cve: cache check failed", "package", q.PackageName, "error", err)
		}
		if fresh {
			entries, err := c.store.GetCVECacheEntries(ctx, q.Ecosystem, q.PackageName, q.Version)
			if err == nil {
				results[q.ContainerID] = entries
			}
			continue
		}
		uncached = append(uncached, q)
	}

	if len(uncached) == 0 {
		return results, nil
	}

	// Build OSV batch request
	osvQueries := make([]osvQuery, len(uncached))
	for i, q := range uncached {
		osvQueries[i] = osvQuery{
			Package: osvPackage{Name: q.PackageName, Ecosystem: q.Ecosystem},
			Version: q.Version,
		}
	}

	body, err := json.Marshal(osvBatchRequest{Queries: osvQueries})
	if err != nil {
		return results, fmt.Errorf("marshal osv request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, osvBatchURL, bytes.NewReader(body))
	if err != nil {
		return results, fmt.Errorf("create osv request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return results, fmt.Errorf("osv request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return results, fmt.Errorf("osv returned status %d", resp.StatusCode)
	}

	var batchResp osvBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		return results, fmt.Errorf("decode osv response: %w", err)
	}

	// Process results
	now := time.Now()
	expires := now.Add(24 * time.Hour)

	for i, result := range batchResp.Results {
		if i >= len(uncached) {
			break
		}
		q := uncached[i]
		var entries []*CVECacheEntry

		for _, vuln := range result.Vulns {
			severity, score := parseSeverity(vuln)
			fixedIn := extractFixedIn(vuln)

			entry := CVECacheEntry{
				Ecosystem:      q.Ecosystem,
				PackageName:    q.PackageName,
				PackageVersion: q.Version,
				CVEID:          vuln.ID,
				CVSSScore:      score,
				Severity:       severity,
				Summary:        truncate(vuln.Summary, 512),
				FixedIn:        fixedIn,
				FetchedAt:      now,
				ExpiresAt:      expires,
			}

			if _, err := c.store.InsertCVECacheEntry(ctx, &entry); err != nil {
				c.logger.Warn("cve: failed to cache entry", "cve", vuln.ID, "error", err)
			}

			entries = append(entries, &entry)
		}

		results[q.ContainerID] = entries
	}

	return results, nil
}

// MapImageToCVEQuery maps a container image to an OSV query.
// This does a best-effort mapping: for well-known images (nginx, postgres, etc.)
// it maps to the corresponding ecosystem package.
func MapImageToCVEQuery(image, currentTag string) *ImageCVEQuery {
	repo, _, _ := parseImageRef(image)

	// Strip registry prefix for mapping
	name := repo
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}

	// Map known images to OSV ecosystem packages
	ecosystemMap := map[string]struct{ pkg, eco string }{
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

	if mapping, ok := ecosystemMap[name]; ok {
		return &ImageCVEQuery{
			PackageName: mapping.pkg,
			Ecosystem:   mapping.eco,
			Version:     currentTag,
		}
	}

	return nil
}

func parseSeverity(vuln osvVuln) (CVESeverity, float64) {
	for _, s := range vuln.Severity {
		if s.Type == "CVSS_V3" {
			score := parseCVSSScore(s.Score)
			if score >= 9.0 {
				return CVESeverityCritical, score
			}
			if score >= 7.0 {
				return CVESeverityHigh, score
			}
			if score >= 4.0 {
				return CVESeverityMedium, score
			}
			return CVESeverityLow, score
		}
	}
	// Default based on OSV ID prefix
	if strings.HasPrefix(vuln.ID, "CVE-") {
		return CVESeverityMedium, 5.0
	}
	return CVESeverityLow, 0
}

// parseCVSSScore extracts the base score from a CVSS v3 vector string.
func parseCVSSScore(vector string) float64 {
	// CVSS vectors look like: CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H
	// We use a simplified scoring based on the attack vector components.
	if vector == "" {
		return 0
	}

	score := 5.0 // base

	if strings.Contains(vector, "AV:N") {
		score += 1.5
	}
	if strings.Contains(vector, "AC:L") {
		score += 0.5
	}
	if strings.Contains(vector, "PR:N") {
		score += 0.5
	}
	if strings.Contains(vector, "C:H") {
		score += 1.0
	}
	if strings.Contains(vector, "I:H") {
		score += 0.5
	}
	if strings.Contains(vector, "A:H") {
		score += 0.5
	}

	if score > 10.0 {
		score = 10.0
	}
	return score
}

func extractFixedIn(vuln osvVuln) string {
	for _, a := range vuln.Affected {
		for _, r := range a.Ranges {
			for _, e := range r.Events {
				if e.Fixed != "" {
					return e.Fixed
				}
			}
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
