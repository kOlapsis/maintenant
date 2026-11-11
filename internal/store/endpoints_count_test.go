// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/require"
)

func TestInsertCheckResult_SameEventTwiceKeepsOneRow(t *testing.T) {
	db := openTestDB(t)
	s := NewEndpointStore(db)
	ctx := context.Background()

	epID, err := s.InsertStandaloneEndpoint(ctx, &endpoint.Endpoint{
		Name: "replayed", EndpointType: endpoint.TypeHTTP, Target: "https://example.com/replay",
	})
	require.NoError(t, err)

	id := uid.EventRecord(uid.LocalAgent, "evt-replayed", "check_result")
	for range 2 {
		_, err := s.InsertCheckResult(ctx, &endpoint.CheckResult{
			ID: id, EndpointID: epID, Success: true, ResponseTimeMs: 12, Timestamp: time.Now(),
		})
		require.NoError(t, err)
	}

	var count int
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM check_results WHERE endpoint_id = ?`, epID).Scan(&count))
	require.Equal(t, 1, count)
}

func TestCountActiveEndpoints_FiltersBySource(t *testing.T) {
	db := openTestDB(t)
	s := NewEndpointStore(db)
	ctx := context.Background()

	for i := 1; i <= 12; i++ {
		_, err := s.UpsertEndpoint(ctx, &endpoint.Endpoint{
			ContainerName: "web",
			LabelKey:      fmt.Sprintf("endpoint.%d.http", i),
			ExternalID:    "container-1",
			EndpointType:  endpoint.TypeHTTP,
			Target:        fmt.Sprintf("http://svc:80%d/health", i),
		})
		require.NoError(t, err)
	}

	count, err := s.CountActiveEndpoints(ctx, string(endpoint.SourceStandalone))
	require.NoError(t, err)
	require.Equal(t, 0, count)
	count, err = s.CountActiveEndpoints(ctx, "")
	require.NoError(t, err)
	require.Equal(t, 12, count)

	var standaloneIDs []string
	for i := 1; i <= 3; i++ {
		id, err := s.InsertStandaloneEndpoint(ctx, &endpoint.Endpoint{
			Name:         fmt.Sprintf("manual %d", i),
			EndpointType: endpoint.TypeHTTP,
			Target:       fmt.Sprintf("https://example.com/%d", i),
		})
		require.NoError(t, err)
		standaloneIDs = append(standaloneIDs, id)
	}

	count, err = s.CountActiveEndpoints(ctx, string(endpoint.SourceStandalone))
	require.NoError(t, err)
	require.Equal(t, 3, count)

	require.NoError(t, s.DeactivateEndpoint(ctx, standaloneIDs[0]))
	count, err = s.CountActiveEndpoints(ctx, string(endpoint.SourceStandalone))
	require.NoError(t, err)
	require.Equal(t, 2, count, "a deactivated endpoint frees its slot")
}

func TestEndpointStore_LimitedEndpointLosesItsStatus(t *testing.T) {
	db := openTestDB(t)
	s := NewEndpointStore(db)
	ctx := context.Background()

	id := clLabelEndpoint(t, s, uid.LocalAgent, "web", "maintenant.endpoint.tcp", "127.0.0.1:7000")
	require.NoError(t, s.UpdateCheckResult(ctx, id, endpoint.StatusUp, endpoint.AlertNormal, 0, 3, 5, nil, ""))

	require.NoError(t, s.SetEndpointLimited(ctx, id, true))
	ep, err := s.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	require.True(t, ep.Limited)
	require.Equal(t, endpoint.StatusUnknown, ep.Status)

	require.NoError(t, s.SetEndpointLimited(ctx, id, false))
	ep, err = s.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	require.False(t, ep.Limited)
	require.Equal(t, endpoint.StatusUnknown, ep.Status, "the status waits for the next check")
}

func TestEndpointStore_RecreatedEndpointKeepsItsRight(t *testing.T) {
	db := openTestDB(t)
	s := NewEndpointStore(db)
	ctx := context.Background()

	id := clLabelEndpoint(t, s, uid.LocalAgent, "web", "maintenant.endpoint.tcp", "127.0.0.1:7000")
	_, err := db.Writer().Exec(ctx, `UPDATE endpoints SET grandfathered=1, first_seen_at=?, last_seen_at=? WHERE id=?`,
		int64(1000), int64(1000), id)
	require.NoError(t, err)

	require.NoError(t, s.DeactivateEndpoint(ctx, id))
	ep, err := s.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	require.False(t, ep.Active)
	require.WithinDuration(t, time.Now(), ep.LastSeenAt, time.Minute, "deactivation records when the endpoint vanished")

	again := clLabelEndpoint(t, s, uid.LocalAgent, "web", "maintenant.endpoint.tcp", "127.0.0.1:7001")
	require.Equal(t, id, again)
	ep, err = s.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	require.True(t, ep.Active)
	require.True(t, ep.Grandfathered)
	require.Equal(t, int64(1000), ep.FirstSeenAt.Unix())
}
