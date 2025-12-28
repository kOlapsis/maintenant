package v1

import (
	"testing"
	"time"

	"github.com/kolapsis/pulseboard/internal/heartbeat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrichPings(t *testing.T) {
	baseTime := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)
	interval := 3600 // 1 hour
	grace := 300     // 5 minutes
	createdAt := baseTime.Add(-24 * time.Hour)

	tests := []struct {
		name        string
		pings       []*heartbeat.HeartbeatPing
		intervalSec int
		graceSec    int
		createdAt   time.Time
		checks      func(t *testing.T, result []EnrichedPing)
	}{
		{
			name:        "empty pings",
			pings:       []*heartbeat.HeartbeatPing{},
			intervalSec: interval,
			graceSec:    grace,
			createdAt:   createdAt,
			checks: func(t *testing.T, result []EnrichedPing) {
				assert.Len(t, result, 0)
			},
		},
		{
			name: "single ping has expected_at and grace_deadline",
			pings: []*heartbeat.HeartbeatPing{
				{ID: 1, HeartbeatID: 1, PingType: heartbeat.PingSuccess, Timestamp: baseTime},
			},
			intervalSec: interval,
			graceSec:    grace,
			createdAt:   createdAt,
			checks: func(t *testing.T, result []EnrichedPing) {
				require.Len(t, result, 1)
				require.NotNil(t, result[0].ExpectedAt)
				require.NotNil(t, result[0].GraceDeadline)
				// Grace deadline should be expected_at + 5 minutes
				assert.Equal(t, result[0].ExpectedAt.Add(5*time.Minute), *result[0].GraceDeadline)
			},
		},
		{
			name: "multiple pings have sequential expected_at",
			pings: []*heartbeat.HeartbeatPing{
				// Most recent first
				{ID: 3, HeartbeatID: 1, PingType: heartbeat.PingSuccess, Timestamp: baseTime.Add(2 * time.Hour)},
				{ID: 2, HeartbeatID: 1, PingType: heartbeat.PingSuccess, Timestamp: baseTime.Add(1 * time.Hour)},
				{ID: 1, HeartbeatID: 1, PingType: heartbeat.PingSuccess, Timestamp: baseTime},
			},
			intervalSec: interval,
			graceSec:    grace,
			createdAt:   createdAt,
			checks: func(t *testing.T, result []EnrichedPing) {
				require.Len(t, result, 3)
				for _, p := range result {
					require.NotNil(t, p.ExpectedAt, "ping %d should have expected_at", p.ID)
					require.NotNil(t, p.GraceDeadline, "ping %d should have grace_deadline", p.ID)
				}
				// Second ping's expected_at should be oldest ping timestamp + interval
				assert.Equal(t, baseTime.Add(1*time.Hour), *result[1].ExpectedAt)
				// Third (most recent) ping's expected_at should be second ping timestamp + interval
				assert.Equal(t, baseTime.Add(1*time.Hour).Add(1*time.Hour), *result[0].ExpectedAt)
			},
		},
		{
			name: "grace_deadline accounts for grace period",
			pings: []*heartbeat.HeartbeatPing{
				{ID: 1, HeartbeatID: 1, PingType: heartbeat.PingSuccess, Timestamp: baseTime},
			},
			intervalSec: 600, // 10 minutes
			graceSec:    120, // 2 minutes
			createdAt:   createdAt,
			checks: func(t *testing.T, result []EnrichedPing) {
				require.Len(t, result, 1)
				expectedDiff := result[0].GraceDeadline.Sub(*result[0].ExpectedAt)
				assert.Equal(t, 2*time.Minute, expectedDiff)
			},
		},
		{
			name: "preserves original ping fields",
			pings: []*heartbeat.HeartbeatPing{
				{
					ID: 42, HeartbeatID: 3, PingType: heartbeat.PingSuccess,
					SourceIP: "192.168.1.1", HTTPMethod: "POST",
					Timestamp: baseTime,
				},
			},
			intervalSec: interval,
			graceSec:    grace,
			createdAt:   createdAt,
			checks: func(t *testing.T, result []EnrichedPing) {
				require.Len(t, result, 1)
				assert.Equal(t, int64(42), result[0].ID)
				assert.Equal(t, int64(3), result[0].HeartbeatID)
				assert.Equal(t, heartbeat.PingSuccess, result[0].PingType)
				assert.Equal(t, "192.168.1.1", result[0].SourceIP)
				assert.Equal(t, "POST", result[0].HTTPMethod)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enrichPings(tt.pings, tt.intervalSec, tt.graceSec, tt.createdAt)
			tt.checks(t, result)
		})
	}
}
