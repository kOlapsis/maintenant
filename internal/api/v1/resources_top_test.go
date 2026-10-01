// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockResourceTopService is a test double for ResourceTopService.
type mockResourceTopService struct {
	snapshots map[string]*resource.ResourceSnapshot
	names     map[string]string
}

func (m *mockResourceTopService) GetAllLatestSnapshots() map[string]*resource.ResourceSnapshot {
	return m.snapshots
}

// TopConsumersNow mirrors what the real service does with the latest samples,
// so these tests keep exercising the ranking the endpoint reports.
func (m *mockResourceTopService) TopConsumersNow(metric string, limit int, agentID *string) []resource.TopConsumerRow {
	rows := make([]resource.TopConsumerRow, 0, len(m.snapshots))
	for id, snap := range m.snapshots {
		if agentID != nil && !onHost(snap.AgentID, *agentID) {
			continue
		}
		var value, percent float64
		switch metric {
		case "cpu":
			value, percent = snap.CPUPercent, snap.CPUPercent
		case "memory":
			value = float64(snap.MemUsed)
			if snap.MemLimit > 0 {
				percent = float64(snap.MemUsed) / float64(snap.MemLimit) * 100.0
			}
		}
		rows = append(rows, resource.TopConsumerRow{
			ContainerID: id, ContainerName: m.GetContainerName(id),
			AvgValue: value, AvgPercent: percent,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].AvgValue > rows[j].AvgValue })
	if limit < len(rows) {
		rows = rows[:limit]
	}
	return rows
}

// onHost tells whether a sample belongs to host, "" being the local server.
func onHost(snapAgent, host string) bool {
	if host == "" {
		return snapAgent == "" || snapAgent == uid.LocalAgent
	}
	return snapAgent == host
}

func (m *mockResourceTopService) GetContainerName(containerID string) string {
	if name, ok := m.names[containerID]; ok {
		return name
	}
	return ""
}

func (m *mockResourceTopService) GetTopConsumersByPeriod(_ context.Context, _ string, _ string, _ int, _ *string) ([]resource.TopConsumerRow, error) {
	return nil, nil
}

func TestHandleGetTopConsumers(t *testing.T) {
	baseSvc := &mockResourceTopService{
		snapshots: map[string]*resource.ResourceSnapshot{
			"1": {ContainerID: "1", CPUPercent: 65.2, MemUsed: 500 * 1024 * 1024, MemLimit: 1024 * 1024 * 1024, Timestamp: time.Now()},
			"2": {ContainerID: "2", CPUPercent: 34.1, MemUsed: 200 * 1024 * 1024, MemLimit: 512 * 1024 * 1024, Timestamp: time.Now()},
			"3": {ContainerID: "3", CPUPercent: 90.5, MemUsed: 800 * 1024 * 1024, MemLimit: 1024 * 1024 * 1024, Timestamp: time.Now()},
		},
		names: map[string]string{
			"1": "postgres",
			"2": "redis",
			"3": "app",
		},
	}

	tests := []struct {
		name       string
		url        string
		svc        *mockResourceTopService
		wantStatus int
		checkBody  func(t *testing.T, body map[string]interface{})
	}{
		{
			name:       "top by cpu",
			url:        "/api/v1/resources/top?metric=cpu",
			svc:        baseSvc,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				assert.Equal(t, "cpu", body["metric"])
				consumers := body["consumers"].([]interface{})
				assert.Len(t, consumers, 3)
				// First should be highest CPU (app=90.5)
				first := consumers[0].(map[string]interface{})
				assert.Equal(t, "app", first["container_name"])
				assert.Equal(t, float64(1), first["rank"])
				assert.Equal(t, 90.5, first["value"])
			},
		},
		{
			name:       "top by memory",
			url:        "/api/v1/resources/top?metric=memory",
			svc:        baseSvc,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				assert.Equal(t, "memory", body["metric"])
				consumers := body["consumers"].([]interface{})
				assert.Len(t, consumers, 3)
				// First should be highest memory percent (app=78.1% vs postgres=48.8% vs redis=39.1%)
				first := consumers[0].(map[string]interface{})
				assert.Equal(t, "app", first["container_name"])
			},
		},
		{
			name:       "custom limit",
			url:        "/api/v1/resources/top?metric=cpu&limit=2",
			svc:        baseSvc,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				consumers := body["consumers"].([]interface{})
				assert.Len(t, consumers, 2)
			},
		},
		{
			name:       "limit exceeds count",
			url:        "/api/v1/resources/top?metric=cpu&limit=10",
			svc:        baseSvc,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				consumers := body["consumers"].([]interface{})
				assert.Len(t, consumers, 3) // only 3 containers
			},
		},
		{
			name:       "invalid metric",
			url:        "/api/v1/resources/top?metric=disk",
			svc:        baseSvc,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing metric",
			url:        "/api/v1/resources/top",
			svc:        baseSvc,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "empty snapshots",
			url:  "/api/v1/resources/top?metric=cpu",
			svc: &mockResourceTopService{
				snapshots: map[string]*resource.ResourceSnapshot{},
				names:     map[string]string{},
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				consumers := body["consumers"].([]interface{})
				assert.Len(t, consumers, 0)
			},
		},
		{
			name:       "rank is sequential",
			url:        "/api/v1/resources/top?metric=cpu",
			svc:        baseSvc,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				consumers := body["consumers"].([]interface{})
				for i, c := range consumers {
					consumer := c.(map[string]interface{})
					assert.Equal(t, float64(i+1), consumer["rank"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewResourceTopHandler(tt.svc)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/resources/top", handler.HandleGetTopConsumers)

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.checkBody != nil {
				var body map[string]interface{}
				err := json.NewDecoder(w.Body).Decode(&body)
				require.NoError(t, err)
				tt.checkBody(t, body)
			}
		})
	}
}
