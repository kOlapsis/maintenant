package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
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

	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "RUNTIME_UNAVAILABLE", body.Error.Code)
	assert.NotEmpty(t, body.Error.Message)
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

type failingLogFetcher struct{}

func (failingLogFetcher) FetchLogs(context.Context, string, int, bool) ([]string, error) {
	return nil, errors.New("no such container")
}

func TestHandleLogs_AnswersLikeTheStreamWhenTheRuntimeIsDisconnected(t *testing.T) {
	c := &container.Container{ID: "ctr-uuid", ExternalID: "cafe", Name: "db", AgentID: uid.LocalAgent}
	local := &countingLogFetcher{lines: []string{"stale"}}
	h := logsHandler(t, c, local, nil)
	h.SetRuntimeChecker(&stubRuntimeChecker{connected: false})

	rec := doLogsRequest(h, c.ID)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "RUNTIME_UNAVAILABLE", body.Error.Code)
	assert.Zero(t, local.calls, "a disconnected runtime is not asked")
}

func TestHandleLogs_KeepsItsOwnCodeForAFailedRead(t *testing.T) {
	c := &container.Container{ID: "ctr-uuid", ExternalID: "cafe", Name: "db", AgentID: uid.LocalAgent}
	h := logsHandler(t, c, nil, nil)
	h.SetLogFetcher(failingLogFetcher{})
	h.SetRuntimeChecker(&stubRuntimeChecker{connected: true})

	rec := doLogsRequest(h, c.ID)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "LOGS_UNAVAILABLE", body.Error.Code)
}
