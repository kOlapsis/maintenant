// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type stagingPinStore struct {
	*insertRecordingStore
}

func (s *stagingPinStore) AdoptVersionPin(_ context.Context, agentID, _, _, _ string) (*VersionPin, error) {
	if agentID == "agent-staging" {
		return &VersionPin{AgentID: agentID, PinnedTag: "1.0.0"}, nil
	}
	return nil, nil
}

func homonymScan(store UpdateStore) {
	svc := NewService(Deps{
		Store:   store,
		Scanner: newTestScanner(&stubRegistry{tags: map[string][]string{"acme/api": {"1.0.0", "1.1.0"}}}, store),
		Containers: stubLister{containers: []ContainerInfo{
			{UID: "uid-api-staging", AgentID: "agent-staging", ExternalID: "shop/Deployment/api", Name: "api", Image: "acme/api:1.0.0"},
			{UID: "uid-api-prod", AgentID: "agent-prod", ExternalID: "shop/Deployment/api", Name: "api", Image: "acme/api:1.0.0"},
		}},
		Logger: testLogger(),
	})
	svc.runScan(context.Background())
}

// Two clusters running a workload under the same namespace/kind/name each get their own update.
func TestRunScan_HomonymsKeepTheirOwnAgent(t *testing.T) {
	store := &insertRecordingStore{stubStore: &stubStore{}}
	homonymScan(store)

	var agents []string
	for _, u := range store.inserted {
		agents = append(agents, u.AgentID)
	}
	assert.ElementsMatch(t, []string{"agent-staging", "agent-prod"}, agents)
}

// A pin on the staging workload does not hold back its prod homonym.
func TestRunScan_PinOfOneHomonymLeavesTheOther(t *testing.T) {
	store := &stagingPinStore{&insertRecordingStore{stubStore: &stubStore{}}}
	homonymScan(store)

	var agents []string
	for _, u := range store.inserted {
		agents = append(agents, u.AgentID)
	}
	assert.Equal(t, []string{"agent-prod"}, agents)
}
