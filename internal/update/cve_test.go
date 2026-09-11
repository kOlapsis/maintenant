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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stubs ---

// cveStubStore is a stubStore that gives controllable CVE cache behavior;
// every other UpdateStore method is the stubStore no-op default.
type cveStubStore struct {
	stubStore
	mu            sync.Mutex
	fresh         map[string]bool
	cachedEntries map[string][]*CVECacheEntry
	inserted      []*CVECacheEntry
}

func cveCacheKey(ecosystem, packageName, packageVersion string) string {
	return ecosystem + "|" + packageName + "|" + packageVersion
}

func (s *cveStubStore) IsCVECacheFresh(_ context.Context, ecosystem, packageName, packageVersion string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fresh[cveCacheKey(ecosystem, packageName, packageVersion)], nil
}

func (s *cveStubStore) GetCVECacheEntries(_ context.Context, ecosystem, packageName, packageVersion string) ([]*CVECacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cachedEntries[cveCacheKey(ecosystem, packageName, packageVersion)], nil
}

func (s *cveStubStore) InsertCVECacheEntry(_ context.Context, e *CVECacheEntry) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inserted = append(s.inserted, e)
	return "cache-id", nil
}

// requestLog records the OSV requests a test server received, safe for
// concurrent use since httptest handlers run on their own goroutine.
type requestLog struct {
	mu    sync.Mutex
	batch []osvBatchRequest
	vulns []string
}

func (l *requestLog) recordBatch(r osvBatchRequest) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.batch = append(l.batch, r)
}

func (l *requestLog) recordVuln(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.vulns = append(l.vulns, id)
}

func (l *requestLog) batchCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.batch)
}

func (l *requestLog) batchAt(i int) osvBatchRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.batch[i]
}

func (l *requestLog) vulnCount(id string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, v := range l.vulns {
		if v == id {
			n++
		}
	}
	return n
}

func (l *requestLog) totalVulnRequests() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.vulns)
}

func newTestCVEClient(store UpdateStore, serverURL string) *CVEClient {
	c := NewCVEClient(store, testLogger())
	c.baseURL = serverURL
	c.delay = 0
	return c
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	assert.NoError(t, json.NewEncoder(w).Encode(v))
}

// --- tests ---

func TestQueryCVEs_HydratesFromVulnsEndpoint(t *testing.T) {
	log := &requestLog{}
	vulnRecords := map[string]osvVulnRecord{
		"CVE-2024-0001": {
			ID:      "CVE-2024-0001",
			Summary: "OpenSSL vulnerable to buffer overflow",
			Severity: []osvSeverity{
				{Type: "CVSS_V3", Score: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
			},
			Affected: []osvAffected{
				{
					Package: osvPackage{Ecosystem: "Debian", Name: "openssl"},
					Ranges: []osvRange{
						{Type: "ECOSYSTEM", Events: []osvEvent{{Introduced: "0"}, {Fixed: "1.2.3"}}},
					},
				},
			},
		},
		"GHSA-xxxx": {
			ID:      "GHSA-xxxx",
			Summary: "Unscored advisory",
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		var req osvBatchRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		log.recordBatch(req)
		writeJSON(t, w, osvBatchResponse{
			Results: []osvBatchResult{
				{Vulns: []osvBatchVuln{{ID: "CVE-2024-0001"}, {ID: "GHSA-xxxx"}}},
			},
		})
	})
	mux.HandleFunc("/v1/vulns/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/vulns/")
		log.recordVuln(id)
		rec, ok := vulnRecords[id]
		assert.True(t, ok, "unexpected vuln id %q", id)
		writeJSON(t, w, rec)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestCVEClient(&cveStubStore{}, server.URL)

	result, err := client.QueryCVEs(context.Background(), []ImageCVEQuery{
		{ContainerID: "c1", PackageName: "openssl", Ecosystem: "Debian", Version: "1.1.1"},
	})
	require.NoError(t, err)

	entries := result["c1"]
	require.Len(t, entries, 2)

	byID := make(map[string]*CVECacheEntry, len(entries))
	for _, e := range entries {
		byID[e.CVEID] = e
	}

	critical := byID["CVE-2024-0001"]
	require.NotNil(t, critical)
	assert.Equal(t, "OpenSSL vulnerable to buffer overflow", critical.Summary)
	assert.Equal(t, CVESeverityCritical, critical.Severity)
	assert.InDelta(t, 9.8, critical.CVSSScore, 0.001)
	assert.Equal(t, "1.2.3", critical.FixedIn)

	unscored := byID["GHSA-xxxx"]
	require.NotNil(t, unscored)
	assert.Equal(t, CVESeverityUnknown, unscored.Severity)
	assert.Equal(t, float64(0), unscored.CVSSScore)
}

func TestQueryCVEs_BatchResponseCarriesNoDetails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		// Deliberately malformed per the OSV contract: a real querybatch
		// response never carries summary/severity, only id and modified.
		_, _ = w.Write([]byte(`{"results":[{"vulns":[{"id":"CVE-2024-0002","summary":"WRONG, MUST BE IGNORED","severity":[{"type":"CVSS_V3","score":"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N"}]}]}]}`))
	})
	mux.HandleFunc("/v1/vulns/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, osvVulnRecord{
			ID:      "CVE-2024-0002",
			Summary: "Real summary from the vulns endpoint",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestCVEClient(&cveStubStore{}, server.URL)

	result, err := client.QueryCVEs(context.Background(), []ImageCVEQuery{
		{ContainerID: "c1", PackageName: "curl", Ecosystem: "Debian", Version: "7.0"},
	})
	require.NoError(t, err)

	entries := result["c1"]
	require.Len(t, entries, 1)
	assert.Equal(t, "Real summary from the vulns endpoint", entries[0].Summary)
	assert.Equal(t, CVESeverityUnknown, entries[0].Severity)
}

