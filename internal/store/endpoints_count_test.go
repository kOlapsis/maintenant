// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/stretchr/testify/require"
)

// The Community cap applies to what an operator adds by hand, so the count
// behind it ignores label-discovered endpoints, however many a host declares.
func TestCountStandaloneEndpoints_IgnoresLabelEndpoints(t *testing.T) {
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

	count, err := s.CountStandaloneEndpoints(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, count)

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

	count, err = s.CountStandaloneEndpoints(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count)

	require.NoError(t, s.DeactivateEndpoint(ctx, standaloneIDs[0]))
	count, err = s.CountStandaloneEndpoints(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count, "a deactivated endpoint frees its slot")
}
