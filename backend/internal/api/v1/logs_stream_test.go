package v1

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockLogStreamer is a test double for LogStreamer.
type mockLogStreamer struct {
	lines []string
	err   error
}

func (m *mockLogStreamer) StreamLogs(_ context.Context, _ string, _ int, _ bool) (io.ReadCloser, error) {
	if m.err != nil {
		return nil, m.err
	}
	content := strings.Join(m.lines, "\n")
	if len(m.lines) > 0 {
		content += "\n"
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func TestHandleLogStream(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		streamer    LogStreamer
		wantStatus  int
		wantSSE     bool
		wantContains string
	}{
		{
			name: "valid stream with lines",
			url:  "/api/v1/containers/1/logs/stream",
			streamer: &mockLogStreamer{
				lines: []string{
					"2026-02-25T10:00:00Z starting server",
					"2026-02-25T10:00:01Z listening on :8080",
				},
			},
			wantStatus:  http.StatusOK,
			wantSSE:     true,
			wantContains: "container.log_line",
		},
		{
			name:       "invalid container ID",
			url:        "/api/v1/containers/abc/logs/stream",
			streamer:   &mockLogStreamer{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "nil streamer returns bad gateway",
			url:        "/api/v1/containers/1/logs/stream",
			streamer:   nil,
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "streamer error returns bad gateway",
			url:        "/api/v1/containers/1/logs/stream",
			streamer:   &mockLogStreamer{err: fmt.Errorf("docker error")},
			wantStatus: http.StatusBadGateway,
		},
		{
			name: "custom lines param",
			url:  "/api/v1/containers/1/logs/stream?lines=50",
			streamer: &mockLogStreamer{
				lines: []string{"log line 1"},
			},
			wantStatus:  http.StatusOK,
			wantSSE:     true,
			wantContains: "container.log_line",
		},
		{
			name: "empty log stream emits error event",
			url:  "/api/v1/containers/1/logs/stream",
			streamer: &mockLogStreamer{
				lines: []string{},
			},
			wantStatus:  http.StatusOK,
			wantSSE:     true,
			wantContains: "container.log_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewLogStreamHandler(tt.streamer, nil)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/containers/{id}/logs/stream", handler.HandleLogStream)

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantSSE {
				assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
				assert.Contains(t, w.Body.String(), tt.wantContains)
			}
		})
	}
}

func TestHandleLogStream_LinesParsing(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantLines int // just verify the handler doesn't crash with various params
	}{
		{name: "default lines", url: "/api/v1/containers/1/logs/stream"},
		{name: "custom lines", url: "/api/v1/containers/1/logs/stream?lines=200"},
		{name: "max lines clamped", url: "/api/v1/containers/1/logs/stream?lines=1000"},
		{name: "invalid lines ignored", url: "/api/v1/containers/1/logs/stream?lines=abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamer := &mockLogStreamer{lines: []string{"test"}}
			handler := NewLogStreamHandler(streamer, nil)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/containers/{id}/logs/stream", handler.HandleLogStream)

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}