func TestQueryCVEs_Paginates(t *testing.T) {
	log := &requestLog{}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		var req osvBatchRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		log.recordBatch(req)

		if len(req.Queries) == 1 && req.Queries[0].PageToken == "tok1" {
			writeJSON(t, w, osvBatchResponse{
				Results: []osvBatchResult{{Vulns: []osvBatchVuln{{ID: "CVE-B"}}}},
			})
			return
		}
		writeJSON(t, w, osvBatchResponse{
			Results: []osvBatchResult{{Vulns: []osvBatchVuln{{ID: "CVE-A"}}, NextPageToken: "tok1"}},
		})
	})
	mux.HandleFunc("/v1/vulns/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/vulns/")
		log.recordVuln(id)
		writeJSON(t, w, osvVulnRecord{ID: id, Summary: "summary for " + id})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestCVEClient(&cveStubStore{}, server.URL)

	result, err := client.QueryCVEs(context.Background(), []ImageCVEQuery{
		{ContainerID: "c1", PackageName: "libfoo", Ecosystem: "Debian", Version: "2.0"},
	})
	require.NoError(t, err)

	entries := result["c1"]
	require.Len(t, entries, 2)

	ids := map[string]bool{}
	for _, e := range entries {
		ids[e.CVEID] = true
	}
	assert.True(t, ids["CVE-A"])
	assert.True(t, ids["CVE-B"])

	require.Equal(t, 2, log.batchCount())
	assert.Equal(t, "tok1", log.batchAt(1).Queries[0].PageToken)
}

func TestQueryCVEs_MemoisesVulnFetchAcrossQueries(t *testing.T) {
	log := &requestLog{}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		var req osvBatchRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		log.recordBatch(req)

		results := make([]osvBatchResult, len(req.Queries))
		for i := range req.Queries {
			results[i] = osvBatchResult{Vulns: []osvBatchVuln{{ID: "CVE-SHARED"}}}
		}
		writeJSON(t, w, osvBatchResponse{Results: results})
	})
	mux.HandleFunc("/v1/vulns/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/vulns/")
		log.recordVuln(id)
		writeJSON(t, w, osvVulnRecord{ID: id, Summary: "shared vuln"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestCVEClient(&cveStubStore{}, server.URL)

	result, err := client.QueryCVEs(context.Background(), []ImageCVEQuery{
		{ContainerID: "c1", PackageName: "libfoo", Ecosystem: "Debian", Version: "1.0"},
		{ContainerID: "c2", PackageName: "libfoo", Ecosystem: "Debian", Version: "1.1"},
	})
	require.NoError(t, err)

	require.Len(t, result["c1"], 1)
	require.Len(t, result["c2"], 1)
	assert.Equal(t, "CVE-SHARED", result["c1"][0].CVEID)
	assert.Equal(t, "CVE-SHARED", result["c2"][0].CVEID)

	assert.Equal(t, 1, log.vulnCount("CVE-SHARED"))
	assert.Equal(t, 1, log.totalVulnRequests())
}

func TestQueryCVEs_VulnFetchFailureSkipsOnlyThatID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, osvBatchResponse{
			Results: []osvBatchResult{{Vulns: []osvBatchVuln{{ID: "CVE-A"}, {ID: "CVE-B"}}}},
		})
	})
	mux.HandleFunc("/v1/vulns/CVE-A", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/v1/vulns/CVE-B", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, osvVulnRecord{ID: "CVE-B", Summary: "fine"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestCVEClient(&cveStubStore{}, server.URL)

	result, err := client.QueryCVEs(context.Background(), []ImageCVEQuery{
		{ContainerID: "c1", PackageName: "libfoo", Ecosystem: "Debian", Version: "1.0"},
	})
	require.NoError(t, err)

	entries := result["c1"]
	require.Len(t, entries, 1)
	assert.Equal(t, "CVE-B", entries[0].CVEID)
}

func TestQueryCVEs_UsesFreshCacheWithoutNetwork(t *testing.T) {
	requests := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cached := []*CVECacheEntry{{CVEID: "CVE-CACHED", Severity: CVESeverityHigh}}
	store := &cveStubStore{
		fresh:         map[string]bool{cveCacheKey("Debian", "openssl", "1.1.1"): true},
		cachedEntries: map[string][]*CVECacheEntry{cveCacheKey("Debian", "openssl", "1.1.1"): cached},
	}
	client := newTestCVEClient(store, server.URL)

	result, err := client.QueryCVEs(context.Background(), []ImageCVEQuery{
		{ContainerID: "c1", PackageName: "openssl", Ecosystem: "Debian", Version: "1.1.1"},
	})
	require.NoError(t, err)

	assert.Equal(t, cached, result["c1"])
	assert.Equal(t, 0, requests)
	assert.Empty(t, store.inserted)
}
