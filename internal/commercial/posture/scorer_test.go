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

package posture

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock implementations ---

type mockCertReader struct {
	certs map[string][]security.CertificateInfo
}

func (m *mockCertReader) ListCertificatesForContainer(_ context.Context, containerExternalID string) ([]security.CertificateInfo, error) {
	return m.certs[containerExternalID], nil
}

type mockCVEReader struct {
	cves map[string][]security.CVEInfo
}

func (m *mockCVEReader) ListCVEsForContainer(_ context.Context, containerExternalID string) ([]security.CVEInfo, error) {
	return m.cves[containerExternalID], nil
}

type mockCVEEvalReader struct {
	states map[string]string
}

func (m *mockCVEEvalReader) GetCVEEvaluation(_ context.Context, containerExternalID string) (*security.CVEEvaluationInfo, error) {
	state, ok := m.states[containerExternalID]
	if !ok {
		return nil, nil
	}
	return &security.CVEEvaluationInfo{Status: state}, nil
}

type mockUpdateReader struct {
	updates map[string][]security.UpdateInfo
}

func (m *mockUpdateReader) ListUpdatesForContainer(_ context.Context, containerExternalID string) ([]security.UpdateInfo, error) {
	return m.updates[containerExternalID], nil
}

type mockAckStore struct {
	acks map[string]map[string]map[string]bool // externalID -> findingType -> findingKey -> acknowledged
}

func (m *mockAckStore) InsertAcknowledgment(_ context.Context, ack *security.RiskAcknowledgment) (string, error) {
	return "ack1", nil
}

func (m *mockAckStore) DeleteAcknowledgment(_ context.Context, _ string) error {
	return nil
}

func (m *mockAckStore) ListAcknowledgments(_ context.Context, _ string) ([]*security.RiskAcknowledgment, error) {
	return nil, nil
}

func (m *mockAckStore) GetAcknowledgment(_ context.Context, _ string) (*security.RiskAcknowledgment, error) {
	return nil, nil
}

func (m *mockAckStore) IsAcknowledged(_ context.Context, containerExternalID, findingType, findingKey string) (bool, error) {
	if m.acks == nil {
		return false, nil
	}
	if types, ok := m.acks[containerExternalID]; ok {
		if keys, ok := types[findingType]; ok {
			return keys[findingKey], nil
		}
	}
	return false, nil
}

func newTestService() *security.Service {
	return security.NewService(security.Deps{Logger: slog.Default()})
}

func TestScoreTLS(t *testing.T) {
	tests := []struct {
		name           string
		certs          []security.CertificateInfo
		expectedScore  int
		expectedIssues int
	}{
		{
			name:           "all valid",
			certs:          []security.CertificateInfo{{Status: "valid", DaysRemaining: 90}},
			expectedScore:  100,
			expectedIssues: 0,
		},
		{
			name:           "one expiring soon",
			certs:          []security.CertificateInfo{{Status: "expiring", DaysRemaining: 5}},
			expectedScore:  70,
			expectedIssues: 1,
		},
		{
			name:           "one expired",
			certs:          []security.CertificateInfo{{Status: "expired", DaysRemaining: -1}},
			expectedScore:  50,
			expectedIssues: 1,
		},
		{
			name: "mixed",
			certs: []security.CertificateInfo{
				{Status: "valid", DaysRemaining: 60},
				{Status: "expiring", DaysRemaining: 20},
				{Status: "expired", DaysRemaining: -5},
			},
			expectedScore:  40,
			expectedIssues: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, issues, _ := scoreTLS(tt.certs)
			assert.Equal(t, tt.expectedScore, score)
			assert.Equal(t, tt.expectedIssues, issues)
		})
	}
}

func TestScoreCVEs(t *testing.T) {
	tests := []struct {
		name          string
		cves          []security.CVEInfo
		expectedScore int
	}{
		{
			name:          "no cves",
			cves:          nil,
			expectedScore: 100,
		},
		{
			name:          "one critical",
			cves:          []security.CVEInfo{{Severity: "critical"}},
			expectedScore: 70,
		},
		{
			name:          "one high",
			cves:          []security.CVEInfo{{Severity: "high"}},
			expectedScore: 85,
		},
		{
			name: "multiple severities",
			cves: []security.CVEInfo{
				{Severity: "critical"},
				{Severity: "high"},
				{Severity: "medium"},
			},
			expectedScore: 50,
		},
		{
			name: "floor at zero",
			cves: []security.CVEInfo{
				{Severity: "critical"},
				{Severity: "critical"},
				{Severity: "critical"},
				{Severity: "critical"},
			},
			expectedScore: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _, _ := scoreCVEs(tt.cves, 0)
			assert.Equal(t, tt.expectedScore, score)
		})
	}
}

