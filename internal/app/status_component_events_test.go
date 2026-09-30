// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/kolapsis/maintenant/internal/api/v1"
)

// drainEvents returns what the broker already delivered, in order.
func drainEvents(ch chan v1.SSEEvent) []v1.SSEEvent {
	var out []v1.SSEEvent
	for {
		select {
		case evt := <-ch:
			out = append(out, evt)
		default:
			return out
		}
	}
}

func TestStatusComponentAdmin_AnnouncesEachChangeOnBothBuses(t *testing.T) {
	a, _ := newTestApp(t, nil)
	admin := make(chan v1.SSEEvent, 64)
	a.broker.AddObserver(admin)
	public := make(chan v1.SSEEvent, 64)
	a.statusBroker.AddObserver(public)
	jsonBody := map[string]string{"Content-Type": "application/json"}

	rec := serve(a, http.MethodPost, "/api/v1/status/components",
		`{"display_name":"Internal API","composition_mode":"match-all","match_all_type":"endpoint","visible":false}`, jsonBody)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = serve(a, http.MethodPut, "/api/v1/status/components/"+created.ID, `{"display_name":"Private API"}`, jsonBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = serve(a, http.MethodDelete, "/api/v1/status/components/"+created.ID, "", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	want := []string{
		"status.component_created", "status.global_changed",
		"status.component_updated", "status.global_changed",
		"status.component_deleted", "status.global_changed",
	}
	for bus, ch := range map[string]chan v1.SSEEvent{"admin": admin, "public": public} {
		events := drainEvents(ch)
		types := make([]string, 0, len(events))
		for _, evt := range events {
			types = append(types, evt.Type)
		}
		require.Equal(t, want, types, "%s bus", bus)
		for _, i := range []int{0, 2, 4} {
			assert.Equal(t, map[string]any{"component_id": created.ID}, events[i].Data,
				"%s bus: a hidden component travels by id only", bus)
		}
	}
}
