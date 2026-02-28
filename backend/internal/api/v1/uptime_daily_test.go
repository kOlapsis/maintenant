package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kolapsis/pulseboard/internal/store/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUptimeDailyStore is a test double for UptimeDailyFetcher.
type mockUptimeDailyStore struct {
	endpointResults  map[int64][]sqlite.DailyUptime
	heartbeatResults map[int64][]sqlite.DailyUptime
	err              error
}

func (m *mockUptimeDailyStore) GetEndpointDailyUptime(_ context.Context, endpointID int64, days int) ([]sqlite.DailyUptime, error) {
	if m.err != nil {
		return nil, m.err
	}
	if results, ok := m.endpointResults[endpointID]; ok {
		if days < len(results) {
			return results[:days], nil
		}
		return results, nil
	}
	// Return null days for missing endpoints.
	result := make([]sqlite.DailyUptime, days)
	for i := range result {
		result[i] = sqlite.DailyUptime{Date: fmt.Sprintf("2026-01-%02d", days-i), UptimePercent: nil}
	}
	return result, nil
}

func (m *mockUptimeDailyStore) GetHeartbeatDailyUptime(_ context.Context, heartbeatID int64, days int) ([]sqlite.DailyUptime, error) {
	if m.err != nil {
		return nil, m.err
	}
	if results, ok := m.heartbeatResults[heartbeatID]; ok {
		if days < len(results) {
			return results[:days], nil
		}
		return results, nil
	}
	result := make([]sqlite.DailyUptime, days)
	for i := range result {
		result[i] = sqlite.DailyUptime{Date: fmt.Sprintf("2026-01-%02d", days-i), UptimePercent: nil}
	}
	return result, nil
}

func ptrFloat(f float64) *float64 { return &f }

func TestHandleEndpointDailyUptime(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		store      *mockUptimeDailyStore
		wantStatus int
		checkBody  func(t *testing.T, body map[string]interface{})
	}{
		{
			name: "valid endpoint with data",
			url:  "/api/v1/endpoints/5/uptime/daily",
			store: &mockUptimeDailyStore{
				endpointResults: map[int64][]sqlite.DailyUptime{
					5: {
						{Date: "2026-02-25", UptimePercent: ptrFloat(100.0), IncidentCount: 0},
						{Date: "2026-02-24", UptimePercent: ptrFloat(95.5), IncidentCount: 1},
					},
				},
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				assert.Equal(t, float64(5), body["monitor_id"])
				assert.Equal(t, "endpoint", body["monitor_type"])
				days := body["days"].([]interface{})
				assert.Len(t, days, 2)
				firstDay := days[0].(map[string]interface{})
				assert.Equal(t, "2026-02-25", firstDay["date"])
				assert.Equal(t, 100.0, firstDay["uptime_percent"])
				assert.Equal(t, float64(0), firstDay["incident_count"])
			},
		},
		{
			name:       "invalid endpoint ID",
			url:        "/api/v1/endpoints/abc/uptime/daily",
			store:      &mockUptimeDailyStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "custom days param",
			url:        "/api/v1/endpoints/1/uptime/daily?days=30",
			store:      &mockUptimeDailyStore{},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				days := body["days"].([]interface{})
				assert.Len(t, days, 30)
			},
		},
		{
			name:       "default 90 days",
			url:        "/api/v1/endpoints/1/uptime/daily",
			store:      &mockUptimeDailyStore{},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				days := body["days"].([]interface{})
				assert.Len(t, days, 90)
			},
		},
		{
			name:       "store error returns 500",
			url:        "/api/v1/endpoints/1/uptime/daily",
			store:      &mockUptimeDailyStore{err: fmt.Errorf("db error")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewUptimeDailyHandler(tt.store)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/endpoints/{id}/uptime/daily", handler.HandleEndpointDailyUptime)

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

func TestHandleHeartbeatDailyUptime(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		store      *mockUptimeDailyStore
		wantStatus int
		checkBody  func(t *testing.T, body map[string]interface{})
	}{
		{
			name: "valid heartbeat with data",
			url:  "/api/v1/heartbeats/3/uptime/daily",
			store: &mockUptimeDailyStore{
				heartbeatResults: map[int64][]sqlite.DailyUptime{
					3: {
						{Date: "2026-02-25", UptimePercent: ptrFloat(100.0), IncidentCount: 0},
					},
				},
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				assert.Equal(t, float64(3), body["monitor_id"])
				assert.Equal(t, "heartbeat", body["monitor_type"])
				days := body["days"].([]interface{})
				assert.Len(t, days, 1)
			},
		},
		{
			name:       "invalid heartbeat ID",
			url:        "/api/v1/heartbeats/xyz/uptime/daily",
			store:      &mockUptimeDailyStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "default 90 days for heartbeat",
			url:        "/api/v1/heartbeats/1/uptime/daily",
			store:      &mockUptimeDailyStore{},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				days := body["days"].([]interface{})
				assert.Len(t, days, 90)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewUptimeDailyHandler(tt.store)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/heartbeats/{id}/uptime/daily", handler.HandleHeartbeatDailyUptime)

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
