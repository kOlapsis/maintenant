// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enricherStubStore records what the enricher persists and always resolves a
// pending update, so changelog and risk enrichment can actually run.
type enricherStubStore struct {
	stubStore
	mu          sync.Mutex
	imageUpdate *update.ImageUpdate
	evaluations map[string]*update.CVEEvaluation
	cves        []*update.ContainerCVE
	riskRecords []*update.RiskScoreRecord
}

func newEnricherStubStore() *enricherStubStore {
	return &enricherStubStore{
		imageUpdate: &update.ImageUpdate{ID: "u1", ContainerID: "c1", UpdateType: update.UpdateTypeMinor},
		evaluations: make(map[string]*update.CVEEvaluation),
	}
}

func (s *enricherStubStore) GetAgentImageUpdate(_ context.Context, _, containerID string) (*update.ImageUpdate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.imageUpdate == nil || s.imageUpdate.ContainerID != containerID {
		return nil, nil
	}
	return s.imageUpdate, nil
}

func (s *enricherStubStore) UpsertCVEEvaluation(_ context.Context, e *update.CVEEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluations[e.ContainerID] = e
	return nil
}

func (s *enricherStubStore) UpsertContainerCVE(_ context.Context, c *update.ContainerCVE) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cves = append(s.cves, c)
	return nil
}

func (s *enricherStubStore) InsertRiskScoreRecord(_ context.Context, r *update.RiskScoreRecord) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.riskRecords = append(s.riskRecords, r)
	return "risk-1", nil
}

func (s *enricherStubStore) evaluation(containerID string) *update.CVEEvaluation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evaluations[containerID]
}

func (s *enricherStubStore) storedCVEs() []*update.ContainerCVE {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*update.ContainerCVE(nil), s.cves...)
}

func (s *enricherStubStore) storedRiskRecords() []*update.RiskScoreRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*update.RiskScoreRecord(nil), s.riskRecords...)
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

func newTestEnricher(store update.UpdateStore, cve *CVEClient) *ProEnricher {
	return NewProEnricher(store, cve, nil, NewRiskEngine(nil, nil), NewEcosystemResolver(nil, testLogger()), testLogger())
}

