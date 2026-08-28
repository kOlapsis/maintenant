package v1

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kolapsis/maintenant/internal/container"
	pbruntime "github.com/kolapsis/maintenant/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRuntime is a minimal runtime stub satisfying pbruntime.Runtime.
type mockRuntime struct {
	name      string
	connected bool
}

func (m *mockRuntime) Name() string                     { return m.name }
func (m *mockRuntime) IsConnected() bool                { return m.connected }
func (m *mockRuntime) Connect(context.Context) error    { return nil }
func (m *mockRuntime) TryConnect(context.Context) error { return nil }
func (m *mockRuntime) SetDisconnected()                 {}
func (m *mockRuntime) Close() error                     { return nil }
func (m *mockRuntime) DiscoverAll(context.Context) ([]*container.Container, error) {
	return nil, nil
}
func (m *mockRuntime) StreamEvents(context.Context) <-chan pbruntime.RuntimeEvent {
	return make(chan pbruntime.RuntimeEvent)
}
func (m *mockRuntime) StatsSnapshot(context.Context, string) (*pbruntime.RawStats, error) {
	return nil, nil
}
func (m *mockRuntime) FetchLogs(context.Context, string, int, bool) ([]string, error) {
	return nil, nil
}
func (m *mockRuntime) StreamLogs(context.Context, string, int, bool) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockRuntime) GetHealthInfo(context.Context, string) (*pbruntime.HealthInfo, error) {
	return nil, nil
}

func newTestRouter(rt pbruntime.Runtime) *Router {
	return &Router{
		mux:     http.NewServeMux(),
		runtime: rt,
	}
}

func TestHandleHealth_RuntimeField(t *testing.T) {
	tests := []struct {
		name          string
		rt            pbruntime.Runtime
		wantConnected bool
		wantName      string
	}{
		{
			name:          "connected docker runtime",
			rt:            &mockRuntime{name: "docker", connected: true},
			wantConnected: true,
			wantName:      "docker",
		},
		{
			name:          "degraded docker runtime",
			rt:            &mockRuntime{name: "docker", connected: false},
			wantConnected: false,
			wantName:      "docker",
		},
		{
			name:          "degraded kubernetes runtime",
			rt:            &mockRuntime{name: "kubernetes", connected: false},
			wantConnected: false,
			wantName:      "kubernetes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(tt.rt)
			r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
			w := httptest.NewRecorder()
			r.mux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var body map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

			assert.Equal(t, "ok", body["status"])

			rt, ok := body["runtime"].(map[string]interface{})
			require.True(t, ok, "response must contain 'runtime' object")
			assert.Equal(t, tt.wantName, rt["name"])
			assert.Equal(t, tt.wantConnected, rt["connected"])
		})
	}
}

func TestHandleHealth_NoRuntime(t *testing.T) {
	r := &Router{mux: http.NewServeMux()}
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	r.mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ok", body["status"])
	assert.Nil(t, body["runtime"], "runtime field must be absent when no runtime set")
}

// fakeStorage is a pilotable StorageStatus for the health tests.
type fakeStorage struct {
	engine    string
	connected bool
	peers     int
}

func (f fakeStorage) Engine() string  { return f.engine }
func (f fakeStorage) Connected() bool { return f.connected }
func (f fakeStorage) Peers() int      { return f.peers }

func healthBody(t *testing.T, r *Router) (int, map[string]interface{}) {
	t.Helper()
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	r.mux.ServeHTTP(w, req)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w.Code, body
}

func TestHandleHealth_StorageField(t *testing.T) {
	tests := []struct {
		name    string
		storage fakeStorage
	}{
		{"sqlite, alone", fakeStorage{engine: "sqlite", connected: true, peers: 0}},
		{"postgres, alone", fakeStorage{engine: "postgres", connected: true, peers: 0}},
		{"postgres, peered", fakeStorage{engine: "postgres", connected: true, peers: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Router{mux: http.NewServeMux(), storage: tt.storage}
			code, body := healthBody(t, r)

			assert.Equal(t, http.StatusOK, code)
			st, ok := body["storage"].(map[string]interface{})
			require.True(t, ok, "response must carry a 'storage' object")
			assert.Equal(t, tt.storage.engine, st["engine"])
			assert.Equal(t, tt.storage.connected, st["connected"])
			assert.Equal(t, float64(tt.storage.peers), st["peers"])

			// FR-021 / SC-007: no connection string, no host, no credential.
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "postgres://")
			assert.NotContains(t, string(raw), "password")
			assert.NotContains(t, string(raw), "host")
		})
	}
}

// TestHandleHealth_StorageOffline pins the invariant that makes this endpoint
// safe as a Kubernetes liveness target: an unreachable database is reported in
// storage.connected, never in the HTTP status. A probe failing on a ten-second
// blip would restart the instance at the worst possible moment.
func TestHandleHealth_StorageOffline(t *testing.T) {
	r := &Router{mux: http.NewServeMux(), storage: fakeStorage{engine: "postgres", connected: false}}
	code, body := healthBody(t, r)

	assert.Equal(t, http.StatusOK, code, "the probe must not kill the pod over a database outage")
	assert.Equal(t, "ok", body["status"])
	st := body["storage"].(map[string]interface{})
	assert.Equal(t, false, st["connected"])
}

func TestHandleHealth_NoStorage(t *testing.T) {
	r := &Router{mux: http.NewServeMux()}
	code, body := healthBody(t, r)

	assert.Equal(t, http.StatusOK, code)
	_, present := body["storage"]
	assert.False(t, present, "no storage source means no storage object")
}