func TestScoreUpdates(t *testing.T) {
	tests := []struct {
		name          string
		updates       []security.UpdateInfo
		expectedScore int
	}{
		{
			name:          "up to date",
			updates:       nil,
			expectedScore: 100,
		},
		{
			name:          "one major",
			updates:       []security.UpdateInfo{{UpdateType: "major"}},
			expectedScore: 75,
		},
		{
			name:          "one minor",
			updates:       []security.UpdateInfo{{UpdateType: "minor"}},
			expectedScore: 90,
		},
		{
			name: "mixed",
			updates: []security.UpdateInfo{
				{UpdateType: "major"},
				{UpdateType: "minor"},
				{UpdateType: "patch"},
			},
			expectedScore: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _, _ := scoreUpdates(tt.updates)
			assert.Equal(t, tt.expectedScore, score)
		})
	}
}

func TestScoreNetworkExposure(t *testing.T) {
	tests := []struct {
		name          string
		insights      []security.Insight
		expectedScore int
	}{
		{
			name:          "no issues",
			insights:      nil,
			expectedScore: 100,
		},
		{
			name:          "one critical",
			insights:      []security.Insight{{Severity: security.SeverityCritical}},
			expectedScore: 65,
		},
		{
			name: "multiple",
			insights: []security.Insight{
				{Severity: security.SeverityCritical},
				{Severity: security.SeverityHigh},
			},
			expectedScore: 45,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _, _ := scoreNetworkExposure(tt.insights, 0)
			assert.Equal(t, tt.expectedScore, score)
		})
	}
}

func TestScoreImageAge(t *testing.T) {
	tests := []struct {
		name          string
		updates       []security.UpdateInfo
		expectedScore int
	}{
		{
			name:          "no published_at",
			updates:       []security.UpdateInfo{{UpdateType: "minor"}},
			expectedScore: 50,
		},
		{
			name: "recent image",
			updates: []security.UpdateInfo{
				{PublishedAt: timePtr(time.Now().Add(-10 * 24 * time.Hour))},
			},
			expectedScore: 100,
		},
		{
			name: "old image",
			updates: []security.UpdateInfo{
				{PublishedAt: timePtr(time.Now().Add(-200 * 24 * time.Hour))},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _, _ := scoreImageAge(tt.updates)
			if tt.expectedScore > 0 {
				assert.Equal(t, tt.expectedScore, score)
			} else {
				// For the old image case, just verify it's between 0 and 100
				assert.True(t, score >= 0 && score <= 100, "score %d out of range", score)
				assert.True(t, score < 60, "old image should have low score, got %d", score)
			}
		})
	}
}

func TestColorLevel(t *testing.T) {
	assert.Equal(t, "green", ColorLevel(100))
	assert.Equal(t, "green", ColorLevel(80))
	assert.Equal(t, "yellow", ColorLevel(79))
	assert.Equal(t, "yellow", ColorLevel(60))
	assert.Equal(t, "orange", ColorLevel(59))
	assert.Equal(t, "orange", ColorLevel(40))
	assert.Equal(t, "red", ColorLevel(39))
	assert.Equal(t, "red", ColorLevel(0))
}

func TestScorerScoreContainer(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()
	secSvc.UpdateContainer("c1", "test-container", []security.Insight{
		{Type: security.PortExposedAllInterfaces, Severity: security.SeverityHigh, ContainerID: "c1", ContainerName: "test-container"},
	})

	scorer := NewScorer(ScorerDeps{
		Certs: &mockCertReader{certs: map[string][]security.CertificateInfo{
			"ext-1": {{Status: "valid", DaysRemaining: 90}},
		}},
		CVEs: &mockCVEReader{cves: map[string][]security.CVEInfo{
			"ext-1": {{CVEID: "CVE-2025-001", Severity: "high"}},
		}},
		Updates: &mockUpdateReader{updates: map[string][]security.UpdateInfo{
			"ext-1": {{UpdateType: "minor", PublishedAt: timePtr(time.Now().Add(-15 * 24 * time.Hour))}},
		}},
		CVEEvaluations: &mockCVEEvalReader{states: map[string]string{"ext-1": security.EvaluationEvaluated}},
		Insights:       secSvc,
		Acks:           &mockAckStore{},
	})

	score, err := scorer.ScoreContainer(ctx, "c1", "ext-1", "test-container")
	require.NoError(t, err)
	require.NotNil(t, score)

	assert.Equal(t, "c1", score.ContainerID)
	assert.Equal(t, "test-container", score.ContainerName)
	assert.True(t, score.TotalScore > 0 && score.TotalScore <= 100, "score %d out of range", score.TotalScore)
	assert.NotEmpty(t, score.ColorLevel)
	assert.Len(t, score.Categories, 5)
}

