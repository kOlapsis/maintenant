// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package posture

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/security"
)

type thresholdCall struct {
	score  int
	breach bool
}

func TestScoreInfrastructure_EvaluatesTheThreshold(t *testing.T) {
	ctx := context.Background()
	secSvc := newTestService()
	secSvc.UpdateContainer("c1", "web", []security.Insight{
		{Type: security.PrivilegedContainer, Severity: security.SeverityCritical, ContainerID: "c1"},
	})
	var calls []thresholdCall
	scorer := NewScorer(ScorerDeps{
		Insights:  secSvc,
		Acks:      &mockAckStore{},
		Threshold: 80,
		PostureAlertCallback: func(score, _ int, _ string, breach bool) {
			calls = append(calls, thresholdCall{score, breach})
		},
	})
	containers := []security.ContainerInfo{{ID: "c1", ExternalID: "ext-1", Name: "web"}}

	posture, err := scorer.ScoreInfrastructure(ctx, containers)
	require.NoError(t, err)
	low := posture.Score
	require.Less(t, low, 80)
	assert.Equal(t, []thresholdCall{{low, true}}, calls, "reading the posture is enough to raise the alert")

	_, err = scorer.ScoreInfrastructure(ctx, containers)
	require.NoError(t, err)
	assert.Len(t, calls, 1, "a score still below the threshold does not alert twice")

	secSvc.UpdateContainer("c1", "web", nil)
	scorer.InvalidateCache("c1")
	posture, err = scorer.ScoreInfrastructure(ctx, containers)
	require.NoError(t, err)
	assert.Equal(t, []thresholdCall{{low, true}, {posture.Score, false}}, calls)
}

func TestScoreInfrastructure_NothingScoredRaisesNothing(t *testing.T) {
	called := false
	scorer := NewScorer(ScorerDeps{
		Acks:                 &mockAckStore{},
		Threshold:            80,
		PostureAlertCallback: func(int, int, string, bool) { called = true },
	})

	posture, err := scorer.ScoreInfrastructure(context.Background(), []security.ContainerInfo{{ID: "c1", ExternalID: "ext-1", Name: "web"}})
	require.NoError(t, err)
	assert.Zero(t, posture.ScoredCount)
	assert.False(t, called, "no data is not a score of zero")
}
