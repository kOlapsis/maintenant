package v1

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/kolapsis/pulseboard/internal/heartbeat"
)

// PingHandler handles public ping endpoints.
type PingHandler struct {
	svc *heartbeat.Service
}

// NewPingHandler creates a new ping handler.
func NewPingHandler(svc *heartbeat.Service) *PingHandler {
	return &PingHandler{svc: svc}
}

// HandlePing handles GET|POST /ping/{uuid}
func (h *PingHandler) HandlePing(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this UUID")
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

	sourceIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		sourceIP = fwd
	}

	_, err := h.svc.ProcessPing(r.Context(), uuid, sourceIP, r.Method, payload)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this UUID")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to process ping")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// HandleStartPing handles GET|POST /ping/{uuid}/start
func (h *PingHandler) HandleStartPing(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this UUID")
		return
	}

	sourceIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		sourceIP = fwd
	}

	_, err := h.svc.ProcessStartPing(r.Context(), uuid, sourceIP, r.Method)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this UUID")
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
		WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this UUID")
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

	sourceIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		sourceIP = fwd
	}

	_, err = h.svc.ProcessExitCodePing(r.Context(), uuid, exitCode, sourceIP, r.Method, payload)
	if err != nil {
		if errors.Is(err, heartbeat.ErrHeartbeatNotFound) {
			WriteError(w, http.StatusNotFound, "HEARTBEAT_NOT_FOUND", "No heartbeat monitor found for this UUID")
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