func TestEnrichScansContainerWithoutUpdate(t *testing.T) {
	server := osvTestServer(t)
	defer server.Close()

	store := newEnricherStubStore()
	enricher := newTestEnricher(store, newTestCVEClient(store, server.URL))

	results := []update.UpdateResult{{
		ContainerID:   "c1",
		ContainerName: "web",
		Image:         "nginx",
		CurrentTag:    "1.27.0",
		HasUpdate:     false,
	}}
	require.NoError(t, enricher.Enrich(context.Background(), results))

	eval := store.evaluation("c1")
	require.NotNil(t, eval, "an up-to-date container must still be evaluated")
	assert.Equal(t, update.CVEEvaluated, eval.Status)
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

	results := []update.UpdateResult{{
		ContainerID:   "c1",
		ContainerName: "web",
		Image:         "nginx",
		CurrentTag:    "1.27.0",
		LatestTag:     "1.28.0",
		UpdateType:    update.UpdateTypeMinor,
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

	results := []update.UpdateResult{{
		ContainerID:   "c1",
		ContainerName: "tiny",
		Image:         "scratch",
		CurrentTag:    "latest",
	}}
	require.NoError(t, enricher.Enrich(context.Background(), results))

	eval := store.evaluation("c1")
	require.NotNil(t, eval)
	assert.Equal(t, update.CVEUnsupported, eval.Status)
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

	results := []update.UpdateResult{{
		ContainerID:   "c1",
		ContainerName: "web",
		Image:         "nginx",
		CurrentTag:    "1.27.0",
	}}
	require.NoError(t, enricher.Enrich(context.Background(), results))

	eval := store.evaluation("c1")
	require.NotNil(t, eval)
	assert.Equal(t, update.CVEEvaluationError, eval.Status)
	assert.NotEmpty(t, eval.Error)
	assert.Empty(t, store.storedCVEs(), "a failed query must not invent a clean bill of health")
}

type insightsStub map[string]*security.ContainerInsights

func (s insightsStub) GetContainerInsights(containerID string) *security.ContainerInsights {
	return s[containerID]
}

type restartsStub map[string]int

func (s restartsStub) CountRestartsSince(_ context.Context, containerID string, _ time.Time) (int, error) {
	return s[containerID], nil
}

// Exposure and restarts come from what monitoring knows about the container,
// and no factor without a source is left in the score.
func TestEnrichRiskWeighsExposureAndRestarts(t *testing.T) {
	store := newEnricherStubStore()
	store.imageUpdate.UpdateType = update.UpdateTypePatch
	risk := NewRiskEngine(
		insightsStub{"uid-1": {Insights: []security.Insight{{Type: security.PortExposedAllInterfaces}}}},
		restartsStub{"uid-1": 6},
	)
	enricher := NewProEnricher(store, nil, nil, risk, nil, testLogger())

	results := []update.UpdateResult{{
		ContainerID:   "c1",
		ContainerUID:  "uid-1",
		ContainerName: "web",
		Image:         "nginx",
		CurrentTag:    "1.27.0",
		LatestTag:     "1.27.1",
		UpdateType:    update.UpdateTypePatch,
		RiskScore:     update.BaseRiskScore(update.UpdateTypePatch),
		HasUpdate:     true,
	}}
	require.NoError(t, enricher.Enrich(context.Background(), results))

	records := store.storedRiskRecords()
	require.Len(t, records, 1)
	var factors map[string]update.RiskFactor
	require.NoError(t, json.Unmarshal([]byte(records[0].FactorsJSON), &factors))
	assert.Equal(t, update.RiskFactor{Label: "yes", Score: 0}, factors["public_exposure"], "exposure without a CVE adds nothing")
	assert.Equal(t, 10, factors["stability"].Score)
	assert.NotContains(t, factors, "criticality")
	assert.NotContains(t, factors, "dependents")
	assert.Equal(t, 55, records[0].Score, "patch 45 + restarts 10")
	assert.Equal(t, 55, results[0].RiskScore, "the final score is written back on the result")
	assert.Equal(t, 55, store.imageUpdate.RiskScore, "and on the stored update")
}

type failingUpdateStore struct {
	*enricherStubStore
}

func (s failingUpdateStore) UpdateImageUpdate(_ context.Context, _ *update.ImageUpdate) error {
	return errors.New("write failed")
}

func TestEnrichRisk_KeepsTheScanScoreWhenTheWriteFails(t *testing.T) {
	store := newEnricherStubStore()
	risk := NewRiskEngine(nil, restartsStub{"uid-1": 6})
	enricher := NewProEnricher(failingUpdateStore{store}, nil, nil, risk, nil, testLogger())

	results := []update.UpdateResult{{
		ContainerID:  "c1",
		ContainerUID: "uid-1",
		Image:        "nginx",
		CurrentTag:   "1.27.0",
		LatestTag:    "1.27.1",
		UpdateType:   update.UpdateTypePatch,
		RiskScore:    update.BaseRiskScore(update.UpdateTypePatch),
		HasUpdate:    true,
	}}
	require.NoError(t, enricher.Enrich(context.Background(), results))

	assert.Equal(t, update.BaseRiskScore(update.UpdateTypePatch), results[0].RiskScore)
}

func cvesOf(severities ...update.CVESeverity) []*update.ContainerCVE {
	cves := make([]*update.ContainerCVE, 0, len(severities))
	for _, s := range severities {
		cves = append(cves, &update.ContainerCVE{Severity: s, FixedIn: "1.0.1"})
	}
	return cves
}

func TestCalculateScore_MeasuresTheUrgencyOfUpdating(t *testing.T) {
	cases := []struct {
		name       string
		updateType update.UpdateType
		scanScore  int
		cves       []*update.ContainerCVE
		rctx       RiskContext
		want       int
	}{
		{"patch fixing a critical CVE", update.UpdateTypePatch, 45, cvesOf(update.CVESeverityLow, update.CVESeverityCritical), RiskContext{}, 90},
		{"major alone", update.UpdateTypeMajor, 20, nil, RiskContext{}, 20},
		{"high CVE", update.UpdateTypeMinor, 40, cvesOf(update.CVESeverityHigh), RiskContext{}, 65},
		{"medium CVE below the scan score", update.UpdateTypePatch, 45, cvesOf(update.CVESeverityMedium), RiskContext{}, 45},
		{"exposure counts with a CVE", update.UpdateTypeMinor, 40, cvesOf(update.CVESeverityHigh), RiskContext{PubliclyExposed: true}, 75},
		{"exposure alone adds nothing", update.UpdateTypeMinor, 40, nil, RiskContext{PubliclyExposed: true}, 40},
		{"end of life kept", update.UpdateTypeMajor, 85, cvesOf(update.CVESeverityLow), RiskContext{}, 85},
		{"capped at 100", update.UpdateTypePatch, 45, cvesOf(update.CVESeverityCritical), RiskContext{PubliclyExposed: true, RestartCount: 6}, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &update.UpdateResult{ContainerID: "c1", UpdateType: tc.updateType, RiskScore: tc.scanScore, LatestTag: "1.0.1"}
			got := NewRiskEngine(nil, nil).CalculateScore(r, tc.cves, tc.rctx)
			assert.Equal(t, tc.want, got.Score)
			assert.Equal(t, update.RiskLevelFromScore(tc.want), got.Level)
		})
	}
}

func TestCalculateScore_CountsOnlyTheCVEsTheUpdateFixes(t *testing.T) {
	cases := []struct {
		name         string
		latest       string
		major        string
		fixedIn      string
		exposed      bool
		want         int
		wantSeverity string
	}{
		{"critical CVE without a fix version", "1.0.1", "", "", true, 45, "none"},
		{"critical CVE fixed above the target", "1.0.1", "", "1.0.5", true, 45, "none"},
		{"critical CVE fixed by the target", "1.0.5", "", "1.0.5", false, 90, "critical"},
		{"fixed by the target, exposed", "1.0.5", "", "1.0.5", true, 100, "critical"},
		{"critical CVE fixed only by the later major", "1.0.1", "2.0.0", "1.9.0", false, 90, "critical"},
		{"critical CVE fixed above the later major", "1.0.1", "2.0.0", "3.0.0", true, 45, "none"},
		{"unparseable fix version", "1.0.1", "2.0.0", "not-a-version", true, 45, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &update.UpdateResult{ContainerID: "c1", UpdateType: update.UpdateTypePatch, RiskScore: 45, LatestTag: tc.latest, MajorTag: tc.major}
			cves := []*update.ContainerCVE{{Severity: update.CVESeverityCritical, FixedIn: tc.fixedIn}}
			got := NewRiskEngine(nil, nil).CalculateScore(r, cves, RiskContext{PubliclyExposed: tc.exposed})
			assert.Equal(t, tc.want, got.Score)
			assert.Equal(t, tc.wantSeverity, got.Factors["cve_severity"].Label)
		})
	}
}

