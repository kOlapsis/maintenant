// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/container"
)

type recordingLogStreamer struct{ ids []string }

func (s *recordingLogStreamer) StreamLogs(_ context.Context, externalID string, _ int, _ bool) (io.ReadCloser, error) {
	s.ids = append(s.ids, externalID)
	return io.NopCloser(strings.NewReader("line\n")), nil
}

func TestHandleLogStream_ContainerParameter(t *testing.T) {
	svc := container.NewService(container.Deps{Logger: slog.Default(), Store: &agentEnrichStore{containers: []*container.Container{
		{ID: "docker-1", ExternalID: "abc123", Name: "web", State: container.StateRunning, RuntimeType: "docker"},
		{ID: "kube-1", ExternalID: "default/Deployment/web", Name: "web", State: container.StateRunning, RuntimeType: "kubernetes"},
	}}})

	for _, tc := range []struct {
		name, url  string
		wantStatus int
		wantStream []string
	}{
		{"docker ignores it", "/api/v1/containers/docker-1/logs/stream?container=sidecar", http.StatusOK, []string{"abc123"}},
		{"kubernetes picks the container", "/api/v1/containers/kube-1/logs/stream?container=sidecar", http.StatusOK, []string{"default/Deployment/web/sidecar"}},
		{"parent path", "/api/v1/containers/docker-1/logs/stream?container=..%2Fdeadbeef", http.StatusBadRequest, nil},
		{"dots only", "/api/v1/containers/kube-1/logs/stream?container=..", http.StatusBadRequest, nil},
		{"slash", "/api/v1/containers/kube-1/logs/stream?container=a%2Fb", http.StatusBadRequest, nil},
		{"too long", "/api/v1/containers/kube-1/logs/stream?container=" + strings.Repeat("a", 64), http.StatusBadRequest, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			streamer := &recordingLogStreamer{}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/containers/{id}/logs/stream", NewLogStreamHandler(streamer, svc).HandleLogStream)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))

			assert.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			assert.Equal(t, tc.wantStream, streamer.ids)
		})
	}
}
