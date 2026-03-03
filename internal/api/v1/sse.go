// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

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
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
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
