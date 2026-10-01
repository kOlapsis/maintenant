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

type countingUpdateStore struct {
	update.UpdateStore
	lists   int
	updates []*update.ImageUpdate
}

func (s *countingUpdateStore) ListImageUpdates(context.Context, update.ListImageUpdatesOpts) ([]*update.ImageUpdate, error) {
	s.lists++
	return s.updates, nil
}

type countingCertStore struct {
	certificate.CertificateStore
	lists    int
	monitors []*certificate.CertMonitor
}

func (s *countingCertStore) ListMonitors(context.Context, certificate.ListCertificatesOpts) ([]*certificate.CertMonitor, error) {
	s.lists++
	return s.monitors, nil
}

func (s *countingCertStore) GetLatestCheckResult(context.Context, string) (*certificate.CertCheckResult, error) {
	return nil, nil
}

type noAcknowledgments struct{ security.AcknowledgmentStore }

// Scoring the whole fleet reads the certificates and the updates once, not once per container.
func TestPostureAdapters_ReadOncePerPass(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	updates := &countingUpdateStore{updates: []*update.ImageUpdate{
		{ContainerID: "ext-1", UpdateType: update.UpdateTypeMajor},
	}}
	certs := &countingCertStore{monitors: []*certificate.CertMonitor{
		{ID: "m1", ExternalID: "ext-2", Status: certificate.StatusExpired},
	}}
	scorer := posture.NewScorer(posture.ScorerDeps{
		Certs:   &CertPostureAdapter{CertSvc: certificate.NewService(certificate.Deps{Store: certs, Logger: logger})},
		Updates: &UpdatePostureAdapter{Store: updates},
		Acks:    noAcknowledgments{},
	})

	containers := []security.ContainerInfo{
		{ID: "c1", ExternalID: "ext-1", Name: "a"},
		{ID: "c2", ExternalID: "ext-2", Name: "b"},
		{ID: "c3", ExternalID: "ext-3", Name: "c"},
	}
	infra, err := scorer.ScoreInfrastructure(context.Background(), containers)
	require.NoError(t, err)

	assert.Equal(t, 3, infra.ScoredCount)
	assert.Equal(t, 1, updates.lists)
	assert.Equal(t, 1, certs.lists)

	byName := map[string]*security.SecurityScore{}
	for _, c := range containers {
		s, err := scorer.ScoreContainer(context.Background(), c.ID, c.ExternalID, c.Name)
		require.NoError(t, err)
		byName[c.Name] = s
	}
	assert.Less(t, byName["a"].TotalScore, byName["c"].TotalScore, "the pending major update weighs on its container only")
	assert.Less(t, byName["b"].TotalScore, byName["c"].TotalScore, "the expired certificate weighs on its container only")
}