func TestCalculateScore_Factors(t *testing.T) {
	r := &update.UpdateResult{ContainerID: "c1", UpdateType: update.UpdateTypeMajor, RiskScore: 85, LatestTag: "1.0.1"}
	got := NewRiskEngine(nil, nil).CalculateScore(r, cvesOf(update.CVESeverityHigh), RiskContext{PubliclyExposed: true, RestartCount: 1})

	assert.Equal(t, map[string]update.RiskFactor{
		"update_type":     {Label: "major", Score: 20},
		"end_of_life":     {Label: "yes", Score: 85},
		"cve_severity":    {Label: "high", Score: 65},
		"public_exposure": {Label: "yes", Score: 10},
		"stability":       {Label: "minor_restarts", Score: 3},
	}, got.Factors)
	assert.Equal(t, 88, got.Score)

	r.RiskScore = update.BaseRiskScore(update.UpdateTypeMajor)
	got = NewRiskEngine(nil, nil).CalculateScore(r, nil, RiskContext{})
	assert.Equal(t, update.RiskFactor{Label: "no", Score: 0}, got.Factors["end_of_life"])
}

func TestShortErrorTruncates(t *testing.T) {
	msg := shortError(&stringError{s: strings.Repeat("x", maxEvaluationErrorLen+50)})
	assert.Len(t, msg, maxEvaluationErrorLen)
}

type stringError struct{ s string }

func (e *stringError) Error() string { return e.s }
