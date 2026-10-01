// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/security"
)

type recordingScorer struct {
	security.PostureScorer
	scored [][]security.ContainerInfo
}

func (s *recordingScorer) ScoreInfrastructure(_ context.Context, infos []security.ContainerInfo) (*security.InfrastructurePosture, error) {
	s.scored = append(s.scored, infos)
	return &security.InfrastructurePosture{}, nil
}

type listedContainers []*container.Container

func (l listedContainers) ListContainers(context.Context, container.ListContainersOpts) ([]*container.Container, error) {
	return l, nil
}

func pinEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = prev })
}

func TestCheckPosture_ScoresEveryContainer(t *testing.T) {
	pinEdition(t, extension.Pro)
	scorer := &recordingScorer{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	checkPosture(context.Background(), scorer, listedContainers{
		{ID: "c1", ExternalID: "ext-1", Name: "web"},
		{ID: "c2", ExternalID: "ext-2", Name: "db"},
	}, logger)

	assert.Equal(t, [][]security.ContainerInfo{{
		{ID: "c1", ExternalID: "ext-1", Name: "web"},
		{ID: "c2", ExternalID: "ext-2", Name: "db"},
	}}, scorer.scored)
}

func TestCheckPosture_WaitsForAnEditionWithThePosture(t *testing.T) {
	pinEdition(t, extension.Community)
	scorer := &recordingScorer{}

	checkPosture(context.Background(), scorer, listedContainers{{ID: "c1"}}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	assert.Empty(t, scorer.scored)
}