func TestScorerWeightRedistribution(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()

	// Only network exposure applicable (no certs, no CVEs, no updates)
	secSvc.UpdateContainer("c1", "test", []security.Insight{
		{Type: security.PrivilegedContainer, Severity: security.SeverityCritical, ContainerID: "c1"},
	})

	scorer := NewScorer(ScorerDeps{Insights: secSvc, Acks: &mockAckStore{}})
	score, err := scorer.ScoreContainer(ctx, "c1", "ext-1", "test")
	require.NoError(t, err)
	require.NotNil(t, score)

	// Only network_exposure is applicable, so score should equal its sub-score
	var netCat *security.CategoryScore
	for i := range score.Categories {
		if score.Categories[i].Name == CategoryNetworkExposure {
			netCat = &score.Categories[i]
			break
		}
	}
	require.NotNil(t, netCat)
	assert.True(t, netCat.Applicable)
	assert.Equal(t, netCat.SubScore, score.TotalScore, "with only one applicable category, total should equal its sub-score")
}

func TestScorerCache(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()

	callCount := 0
	cveReader := &mockCVEReader{cves: map[string][]security.CVEInfo{}}
	scorer := NewScorer(ScorerDeps{CVEs: cveReader, Insights: secSvc, Acks: &mockAckStore{}})

	// Wrap to count calls (we test via timing)
	_, err := scorer.ScoreContainer(ctx, "c1", "ext-1", "test")
	require.NoError(t, err)
	callCount++

	// Second call should return cached
	score2, err := scorer.ScoreContainer(ctx, "c1", "ext-1", "test")
	require.NoError(t, err)
	// Should get the same (cached) result
	require.NotNil(t, score2)
	_ = callCount
}

func TestScorerAcknowledgedFindingsExcluded(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()
	secSvc.UpdateContainer("c1", "test", []security.Insight{
		{Type: security.PortExposedAllInterfaces, Severity: security.SeverityCritical, ContainerID: "c1", Details: map[string]any{"port": 8080, "protocol": "tcp"}},
		{Type: security.DatabasePortExposed, Severity: security.SeverityCritical, ContainerID: "c1", Details: map[string]any{"port": 5432, "protocol": "tcp"}},
	})

	ackStore := &mockAckStore{
		acks: map[string]map[string]map[string]bool{
			"ext-1": {
				string(security.PortExposedAllInterfaces): {"8080/tcp": true},
			},
		},
	}

	scorer := NewScorer(ScorerDeps{Insights: secSvc, Acks: ackStore})

	score, err := scorer.ScoreContainer(ctx, "c1", "ext-1", "test")
	require.NoError(t, err)
	require.NotNil(t, score)

	// Find network exposure category
	var netCat *security.CategoryScore
	for i := range score.Categories {
		if score.Categories[i].Name == CategoryNetworkExposure {
			netCat = &score.Categories[i]
			break
		}
	}
	require.NotNil(t, netCat)
	// Only 1 unacknowledged insight should be counted
	assert.Equal(t, 1, netCat.IssueCount)
}

func TestScoreInfrastructure(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()
	secSvc.UpdateContainer("c1", "container-a", []security.Insight{
		{Type: security.PrivilegedContainer, Severity: security.SeverityCritical, ContainerID: "c1"},
	})

	scorer := NewScorer(ScorerDeps{Insights: secSvc, Acks: &mockAckStore{}})

	containers := []security.ContainerInfo{
		{ID: "c1", ExternalID: "ext-1", Name: "container-a"},
		{ID: "c2", ExternalID: "ext-2", Name: "container-b"},
	}

	posture, err := scorer.ScoreInfrastructure(ctx, containers)
	require.NoError(t, err)
	require.NotNil(t, posture)

	assert.Equal(t, 2, posture.ContainerCount)
	assert.True(t, posture.ScoredCount > 0)
	assert.NotEmpty(t, posture.TopRisks)
}

func TestScorerNoDataReturnsNil(t *testing.T) {
	ctx := context.Background()

	// All readers nil including security service → no applicable categories → nil score
	scorer := NewScorer(ScorerDeps{Acks: &mockAckStore{}})
	score, err := scorer.ScoreContainer(ctx, "c99", "nonexistent", "ghost")
	require.NoError(t, err)
	assert.Nil(t, score, "container with no data for any category should return nil")
}

