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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	osvBaseURL    = "https://api.osv.dev"
	maxOSVPages   = 10
	cveCacheTTL   = 24 * time.Hour
	cveSummaryMax = 512
)

// CVEClient queries OSV.dev for known vulnerabilities.
type CVEClient struct {
	store   UpdateStore
	client  *http.Client
	logger  *slog.Logger
	delay   time.Duration
	baseURL string
}

// NewCVEClient creates a CVE lookup client.
func NewCVEClient(store UpdateStore, logger *slog.Logger) *CVEClient {
	return &CVEClient{
		store:   store,
		client:  &http.Client{Timeout: 30 * time.Second},
		logger:  logger,
		delay:   500 * time.Millisecond,
		baseURL: osvBaseURL,
	}
}

// osvQuery is a single query in the OSV batch request.
type osvQuery struct {
	Package   osvPackage `json:"package"`
	Version   string     `json:"version,omitempty"`
	PageToken string     `json:"page_token,omitempty"`
}

type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

// osvBatchRequest is the batch request body.
type osvBatchRequest struct {
	Queries []osvQuery `json:"queries"`
}

// osvBatchResponse is the batch response: ids only, no vulnerability detail.
type osvBatchResponse struct {
	Results []osvBatchResult `json:"results"`
}

type osvBatchResult struct {
	Vulns         []osvBatchVuln `json:"vulns"`
	NextPageToken string         `json:"next_page_token"`
}

type osvBatchVuln struct {
	ID       string `json:"id"`
	Modified string `json:"modified"`
}

// osvVulnRecord is the full vulnerability record from GET /v1/vulns/{id}.
type osvVulnRecord struct {
	ID         string         `json:"id"`
	Summary    string         `json:"summary"`
	Details    string         `json:"details"`
	Severity   []osvSeverity  `json:"severity"`
	Affected   []osvAffected  `json:"affected"`
	References []osvReference `json:"references"`
	Aliases    []string       `json:"aliases"`
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

type osvReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// ImageCVEQuery holds parameters for querying CVEs for an image.
type ImageCVEQuery struct {
	ContainerID string
	PackageName string
	Ecosystem   string
	Version     string
}

// QueryCVEs queries OSV.dev for a batch of images and returns CVEs, hydrating
// each vulnerability id returned by the batch endpoint with its full record.
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

	osvQueries := make([]osvQuery, len(uncached))
	for i, q := range uncached {
		osvQueries[i] = osvQuery{
			Package: osvPackage{Name: q.PackageName, Ecosystem: q.Ecosystem},
			Version: q.Version,
		}
	}

	batchResp, err := c.postBatch(ctx, osvQueries)
	if err != nil {
		return results, err
	}

	vulnIDs := make([][]string, len(uncached))
	for i := range uncached {
		if i >= len(batchResp.Results) {
			break
		}
		ids, err := c.paginateResult(ctx, osvQueries[i], batchResp.Results[i])
		if err != nil {
			return results, err
		}
		vulnIDs[i] = ids
	}

	records := c.hydrateVulns(ctx, vulnIDs)

	now := time.Now()
	expires := now.Add(cveCacheTTL)

	for i, q := range uncached {
		var entries []*CVECacheEntry
		for _, id := range vulnIDs[i] {
			rec, ok := records[id]
			if !ok {
				continue
			}
			entry := c.buildCVECacheEntry(q, rec, now, expires)
			if _, err := c.store.InsertCVECacheEntry(ctx, entry); err != nil {
				c.logger.Warn("cve: failed to cache entry", "cve", entry.CVEID, "error", err)
			}
			entries = append(entries, entry)
		}
		results[q.ContainerID] = entries
	}

	return results, nil
}

// paginateResult follows next_page_token for a single query's batch result,
// re-issuing the batch with only that query until the token is exhausted.
func (c *CVEClient) paginateResult(ctx context.Context, q osvQuery, first osvBatchResult) ([]string, error) {
	ids := idsFromVulns(first.Vulns)
	token := first.NextPageToken
	page := 1

	for token != "" {
		if page >= maxOSVPages {
			c.logger.Warn("cve: osv pagination cap reached", "package", q.Package.Name, "ecosystem", q.Package.Ecosystem)
			break
		}

		select {
		case <-ctx.Done():
			return ids, ctx.Err()
		case <-time.After(c.delay):
		}

		pageQuery := q
		pageQuery.PageToken = token
		resp, err := c.postBatch(ctx, []osvQuery{pageQuery})
		if err != nil {
			return ids, err
		}
		page++
		if len(resp.Results) == 0 {
			break
		}
		ids = append(ids, idsFromVulns(resp.Results[0].Vulns)...)
		token = resp.Results[0].NextPageToken
	}

	return ids, nil
}

