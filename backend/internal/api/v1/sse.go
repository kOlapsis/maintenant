package v1

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// SSE event type constants for endpoint monitoring.
const (
	EventEndpointDiscovered    = "endpoint.discovered"
	EventEndpointStatusChanged = "endpoint.status_changed"
	EventEndpointRemoved       = "endpoint.removed"
	EventEndpointAlert         = "endpoint.alert"
	EventEndpointRecovery      = "endpoint.recovery"
	EventEndpointConfigError   = "endpoint.config_error"
)

// SSE event type constants for heartbeat monitoring.
const (
	EventHeartbeatCreated       = "heartbeat.created"
	EventHeartbeatPingReceived  = "heartbeat.ping_received"
	EventHeartbeatStatusChanged = "heartbeat.status_changed"
	EventHeartbeatAlert         = "heartbeat.alert"
	EventHeartbeatRecovery      = "heartbeat.recovery"
	EventHeartbeatDeleted       = "heartbeat.deleted"
)

// SSE event type constants for certificate monitoring.
const (
	EventCertificateCreated        = "certificate.created"
	EventCertificateCheckCompleted = "certificate.check_completed"
	EventCertificateStatusChanged  = "certificate.status_changed"
	EventCertificateAlert          = "certificate.alert"
	EventCertificateRecovery       = "certificate.recovery"
	EventCertificateDeleted        = "certificate.deleted"
)

// SSE event type constants for resource monitoring.
const (
	EventResourceSnapshot = "resource.snapshot"
	EventResourceAlert    = "resource.alert"
	EventResourceRecovery = "resource.recovery"
)

// SSE event type constants for alert engine.
const (
	EventAlertFired    = "alert.fired"
	EventAlertResolved = "alert.resolved"
	EventAlertSilenced = "alert.silenced"
)

// SSE event type constants for channel management.
const (
	EventChannelCreated = "channel.created"
	EventChannelUpdated = "channel.updated"
	EventChannelDeleted = "channel.deleted"
)

// SSE event type constants for silence rule management.
const (
	EventSilenceCreated   = "silence.created"
	EventSilenceCancelled = "silence.cancelled"
)

// SSE event type constants for runtime status.
const (
	EventRuntimeStatus = "runtime.status"
)

// SSE event type constants for update intelligence.
const (
	EventUpdateScanStarted   = "update.scan_started"
	EventUpdateScanCompleted = "update.scan_completed"
	EventUpdateDetected      = "update.detected"
	EventUpdatePinned        = "update.pinned"
	EventUpdateUnpinned      = "update.unpinned"
)

// SSE event type constants for public status page.
const (
	EventStatusComponentChanged  = "status.component_changed"
	EventStatusIncidentCreated   = "status.incident_created"
	EventStatusIncidentUpdated   = "status.incident_updated"
	EventStatusIncidentResolved  = "status.incident_resolved"
	EventStatusMaintenanceStart  = "status.maintenance_started"
	EventStatusMaintenanceEnd    = "status.maintenance_ended"
	EventStatusGlobalChanged     = "status.global_changed"
)

// SSEBroker manages Server-Sent Event connections and broadcasts events.
type SSEBroker struct {
	clients   map[chan SSEEvent]struct{}
	observers map[chan SSEEvent]struct{}
	mu        sync.RWMutex
	logger    *slog.Logger
}

// SSEEvent represents an event to be sent to SSE clients.
type SSEEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// NewSSEBroker creates a new SSE broker.
func NewSSEBroker(logger *slog.Logger) *SSEBroker {
	return &SSEBroker{
		clients:   make(map[chan SSEEvent]struct{}),
		observers: make(map[chan SSEEvent]struct{}),
		logger:    logger,
	}
}

// Broadcast sends an event to all connected SSE clients and observers.
func (b *SSEBroker) Broadcast(event SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			b.logger.Warn("SSE client buffer full, dropping event", "event_type", event.Type)
		}
	}

	for ch := range b.observers {
		select {
		case ch <- event:
		default:
			b.logger.Warn("SSE observer buffer full, dropping event", "event_type", event.Type)
		}
	}
}

// AddObserver registers a channel that receives all broadcast events (non-blocking).
func (b *SSEBroker) AddObserver(ch chan SSEEvent) {
	b.mu.Lock()
	b.observers[ch] = struct{}{}
	b.mu.Unlock()
}

// RemoveObserver unregisters an observer channel.
func (b *SSEBroker) RemoveObserver(ch chan SSEEvent) {
	b.mu.Lock()
	delete(b.observers, ch)
	b.mu.Unlock()
}

// ServeHTTP handles SSE connections.
func (b *SSEBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan SSEEvent, 64)

	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.clients, ch)
		b.mu.Unlock()
		close(ch)
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			data, err := json.Marshal(event.Data)
			if err != nil {
				b.logger.Error("marshal SSE event", "error", err)
				continue
			}
			eventType := strings.NewReplacer("\n", "", "\r", "").Replace(event.Type)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
			flusher.Flush()
		}
	}
}

// ClientCount returns the number of connected SSE clients.
func (b *SSEBroker) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
