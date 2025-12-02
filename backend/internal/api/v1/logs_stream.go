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

	"github.com/kolapsis/pulseboard/internal/container"
)

// LogStreamer abstracts runtime log streaming for the API layer.
type LogStreamer interface {
	// StreamLogs returns an io.ReadCloser for following container logs.
	// The reader must be closed by the caller. Demuxing (if needed) is handled internally by the runtime.
	StreamLogs(ctx context.Context, externalID string, lines int, timestamps bool) (io.ReadCloser, error)
}

// LogStreamHandler handles SSE log streaming endpoints.
type LogStreamHandler struct {
	streamer LogStreamer
	service  *container.Service
}

// NewLogStreamHandler creates a new log stream handler.
func NewLogStreamHandler(streamer LogStreamer, svc *container.Service) *LogStreamHandler {
	return &LogStreamHandler{streamer: streamer, service: svc}
}

// HandleLogStream handles GET /api/v1/containers/{id}/logs/stream.
// It opens an SSE connection and streams container logs in real-time.
func (h *LogStreamHandler) HandleLogStream(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ID", "Container ID must be an integer")
		return
	}

	// Look up the container to get its externalID.
	var externalID string
	var containerDBID int64 = id
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
	} else {
		externalID = idStr
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
	defer reader.Close()

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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

		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse timestamp from log line if present (format: "2006-01-02T15:04:05.000000000Z message")
		stream := "stdout"
		timestamp := time.Now().UTC().Format(time.RFC3339)
		logLine := line

		if len(line) > 30 && line[4] == '-' && line[10] == 'T' {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				timestamp = parts[0]
				logLine = parts[1]
			}
		}

		event := map[string]interface{}{
			"container_id": containerDBID,
			"line":         logLine,
			"stream":       stream,
			"timestamp":    timestamp,
		}

		data, err := json.Marshal(event)
		if err != nil {
			continue
		}

		fmt.Fprintf(w, "event: container.log_line\ndata: %s\n\n", data)
		flusher.Flush()
	}

	// If scanner stops (container stopped or error), emit error event.
	errEvent := map[string]interface{}{
		"container_id": containerDBID,
		"error":        "container stopped",
	}
	data, _ := json.Marshal(errEvent)
	fmt.Fprintf(w, "event: container.log_error\ndata: %s\n\n", data)
	flusher.Flush()
}
