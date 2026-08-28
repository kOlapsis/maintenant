package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRuntimeChecker struct{ connected bool }

func (s *stubRuntimeChecker) IsConnected() bool { return s.connected }


// TestLogStream_503WhenDegraded verifies that log stream returns 503 when runtime is disconnected.
func TestLogStream_503WhenDegraded(t *testing.T) {
	h := NewLogStreamHandler(&mockLogStreamer{lines: []string{"log"}}, nil)
	h.SetRuntimeChecker(&stubRuntimeChecker{connected: false})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/containers/{id}/logs/stream", h.HandleLogStream)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/1/logs/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "container monitoring unavailable", body["error"],
		"503 body must be exactly {\"error\":\"container monitoring unavailable\"}")
}

// TestLogStream_200WhenConnected verifies that log stream works normally when connected.
func TestLogStream_200WhenConnected(t *testing.T) {
	h := NewLogStreamHandler(&mockLogStreamer{lines: []string{"2026-01-01T00:00:00Z log line"}}, nil)
	h.SetRuntimeChecker(&stubRuntimeChecker{connected: true})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/containers/{id}/logs/stream", h.HandleLogStream)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/1/logs/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLogStream_503ExactBody ensures the exact JSON contract.
func TestLogStream_503ExactBody(t *testing.T) {
	h := NewLogStreamHandler(nil, nil)
	h.SetRuntimeChecker(&stubRuntimeChecker{connected: false})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/containers/{id}/logs/stream", h.HandleLogStream)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/42/logs/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.JSONEq(t, `{"error":"container monitoring unavailable"}`, w.Body.String())
}