func (c *CVEClient) postBatch(ctx context.Context, queries []osvQuery) (osvBatchResponse, error) {
	body, err := json.Marshal(osvBatchRequest{Queries: queries})
	if err != nil {
		return osvBatchResponse{}, fmt.Errorf("marshal osv request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/querybatch", bytes.NewReader(body))
	if err != nil {
		return osvBatchResponse{}, fmt.Errorf("create osv request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return osvBatchResponse{}, fmt.Errorf("osv request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return osvBatchResponse{}, fmt.Errorf("osv returned status %d", resp.StatusCode)
	}

	var batchResp osvBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		return osvBatchResponse{}, fmt.Errorf("decode osv response: %w", err)
	}
	return batchResp, nil
}

// hydrateVulns fetches each distinct vuln id once, memoising across queries.
// A fetch failure for one id is logged and skipped, never fails the run.
func (c *CVEClient) hydrateVulns(ctx context.Context, perQueryIDs [][]string) map[string]*osvVulnRecord {
	records := make(map[string]*osvVulnRecord)
	seen := make(map[string]bool)
	fetched := false

	for _, ids := range perQueryIDs {
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true

			if fetched {
				select {
				case <-ctx.Done():
					return records
				case <-time.After(c.delay):
				}
			}
			fetched = true

			rec, err := c.fetchVuln(ctx, id)
			if err != nil {
				c.logger.Warn("cve: failed to fetch vuln", "id", id, "error", err)
				continue
			}
			records[id] = rec
		}
	}

	return records
}

func (c *CVEClient) fetchVuln(ctx context.Context, id string) (*osvVulnRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/vulns/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, fmt.Errorf("create osv vuln request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("osv vuln request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osv vuln %s returned status %d", id, resp.StatusCode)
	}

	var rec osvVulnRecord
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		return nil, fmt.Errorf("decode osv vuln %s: %w", id, err)
	}
	return &rec, nil
}

func (c *CVEClient) buildCVECacheEntry(q ImageCVEQuery, rec *osvVulnRecord, now, expires time.Time) *CVECacheEntry {
	entry := &CVECacheEntry{
		Ecosystem:      q.Ecosystem,
		PackageName:    q.PackageName,
		PackageVersion: q.Version,
		CVEID:          rec.ID,
		Summary:        summaryFor(rec),
		FixedIn:        fixedInFor(q, rec),
		Severity:       CVESeverityUnknown,
		FetchedAt:      now,
		ExpiresAt:      expires,
	}

	if vector, ok := cvssV3Vector(rec); ok {
		entry.CVSSVector = vector
		if metrics, err := ParseCVSSv3(vector); err == nil {
			entry.CVSSScore = metrics.BaseScore()
			entry.Severity = SeverityForScore(entry.CVSSScore)
		} else if !errors.Is(err, ErrInvalidCVSS) {
			c.logger.Warn("cve: unexpected cvss parse error", "cve", rec.ID, "error", err)
		}
	}

	if len(rec.References) > 0 {
		if b, err := json.Marshal(rec.References); err == nil {
			entry.ReferencesJSON = string(b)
		}
	}

	return entry
}

func idsFromVulns(vulns []osvBatchVuln) []string {
	ids := make([]string, len(vulns))
	for i, v := range vulns {
		ids[i] = v.ID
	}
	return ids
}

func cvssV3Vector(rec *osvVulnRecord) (string, bool) {
	for _, s := range rec.Severity {
		if s.Type == "CVSS_V3" {
			return s.Score, true
		}
	}
	return "", false
}

func fixedInFor(q ImageCVEQuery, rec *osvVulnRecord) string {
	for _, a := range rec.Affected {
		if a.Package.Ecosystem != q.Ecosystem || !strings.EqualFold(a.Package.Name, q.PackageName) {
			continue
		}
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

func summaryFor(rec *osvVulnRecord) string {
	if rec.Summary != "" {
		return rec.Summary
	}
	return truncate(rec.Details, cveSummaryMax)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