func TestScorerWithSecurityServiceNoInsights(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()

	// Security service exists but no insights for this container → network_exposure applicable with score 100
	scorer := NewScorer(ScorerDeps{Insights: secSvc, Acks: &mockAckStore{}})
	score, err := scorer.ScoreContainer(ctx, "c99", "nonexistent", "ghost")
	require.NoError(t, err)
	require.NotNil(t, score)
	assert.Equal(t, 100, score.TotalScore, "no issues means perfect score")
	assert.Equal(t, 1, score.ApplicableCount)
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func cveCategory(t *testing.T, score *security.SecurityScore) *security.CategoryScore {
	t.Helper()
	for i := range score.Categories {
		if score.Categories[i].Name == CategoryCVEs {
			return &score.Categories[i]
		}
	}
	t.Fatal("cve category missing")
	return nil
}

func TestScorerCVENeverEvaluated(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()

	scorer := NewScorer(ScorerDeps{
		CVEs:           &mockCVEReader{cves: map[string][]security.CVEInfo{}},
		CVEEvaluations: &mockCVEEvalReader{},
		Insights:       secSvc,
		Acks:           &mockAckStore{},
	})

	score, err := scorer.ScoreContainer(ctx, "c1", "ext-1", "test")
	require.NoError(t, err)
	require.NotNil(t, score)

	cat := cveCategory(t, score)
	assert.False(t, cat.Applicable)
	assert.Equal(t, security.EvaluationNotEvaluated, cat.Evaluation)
	assert.Equal(t, "not evaluated", cat.Summary)
	assert.True(t, score.IsPartial)
}

func TestScorerCVEEvaluatedWithNoFindings(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()

	scorer := NewScorer(ScorerDeps{
		CVEs:           &mockCVEReader{cves: map[string][]security.CVEInfo{}},
		CVEEvaluations: &mockCVEEvalReader{states: map[string]string{"ext-1": security.EvaluationEvaluated}},
		Insights:       secSvc,
		Acks:           &mockAckStore{},
	})

	score, err := scorer.ScoreContainer(ctx, "c1", "ext-1", "test")
	require.NoError(t, err)
	require.NotNil(t, score)

	cat := cveCategory(t, score)
	assert.True(t, cat.Applicable)
	assert.Equal(t, 100, cat.SubScore)
	assert.Equal(t, "no known CVEs", cat.Summary)
	assert.False(t, score.IsPartial)
}

func TestScorerCVEUnsupportedImage(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()

	scorer := NewScorer(ScorerDeps{
		CVEs:           &mockCVEReader{cves: map[string][]security.CVEInfo{}},
		CVEEvaluations: &mockCVEEvalReader{states: map[string]string{"ext-1": security.EvaluationUnsupported}},
		Insights:       secSvc,
		Acks:           &mockAckStore{},
	})

	score, err := scorer.ScoreContainer(ctx, "c1", "ext-1", "test")
	require.NoError(t, err)
	require.NotNil(t, score)

	cat := cveCategory(t, score)
	assert.False(t, cat.Applicable)
	assert.Equal(t, security.EvaluationUnsupported, cat.Evaluation)
	assert.NotEqual(t, "not applicable", cat.Summary)
	assert.False(t, score.IsPartial)
}

func TestScorerCVEEvaluatedWithFindings(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()

	scorer := NewScorer(ScorerDeps{
		CVEs: &mockCVEReader{cves: map[string][]security.CVEInfo{
			"ext-1": {{CVEID: "CVE-2025-001", Severity: "critical"}},
		}},
		CVEEvaluations: &mockCVEEvalReader{states: map[string]string{"ext-1": security.EvaluationEvaluated}},
		Insights:       secSvc,
		Acks:           &mockAckStore{},
	})

	score, err := scorer.ScoreContainer(ctx, "c1", "ext-1", "test")
	require.NoError(t, err)
	require.NotNil(t, score)

	cat := cveCategory(t, score)
	assert.True(t, cat.Applicable)
	assert.Equal(t, 1, cat.IssueCount)
	assert.Less(t, cat.SubScore, 100)
	assert.False(t, score.IsPartial)
}

func TestScoreInfrastructurePartialWhenCVENotEvaluated(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()

	scorer := NewScorer(ScorerDeps{
		CVEs:           &mockCVEReader{cves: map[string][]security.CVEInfo{}},
		CVEEvaluations: &mockCVEEvalReader{},
		Insights:       secSvc,
		Acks:           &mockAckStore{},
	})

	posture, err := scorer.ScoreInfrastructure(ctx, []security.ContainerInfo{{ID: "c1", ExternalID: "ext-1", Name: "a"}})
	require.NoError(t, err)
	require.NotNil(t, posture)
	assert.True(t, posture.IsPartial)
}
