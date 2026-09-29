// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/kolapsis/maintenant/internal/outbound"
)

// OutboundHeartbeatHandler serves the outbound heartbeat endpoints.
type OutboundHeartbeatHandler struct {
	svc *outbound.Service
}

// NewOutboundHeartbeatHandler creates an outbound heartbeat handler.
func NewOutboundHeartbeatHandler(svc *outbound.Service) *OutboundHeartbeatHandler {
	return &OutboundHeartbeatHandler{svc: svc}
}

// HandleList handles GET /api/v1/outbound-heartbeats.
func (h *OutboundHeartbeatHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		WriteStoreError(w, err, "Failed to list outbound heartbeats")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{"outbound_heartbeats": list})
}

// HandleCreate handles POST /api/v1/outbound-heartbeats.
func (h *OutboundHeartbeatHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var in outbound.Input
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	o, err := h.svc.Create(r.Context(), in)
	if err != nil {
		writeOutboundError(w, err, "Failed to create outbound heartbeat")
		return
	}
	WriteJSON(w, http.StatusCreated, o)
}

// HandleUpdate handles PUT /api/v1/outbound-heartbeats/{id}.
func (h *OutboundHeartbeatHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var in outbound.Input
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	o, err := h.svc.Update(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeOutboundError(w, err, "Failed to update outbound heartbeat")
		return
	}
	WriteJSON(w, http.StatusOK, o)
}

// HandleDelete handles DELETE /api/v1/outbound-heartbeats/{id}.
func (h *OutboundHeartbeatHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeOutboundError(w, err, "Failed to delete outbound heartbeat")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleSend handles POST /api/v1/outbound-heartbeats/{id}/send.
func (h *OutboundHeartbeatHandler) HandleSend(w http.ResponseWriter, r *http.Request) {
	o, err := h.svc.SendNow(r.Context(), r.PathValue("id"))
	if err != nil {
		writeOutboundError(w, err, "Failed to send outbound heartbeat")
		return
	}
	WriteJSON(w, http.StatusOK, o)
}

func writeOutboundError(w http.ResponseWriter, err error, message string) {
	switch {
	case errors.Is(err, outbound.ErrNotFound):
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "Outbound heartbeat not found")
	case errors.Is(err, outbound.ErrInvalidInput):
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		slog.Error(message, "error", err)
		WriteStoreError(w, err, message)
	}
}
