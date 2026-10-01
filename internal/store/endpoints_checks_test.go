// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/uid"
)

func TestListCheckResults_CarriesTheProbingAgent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	es := NewEndpointStore(db)
	seedAgent(t, db, "agent-a")

	remoteID, err := es.UpsertEndpoint(ctx, &endpoint.Endpoint{
		AgentID: "agent-a", ContainerName: "web", LabelKey: "maintenant.endpoint.http",
		ExternalID: "ext-1", EndpointType: endpoint.TypeHTTP, Target: "http://web:80",
	})
	require.NoError(t, err)
	localID, err := es.InsertStandaloneEndpoint(ctx, &endpoint.Endpoint{
		Name: "site", Target: "https://example.com", EndpointType: endpoint.TypeHTTP,
	})
	require.NoError(t, err)

	for _, id := range []string{remoteID, localID} {
		_, err := es.InsertCheckResult(ctx, &endpoint.CheckResult{EndpointID: id, Success: true, Timestamp: time.Now()})
		require.NoError(t, err)
	}

	for id, want := range map[string]string{remoteID: "agent-a", localID: uid.LocalAgent} {
		checks, total, err := es.ListCheckResults(ctx, id, endpoint.ListChecksOpts{})
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Len(t, checks, 1)
		assert.Equal(t, want, checks[0].AgentID)
	}
}
