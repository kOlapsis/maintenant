// Copyright 2026 Benjamin Touchard (Kolapsis)
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
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// ChangelogResolver fetches release notes from GitHub.
type ChangelogResolver struct {
	registry *RegistryClient
	client   *http.Client
	logger   *slog.Logger
	token    string
}

// NewChangelogResolver creates a changelog resolver.
func NewChangelogResolver(registry *RegistryClient, logger *slog.Logger) *ChangelogResolver {
	return &ChangelogResolver{
		registry: registry,
		client:   &http.Client{Timeout: 15 * time.Second},
		logger:   logger,
		token:    os.Getenv("GITHUB_TOKEN"),
	}
}

// ghRelease is the GitHub release API response.
type ghRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// ResolveSourceURL extracts the source repository URL from OCI image labels.
func (cr *ChangelogResolver) ResolveSourceURL(ctx context.Context, imageRef string) (string, error) {
	labels, err := cr.registry.GetConfigLabels(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("get config labels: %w", err)
	}

	// Check standard OCI labels
	for _, key := range []string{
		"org.opencontainers.image.source",
		"org.label-schema.vcs-url",
	} {
		if url, ok := labels[key]; ok && url != "" {
			return url, nil
		}
	}

	return "", nil
}

// FetchLatestReleases fetches the latest releases from a GitHub repository.
func (cr *ChangelogResolver) FetchLatestReleases(ctx context.Context, owner, repo string, count int) ([]ReleaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=%d", owner, repo, count)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Maintenant/1.0")
	if cr.token != "" {
		req.Header.Set("Authorization", "Bearer "+cr.token)
	}

	resp, err := cr.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned status %d", resp.StatusCode)
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode github releases: %w", err)
	}

	result := make([]ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		publishedAt, _ := time.Parse(time.RFC3339, r.PublishedAt)
		result = append(result, ReleaseInfo{
			TagName:            r.TagName,
			Name:               r.Name,
			Body:               truncate(r.Body, 2000),
			PublishedAt:        publishedAt,
			HTMLURL:            r.HTMLURL,
			HasBreakingChanges: DetectBreakingChanges(r.Body),
		})
	}

	return result, nil
}

// ResolveChangelog resolves changelog data for an image update.
func (cr *ChangelogResolver) ResolveChangelog(ctx context.Context, imageRef, latestTag string) (changelogURL, summary string, hasBreaking bool, sourceURL string) {
	// Try to get the source URL from OCI labels
	srcURL, err := cr.ResolveSourceURL(ctx, imageRef)
	if err != nil || srcURL == "" {
		return "", "", false, ""
	}
	sourceURL = srcURL

	// Parse GitHub owner/repo from URL
	owner, repo := parseGitHubURL(srcURL)
	if owner == "" || repo == "" {
		return "", "", false, sourceURL
	}

	// Fetch latest releases
	releases, err := cr.FetchLatestReleases(ctx, owner, repo, 5)
	if err != nil {
		cr.logger.Warn("changelog: failed to fetch releases",
			"owner", owner, "repo", repo, "error", err)
		return "", "", false, sourceURL
	}

	// Find the release matching the latest tag
	for _, rel := range releases {
		if rel.TagName == latestTag || rel.TagName == "v"+latestTag {
			summary := rel.Body
			if len(summary) > 500 {
				summary = summary[:497] + "..."
			}
			return rel.HTMLURL, summary, rel.HasBreakingChanges, sourceURL
		}
	}

	// If no exact match, use the most recent release
	if len(releases) > 0 {
		rel := releases[0]
		summary := rel.Body
		if len(summary) > 500 {
			summary = summary[:497] + "..."
		}
		return rel.HTMLURL, summary, rel.HasBreakingChanges, sourceURL
	}

	return "", "", false, sourceURL
}

// DetectBreakingChanges scans release body text for breaking change indicators.
func DetectBreakingChanges(body string) bool {
	lower := strings.ToLower(body)
	patterns := []string{
		"breaking change",
		"breaking:",
		"migration required",
		"deprecated",
		"removed",
		"incompatible",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// parseGitHubURL extracts owner and repo from a GitHub URL.
func parseGitHubURL(rawURL string) (owner, repo string) {
	rawURL = strings.TrimSuffix(rawURL, ".git")

	if strings.Contains(rawURL, "github.com/") {
		parts := strings.Split(rawURL, "github.com/")
		if len(parts) < 2 {
			return "", ""
		}
		segments := strings.SplitN(parts[1], "/", 3)
		if len(segments) >= 2 {
			return segments[0], segments[1]
		}
	}

	return "", ""
}
