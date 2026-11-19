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
)

// sparklineReference is the window-function form GetSparklineData used before
// it moved to an index-driven join; both must return the same data.
func sparklineReference(t *testing.T, db *DB, limit int) map[string][]float64 {
	t.Helper()
	rows, err := db.Reader().QueryContext(context.Background(), `
		SELECT endpoint_id, response_time_ms
		FROM (
			SELECT endpoint_id, response_time_ms,
				ROW_NUMBER() OVER (PARTITION BY endpoint_id ORDER BY timestamp DESC) AS rn
			FROM check_results
			WHERE endpoint_id IN (SELECT id FROM endpoints WHERE active=1)
				AND response_time_ms IS NOT NULL
		) ranked
		WHERE rn <= ?
		ORDER BY endpoint_id, rn DESC
	`, limit)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := make(map[string][]float64)
	for rows.Next() {
		var id string
		var ms float64
		require.NoError(t, rows.Scan(&id, &ms))
		out[id] = append(out[id], ms)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestEndpointStore_SparklineDataMatchesTheRankedReference(t *testing.T) {
	db := openTestDB(t)
	s := NewEndpointStore(db)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour).Truncate(time.Second)

	newEndpoint := func(name string, results int) string {
		id, err := s.InsertStandaloneEndpoint(ctx, &endpoint.Endpoint{
			Name: name, EndpointType: endpoint.TypeHTTP, Target: "https://example.com/" + name,
		})
		require.NoError(t, err)
		for i := 0; i < results; i++ {
			_, err := s.InsertCheckResult(ctx, &endpoint.CheckResult{
				EndpointID: id, Success: true, ResponseTimeMs: int64(100 + i),
				Timestamp: base.Add(time.Duration(i) * time.Second),
			})
			require.NoError(t, err)
		}
		return id
	}

	many := newEndpoint("many", 35)
	exactly := newEndpoint("exactly", 20)
	few := newEndpoint("few", 7)
	empty := newEndpoint("empty", 0)
	inactive := newEndpoint("inactive", 30)
	_, err := db.Writer().Exec(ctx, `UPDATE endpoints SET active = 0 WHERE id = ?`, inactive)
	require.NoError(t, err)

	for _, limit := range []int{0, 1, 7, 20, 40} {
		eff := limit
		if eff <= 0 {
			eff = 20
		}
		got, err := s.GetSparklineData(ctx, limit)
		require.NoError(t, err)
		assert.Equal(t, sparklineReference(t, db, eff), got, "limit %d", limit)
	}

	got, err := s.GetSparklineData(ctx, 20)
	require.NoError(t, err)
	require.Len(t, got[many], 20)
	assert.Equal(t, 115.0, got[many][0], "oldest of the last 20 first")
	assert.Equal(t, 134.0, got[many][19])
	assert.Len(t, got[exactly], 20)
	assert.Len(t, got[few], 7, "fewer results than the window keeps them all")
	assert.NotContains(t, got, empty)
	assert.NotContains(t, got, inactive)
}
