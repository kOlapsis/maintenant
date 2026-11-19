// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/commercial/posture"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/update"
)

// The pending update and the expired certificate of the prod workload weigh on prod only, not on its staging homonym.
func TestPostureAdapters_HomonymsScoreApart(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const workload = "shop/Deployment/api"
	updates := &countingUpdateStore{updates: []*update.ImageUpdate{
		{ContainerID: workload, AgentID: "agent-prod", UpdateType: update.UpdateTypeMajor},
	}}
	certs := &countingCertStore{monitors: []*certificate.CertMonitor{
		{ID: "m1", AgentID: "agent-prod", ExternalID: workload, Status: certificate.StatusExpired},
	}}
	scorer := posture.NewScorer(posture.ScorerDeps{
		Certs:   &CertPostureAdapter{CertSvc: certificate.NewService(certificate.Deps{Store: certs, Logger: logger})},
		Updates: &UpdatePostureAdapter{Store: updates},
		Acks:    noAcknowledgments{},
	})

	staging := security.ContainerInfo{ID: "c-staging", AgentID: "agent-staging", ExternalID: workload, Name: "api"}
	prod := security.ContainerInfo{ID: "c-prod", AgentID: "agent-prod", ExternalID: workload, Name: "api"}
	_, err := scorer.ScoreInfrastructure(context.Background(), []security.ContainerInfo{staging, prod})
	require.NoError(t, err)

	category := func(c security.ContainerInfo, name string) security.CategoryScore {
		t.Helper()
		s, err := scorer.ScoreContainer(context.Background(), c)
		require.NoError(t, err)
		for _, cat := range s.Categories {
			if cat.Name == name {
				return cat
			}
		}
		t.Fatalf("no %s category", name)
		return security.CategoryScore{}
	}
	assert.Equal(t, 100, category(staging, posture.CategoryUpdates).SubScore, "staging has no pending update")
	assert.Less(t, category(prod, posture.CategoryUpdates).SubScore, 100, "prod waits for a major update")
	assert.False(t, category(staging, posture.CategoryTLS).Applicable, "staging has no certificate")
	assert.True(t, category(prod, posture.CategoryTLS).Applicable, "prod's expired certificate counts")
}
