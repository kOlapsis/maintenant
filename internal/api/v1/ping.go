// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/ratelimit"
)

// PingHandler handles public ping endpoints.
type PingHandler struct {
	svc      *heartbeat.Service
	clientIP *ratelimit.ClientIPResolver
	ipRL     *ratelimit.Limiter
	tokenRL  *ratelimit.Limiter
}

// NewPingHandler creates a ping handler that records the source address the way the rate limit resolves it.
func NewPingHandler(svc *heartbeat.Service, clientIP *ratelimit.ClientIPResolver) *PingHandler {
	return &PingHandler{svc: svc, clientIP: clientIP}
}

// HandlePing handles GET|POST /ping/{uuid}
func (h *PingHandler) HandlePing(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this token")
		return
	}
	if !h.allow(w, r, uuid) {
		return
	}

	var payload *string
	if r.Method == http.MethodPost && r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, heartbeat.MaxPayloadBytes+1))
		if err == nil && len(body) > 0 {
			s := string(body)
			if len(s) > heartbeat.MaxPayloadBytes {
				s = s[:heartbeat.MaxPayloadBytes]
			}
			payload = &s
		}
	}

	sourceIP := h.clientIP.ClientIP(r)

	_, pingID, err := h.svc.RecordPing(r.Context(), uuid, sourceIP, r.Method, payload, nil)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this token")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to process ping")
		return
	}

	resp := map[string]interface{}{"ok": true}
	if pingID != "" {
		resp["id"] = pingID
	}
	WriteJSON(w, http.StatusOK, resp)
}

// HandleStartPing handles GET|POST /ping/{uuid}/start
func (h *PingHandler) HandleStartPing(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this token")
		return
	}
	if !h.allow(w, r, uuid) {
		return
	}

	sourceIP := h.clientIP.ClientIP(r)

	_, err := h.svc.ProcessStartPing(r.Context(), uuid, sourceIP, r.Method)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this token")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to process start ping")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// HandleExitCodePing handles GET|POST /ping/{uuid}/{exit_code}
func (h *PingHandler) HandleExitCodePing(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this token")
		return
	}
	if !h.allow(w, r, uuid) {
		return
	}

	exitCodeStr := r.PathValue("exit_code")
	exitCode, err := strconv.Atoi(exitCodeStr)
	if err != nil || exitCode < 0 || exitCode > 255 {
		WriteError(w, http.StatusBadRequest, "INVALID_EXIT_CODE", "Exit code must be an integer between 0 and 255")
		return
	}

	var payload *string
	if r.Method == http.MethodPost && r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, heartbeat.MaxPayloadBytes+1))
		if err == nil && len(body) > 0 {
			s := string(body)
			if len(s) > heartbeat.MaxPayloadBytes {
				s = s[:heartbeat.MaxPayloadBytes]
			}
			payload = &s
		}
	}

	sourceIP := h.clientIP.ClientIP(r)

	_, err = h.svc.ProcessExitCodePing(r.Context(), uuid, exitCode, sourceIP, r.Method, payload)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this token")
			return
		}
		if errors.Is(err, heartbeat.ErrInvalidExitCode) {
			WriteError(w, http.StatusBadRequest, "INVALID_EXIT_CODE", "Exit code must be an integer between 0 and 255")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to process ping")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "exit_code": exitCode})
}

// SetLimiters sets the per-address bucket for unknown tokens and the per-token bucket for known ones.
func (h *PingHandler) SetLimiters(ipRL, tokenRL *ratelimit.Limiter) {
	h.ipRL, h.tokenRL = ipRL, tokenRL
}

// allow counts a known token against its own bucket and an unknown one against the caller's address, answering 429 when refused.
func (h *PingHandler) allow(w http.ResponseWriter, r *http.Request, token string) bool {
	if hb, _ := h.svc.GetHeartbeatByPingToken(r.Context(), token); hb != nil {
		if h.tokenRL != nil && !h.tokenRL.Allow(token) {
			h.tokenRL.Reject(w)
			return false
		}
		return true
	}
	if h.ipRL != nil && !h.ipRL.AllowRequest(r) {
		h.ipRL.Reject(w)
		return false
	}
	return true
}
