// Copyright 2026 Benjamin Touchard (Kolapsis)
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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enricherStubStore records what the enricher persists and always resolves a
// pending update, so changelog and risk enrichment can actually run.
type enricherStubStore struct {
	stubStore
	mu          sync.Mutex
	imageUpdate *ImageUpdate
	evaluations map[string]*CVEEvaluation
	cves        []*ContainerCVE
	riskRecords []*RiskScoreRecord
}

func newEnricherStubStore() *enricherStubStore {
	return &enricherStubStore{
		imageUpdate: &ImageUpdate{ID: "u1", ContainerID: "c1", UpdateType: UpdateTypeMinor},
		evaluations: make(map[string]*CVEEvaluation),
	}
}

func (s *enricherStubStore) GetImageUpdateByContainer(_ context.Context, containerID string) (*ImageUpdate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.imageUpdate == nil || s.imageUpdate.ContainerID != containerID {
		return nil, nil
	}
	return s.imageUpdate, nil
}

func (s *enricherStubStore) UpsertCVEEvaluation(_ context.Context, e *CVEEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluations[e.ContainerID] = e
	return nil
}

func (s *enricherStubStore) UpsertContainerCVE(_ context.Context, c *ContainerCVE) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cves = append(s.cves, c)
	return nil
}

func (s *enricherStubStore) InsertRiskScoreRecord(_ context.Context, r *RiskScoreRecord) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.riskRecords = append(s.riskRecords, r)
	return "risk-1", nil
}

func (s *enricherStubStore) evaluation(containerID string) *CVEEvaluation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evaluations[containerID]
}

func (s *enricherStubStore) storedCVEs() []*ContainerCVE {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*ContainerCVE(nil), s.cves...)
}

func (s *enricherStubStore) storedRiskRecords() []*RiskScoreRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*RiskScoreRecord(nil), s.riskRecords...)
}

// osvTestServer serves one vulnerability for every batch query.
func osvTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, osvBatchResponse{
			Results: []osvBatchResult{{Vulns: []osvBatchVuln{{ID: "CVE-2024-9999"}}}},
		})
	})
	mux.HandleFunc("/v1/vulns/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, osvVulnRecord{
			ID:      strings.TrimPrefix(r.URL.Path, "/v1/vulns/"),
			Summary: "test advisory",
			Severity: []osvSeverity{
				{Type: "CVSS_V3", Score: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
			},
		})
	})
	return httptest.NewServer(mux)
}

func newTestEnricher(store UpdateStore, cve *CVEClient) *ProEnricher {
	return NewProEnricher(store, cve, nil, NewRiskEngine(), NewEcosystemResolver(nil, testLogger()), testLogger())
}

func TestEnrichScansContainerWithoutUpdate(t *testing.T) {
	server := osvTestServer(t)
	defer server.Close()

	store := newEnricherStubStore()
	enricher := newTestEnricher(store, newTestCVEClient(store, server.URL))

	results := []UpdateResult{{
		ContainerID:   "c1",
		ContainerName: "web",
		Image:         "nginx",
		CurrentTag:    "1.27.0",
		HasUpdate:     false,
	}}
	require.NoError(t, enricher.Enrich(context.Background(), results))

	eval := store.evaluation("c1")
	require.NotNil(t, eval, "an up-to-date container must still be evaluated")
	assert.Equal(t, CVEEvaluated, eval.Status)
	assert.Equal(t, "Debian:12", eval.Ecosystem)
	assert.Equal(t, "nginx", eval.PackageName)
	assert.Equal(t, "1.27.0", eval.PackageVersion)

	require.Len(t, store.storedCVEs(), 1)
	assert.Empty(t, store.storedRiskRecords(), "risk scoring describes an update that does not exist")
	assert.Empty(t, results[0].ChangelogURL)
}

func TestEnrichRunsChangelogAndRiskOnlyWithUpdate(t *testing.T) {
	server := osvTestServer(t)
	defer server.Close()

	store := newEnricherStubStore()
	enricher := newTestEnricher(store, newTestCVEClient(store, server.URL))

	results := []UpdateResult{{
		ContainerID:   "c1",
		ContainerName: "web",
		Image:         "nginx",
		CurrentTag:    "1.27.0",
		LatestTag:     "1.28.0",
		UpdateType:    UpdateTypeMinor,
		HasUpdate:     true,
	}}
	require.NoError(t, enricher.Enrich(context.Background(), results))

	require.NotNil(t, store.evaluation("c1"))
	assert.Len(t, store.storedRiskRecords(), 1)
}

func TestEnrichRecordsUnsupportedWithoutEcosystem(t *testing.T) {
	server := osvTestServer(t)
	defer server.Close()

	store := newEnricherStubStore()
	enricher := newTestEnricher(store, newTestCVEClient(store, server.URL))

	results := []UpdateResult{{
		ContainerID:   "c1",
		ContainerName: "tiny",
		Image:         "scratch",
		CurrentTag:    "latest",
	}}
	require.NoError(t, enricher.Enrich(context.Background(), results))

	eval := store.evaluation("c1")
	require.NotNil(t, eval)
	assert.Equal(t, CVEUnsupported, eval.Status)
	assert.Empty(t, eval.Ecosystem)
	assert.Empty(t, store.storedCVEs())
}

func TestEnrichRecordsErrorOnQueryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	store := newEnricherStubStore()
	enricher := newTestEnricher(store, newTestCVEClient(store, server.URL))

	results := []UpdateResult{{
		ContainerID:   "c1",
		ContainerName: "web",
		Image:         "nginx",
		CurrentTag:    "1.27.0",
	}}
	require.NoError(t, enricher.Enrich(context.Background(), results))

	eval := store.evaluation("c1")
	require.NotNil(t, eval)
	assert.Equal(t, CVEEvaluationError, eval.Status)
	assert.NotEmpty(t, eval.Error)
	assert.Empty(t, store.storedCVEs(), "a failed query must not invent a clean bill of health")
}

func TestShortErrorTruncates(t *testing.T) {
	msg := shortError(&stringError{s: strings.Repeat("x", maxEvaluationErrorLen+50)})
	assert.Len(t, msg, maxEvaluationErrorLen)
}

type stringError struct{ s string }

func (e *stringError) Error() string { return e.s }
