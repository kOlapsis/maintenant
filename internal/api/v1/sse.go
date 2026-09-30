// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
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

// BroadcastEvent is a convenience wrapper used by packages that cannot import SSEEvent directly.
func (b *SSEBroker) BroadcastEvent(eventType string, data any) {
	b.Broadcast(SSEEvent{Type: eventType, Data: data})
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

	setSSEHeaders(w.Header())

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
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
			flusher.Flush()
		}
	}
}

// setSSEHeaders marks a response as an event stream that no cache or buffering proxy may hold back.
func setSSEHeaders(h http.Header) {
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
}

// ClientCount returns the number of connected SSE clients.
func (b *SSEBroker) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
