// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/uid"
)

func TestRunScan_FailedContainerDoesNotKeepItsHomonymOnAnotherHost(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"registry refuses the image", errors.New("GET https://ghcr.io/v2/acme/db/tags/list: UNAUTHORIZED: authentication required")},
		{"scan fails", errors.New("dial tcp 10.0.0.9:443: connect: connection refused")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			failedOnA := StaleImageUpdate{ContainerID: "ctr-db-a", ContainerName: "db", AgentID: "agent-a"}
			upgradedOnB := StaleImageUpdate{ContainerID: "ctr-db-b", ContainerName: "db", AgentID: "agent-b"}
			store := &hostPendingStore{stubStore: &stubStore{}, pending: []StaleImageUpdate{failedOnA, upgradedOnB}}
			reg := &refusingRegistry{
				stubRegistry: &stubRegistry{tags: map[string][]string{"library/postgres": {"16.4"}}},
				tagsErr:      map[string]error{"ghcr.io/acme/db": tc.err},
			}

			var resolved []map[string]interface{}
			svc := NewService(Deps{
				Store:   store,
				Scanner: newTestScanner(reg, store),
				Containers: stubLister{containers: []ContainerInfo{
					{UID: uid.Container("agent-a", "ctr-db-a"), AgentID: "agent-a", ExternalID: "ctr-db-a", Name: "db", Image: "ghcr.io/acme/db:16.1"},
					{UID: uid.Container("agent-b", "ctr-db-b"), AgentID: "agent-b", ExternalID: "ctr-db-b", Name: "db", Image: "postgres:16.4"},
				}},
				Logger:        testLogger(),
				EventCallback: resolvedAgents(t, &resolved),
			})

			svc.runScan(context.Background())

			assert.Equal(t, []StaleImageUpdate{failedOnA}, store.pending)
			require.Len(t, resolved, 1)
			assert.Equal(t, "ctr-db-b", resolved[0]["container_id"])
			assert.Equal(t, "agent-b", resolved[0]["agent_id"])
		})
	}
}
