package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kolapsis/pulseboard/internal/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockResourceTopService is a test double for ResourceTopService.
type mockResourceTopService struct {
	snapshots map[int64]*resource.ResourceSnapshot
	names     map[int64]string
}

func (m *mockResourceTopService) GetAllLatestSnapshots() map[int64]*resource.ResourceSnapshot {
	return m.snapshots
}

func (m *mockResourceTopService) GetContainerName(containerID int64) string {
	if name, ok := m.names[containerID]; ok {
		return name
	}
	return ""
}

func TestHandleGetTopConsumers(t *testing.T) {
	baseSvc := &mockResourceTopService{
		snapshots: map[int64]*resource.ResourceSnapshot{
			1: {ContainerID: 1, CPUPercent: 65.2, MemUsed: 500 * 1024 * 1024, MemLimit: 1024 * 1024 * 1024, Timestamp: time.Now()},
			2: {ContainerID: 2, CPUPercent: 34.1, MemUsed: 200 * 1024 * 1024, MemLimit: 512 * 1024 * 1024, Timestamp: time.Now()},
			3: {ContainerID: 3, CPUPercent: 90.5, MemUsed: 800 * 1024 * 1024, MemLimit: 1024 * 1024 * 1024, Timestamp: time.Now()},
		},
		names: map[int64]string{
			1: "postgres",
			2: "redis",
			3: "app",
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
				snapshots: map[int64]*resource.ResourceSnapshot{},
				names:     map[int64]string{},
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
