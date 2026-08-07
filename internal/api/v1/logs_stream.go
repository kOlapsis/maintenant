// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package v1

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/kolapsis/maintenant/internal/container"
)

// LogStreamer abstracts runtime log streaming for the API layer.
type LogStreamer interface {
	// StreamLogs returns an io.ReadCloser for following container logs.
	// The reader must be closed by the caller. Demuxing (if needed) is handled internally by the runtime.
	StreamLogs(ctx context.Context, externalID string, lines int, timestamps bool) (io.ReadCloser, error)
}

// LogStreamHandler handles SSE log streaming endpoints.
type LogStreamHandler struct {
	streamer       LogStreamer
	service        *container.Service
	runtimeChecker RuntimeChecker
	logRequester   AgentLogRequester
	agentDirectory AgentDirectory
}

// SetAgentDirectory wires agent identity resolution so errors about a remote
// container name the agent the operator knows, not its UUID.
func (h *LogStreamHandler) SetAgentDirectory(ad AgentDirectory) {
	h.agentDirectory = ad
}

// NewLogStreamHandler creates a new log stream handler.
func NewLogStreamHandler(streamer LogStreamer, svc *container.Service) *LogStreamHandler {
	return &LogStreamHandler{streamer: streamer, service: svc}
}

// SetRuntimeChecker injects the runtime availability checker.
func (h *LogStreamHandler) SetRuntimeChecker(rc RuntimeChecker) {
	h.runtimeChecker = rc
}

// SetLogRequester wires the agent command channel for following logs of remote
// containers.
func (h *LogStreamHandler) SetLogRequester(lr AgentLogRequester) {
	h.logRequester = lr
}

// HandleLogStream handles GET /api/v1/containers/{id}/logs/stream.
// It opens an SSE connection and streams container logs in real-time.
func (h *LogStreamHandler) HandleLogStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID is required")
		return
	}

	// Look up the container to get its externalID.
	var externalID string
	var agentID string
	var containerDBID = id
	if h.service != nil {
		c, err := h.service.GetContainer(r.Context(), id)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get container")
			return
		}
		if c == nil {
			WriteError(w, http.StatusNotFound, "CONTAINER_NOT_FOUND", "Container not found")
			return
		}
		externalID = c.ExternalID
		agentID = c.AgentID
	} else {
		externalID = id
	}

	// The local runtime being down says nothing about a remote agent's ability to
	// serve its own logs, so only gate the local path on it.
	remote := isRemoteAgent(agentID)
	if !remote && h.runtimeChecker != nil && !h.runtimeChecker.IsConnected() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"container monitoring unavailable"}`))
		return
	}

	lines := 100
	if l := r.URL.Query().Get("lines"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			lines = n
			if lines > 500 {
				lines = 500
			}
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "SSE_NOT_SUPPORTED", "Streaming not supported")
		return
	}

	if remote {
		h.streamRemote(w, r, flusher, containerDBID, agentID, externalID, lines)
		return
	}

	if h.streamer == nil {
		WriteError(w, http.StatusBadGateway, "RUNTIME_UNAVAILABLE",
			"Cannot connect to container runtime for log streaming.")
		return
	}

	// Build the stream target: externalID + optional container name for K8s
	streamID := externalID
	if containerName := r.URL.Query().Get("container"); containerName != "" {
		streamID = streamID + "/" + containerName
	}

	reader, err := h.streamer.StreamLogs(r.Context(), streamID, lines, true)
	if err != nil {
		WriteError(w, http.StatusBadGateway, "LOGS_UNAVAILABLE",
			"Cannot retrieve logs from container runtime")
		return
	}
	defer func() { _ = reader.Close() }()

	// Set SSE headers. CORS is deliberately absent: the cors() middleware
	// applies the configured policy, and forcing a wildcard here let any site
	// an operator visited read this container's logs.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	ctx := r.Context()
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !writeLogLine(w, flusher, containerDBID, scanner.Text()) {
			return
		}
	}

	// If scanner stops (container stopped or error), emit error event.
	writeLogError(w, flusher, containerDBID, "container stopped")
}

// writeLogLine emits one SSE log line, reporting whether the client is still
// reachable. Shared by the local and remote paths so both produce the same shape.
func writeLogLine(w http.ResponseWriter, flusher http.Flusher, containerDBID, line string) bool {
	if line == "" {
		return true
	}

	// Parse timestamp from log line if present (format: "2006-01-02T15:04:05.000000000Z message")
	timestamp := time.Now().UTC().Format(time.RFC3339)
	logLine := line

	if len(line) > 30 && line[4] == '-' && line[10] == 'T' {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			timestamp = parts[0]
			logLine = parts[1]
		}
	}

	data, err := json.Marshal(map[string]interface{}{
		"container_id": containerDBID,
		"line":         logLine,
		"stream":       "stdout",
		"timestamp":    timestamp,
	})
	if err != nil {
		return true
	}

	if _, err := fmt.Fprintf(w, "event: container.log_line\ndata: %s\n\n", data); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// writeLogError emits the terminal SSE error event.
func writeLogError(w http.ResponseWriter, flusher http.Flusher, containerDBID, reason string) {
	data, _ := json.Marshal(map[string]interface{}{
		"container_id": containerDBID,
		"error":        reason,
	})
	_, _ = fmt.Fprintf(w, "event: container.log_error\ndata: %s\n\n", data)
	flusher.Flush()
}

// streamRemote follows the logs of a container living on a remote agent, relaying
// the chunks it sends back as SSE events.
func (h *LogStreamHandler) streamRemote(
	w http.ResponseWriter, r *http.Request, flusher http.Flusher,
	containerDBID, agentID, externalID string, lines int,
) {
	if h.logRequester == nil {
		WriteError(w, http.StatusBadGateway, "RUNTIME_UNAVAILABLE",
			"Multi-host agent support is not enabled on this server.")
		return
	}

	results, release, err := h.logRequester.SendCommand(r.Context(), agentID, agentserver.CapabilityLogs,
		agentserver.LogsCommand(externalID, lines, true, true))
	if err != nil {
		writeRemoteLogsError(w, resolveAgentLabel(r.Context(), h.agentDirectory, agentID), err)
		return
	}
	// Releasing tells the agent to stop tailing as soon as the viewer goes away.
	defer release()

	// Same as the local path: no wildcard CORS, cors() owns the policy.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case res, ok := <-results:
			if !ok {
				writeLogError(w, flusher, containerDBID, "agent disconnected")
				return
			}
			if code := res.GetErrorCode(); code != "" {
				writeLogError(w, flusher, containerDBID, res.GetErrorMessage())
				return
			}
			for _, line := range res.GetLogs().GetLines() {
				if !writeLogLine(w, flusher, containerDBID, line) {
					return
				}
			}
			if res.GetLast() {
				writeLogError(w, flusher, containerDBID, "container stopped")
				return
			}
		}
	}
}
