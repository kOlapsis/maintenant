// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
)

type failingInsertStore struct {
	*stubStore
	failFor string
}

func (s *failingInsertStore) InsertImageUpdate(_ context.Context, u *ImageUpdate) (string, error) {
	if u.ContainerName == s.failFor {
		return "", errors.New("insert failed")
	}
	return "", nil
}

type perContainerScoreEnricher struct {
	scores map[string]int
}

func (e *perContainerScoreEnricher) Enrich(_ context.Context, results []UpdateResult) error {
	for i := range results {
		if score, ok := e.scores[results[i].ContainerName]; ok {
			results[i].RiskScore = score
		}
	}
	return nil
}

func TestRunScan_EachDetectionCarriesItsOwnContainer(t *testing.T) {
	store := &failingInsertStore{stubStore: &stubStore{}, failFor: "db"}
	reg := &stubRegistry{tags: map[string][]string{
		"library/postgres": {"16.1", "16.4"},
		"library/redis":    {"7.2.4"},
		"library/nginx":    {"1.24.0", "1.26.0"},
		"library/mariadb":  {"10.11.2", "10.11.9"},
	}}
	enricher := &perContainerScoreEnricher{scores: map[string]int{"db": 71, "cache": 72, "web": 73, "sql": 74}}

	detected := map[string]map[string]interface{}{}
	svc := NewService(Deps{
		Store:   store,
		Scanner: newTestScanner(reg, store),
		Containers: stubLister{containers: []ContainerInfo{
			{UID: "uid-db", ExternalID: "ctr-db", Name: "db", Image: "postgres:16.1"},
			{UID: "uid-cache", ExternalID: "ctr-cache", Name: "cache", Image: "redis:7.2.4"},
			{UID: "uid-web", ExternalID: "ctr-web", Name: "web", Image: "nginx:1.24.0"},
			{UID: "uid-sql", ExternalID: "ctr-sql", Name: "sql", Image: "mariadb:10.11.2"},
		}},
		Logger:   testLogger(),
		Enricher: enricher,
		EventCallback: func(eventType string, data interface{}) {
			if eventType != event.UpdateDetected {
				return
			}
			m := data.(map[string]interface{})
			name := m["container_name"].(string)
			require.NotContains(t, detected, name, "one event per update")
			detected[name] = m
		},
	})

	svc.runScan(context.Background())

	require.Len(t, detected, 2)
	assert.Equal(t, "1.26.0", detected["web"]["latest_tag"])
	assert.Equal(t, 73, detected["web"]["risk_score"])
	assert.Equal(t, "ctr-web", detected["web"]["container_id"])
	assert.Equal(t, "10.11.9", detected["sql"]["latest_tag"])
	assert.Equal(t, 74, detected["sql"]["risk_score"])
	assert.Equal(t, "ctr-sql", detected["sql"]["container_id"])
}
