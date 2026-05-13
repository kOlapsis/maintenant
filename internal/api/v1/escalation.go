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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/kolapsis/maintenant/internal/alert/escalation"
)

// EscalationHandler handles HTTP endpoints for escalation policies.
type EscalationHandler struct {
	svc *escalation.Service
}

// NewEscalationHandler creates a new EscalationHandler.
func NewEscalationHandler(svc *escalation.Service) *EscalationHandler {
	return &EscalationHandler{svc: svc}
}

// mapServiceError maps service-layer errors to standard HTTP error responses.
func mapServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, escalation.ErrPolicyNotFound) {
		WriteError(w, http.StatusNotFound, "policy_not_found", "Policy not found")
	} else if errors.Is(err, escalation.ErrRunNotFound) {
		WriteError(w, http.StatusNotFound, "run_not_found", "Run not found")
	} else if errors.Is(err, escalation.ErrValidationFailed) {
		WriteError(w, http.StatusBadRequest, "validation_failed", err.Error())
	} else {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
	}
}

// HandleCreatePolicy handles POST /api/v1/escalation-policies.
func (h *EscalationHandler) HandleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	var req escalation.PolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	policy, err := h.svc.CreatePolicy(r.Context(), req)
	if err != nil {
		mapServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusCreated, policy)
}

// HandleListPolicies handles GET /api/v1/escalation-policies.
func (h *EscalationHandler) HandleListPolicies(w http.ResponseWriter, r *http.Request) {
	activeOnly := false
	if v := r.URL.Query().Get("active"); v == "true" {
		activeOnly = true
	}

	limits, err := h.svc.GetPlanLimits(r.Context())
	if err != nil {
		mapServiceError(w, err)
		return
	}

	policies, err := h.svc.ListPolicies(r.Context(), activeOnly)
	if err != nil {
		mapServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"policies": policies,
		"limits":   limits,
	})
}

// HandleGetPolicy handles GET /api/v1/escalation-policies/{id}.
func (h *EscalationHandler) HandleGetPolicy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid policy ID")
		return
	}

	policy, err := h.svc.GetPolicy(r.Context(), id)
	if err != nil {
		mapServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, policy)
}

// HandleDeletePolicy handles DELETE /api/v1/escalation-policies/{id}.
func (h *EscalationHandler) HandleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid policy ID")
		return
	}

	if err := h.svc.DeletePolicy(r.Context(), id); err != nil {
		mapServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleUpdatePolicy handles PUT /api/v1/escalation-policies/{id}.
func (h *EscalationHandler) HandleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid policy ID")
		return
	}
	var req escalation.PolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	policy, err := h.svc.UpdatePolicy(r.Context(), id, req)
	if err != nil {
		mapServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, policy)
}

// HandleSetPolicyActive handles PATCH /api/v1/escalation-policies/{id}/active.
func (h *EscalationHandler) HandleSetPolicyActive(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid policy ID")
		return
	}
	var body struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	policy, err := h.svc.SetPolicyActive(r.Context(), id, body.Active)
	if err != nil {
		mapServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"id":         policy.ID,
		"active":     policy.Active,
		"updated_at": policy.UpdatedAt,
	})
}

// HandleListAlertRuns handles GET /api/v1/alerts/{alert_id}/escalation-runs.
func (h *EscalationHandler) HandleListAlertRuns(w http.ResponseWriter, r *http.Request) {
	alertID, err := strconv.ParseInt(r.PathValue("alert_id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid alert ID")
		return
	}

	runs, err := h.svc.ListRunsForAlert(r.Context(), alertID)
	if err != nil {
		mapServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"runs": runs,
	})
}

// HandleGetRun handles GET /api/v1/escalation-runs/{run_id}.
func (h *EscalationHandler) HandleGetRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(r.PathValue("run_id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid run ID")
		return
	}

	run, err := h.svc.GetRun(r.Context(), runID)
	if err != nil {
		mapServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, run)
}

// HandleListPolicyRuns handles GET /api/v1/escalation-policies/{id}/runs.
func (h *EscalationHandler) HandleListPolicyRuns(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid policy ID")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	var cursor int64
	if v := r.URL.Query().Get("cursor"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cursor = n
		}
	}

	runs, err := h.svc.ListPolicyRuns(r.Context(), id, limit, cursor)
	if err != nil {
		mapServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"runs": runs,
	})
}

// HandleOverlapProbe handles POST /api/v1/escalation-policies/overlap-probe.
func (h *EscalationHandler) HandleOverlapProbe(w http.ResponseWriter, r *http.Request) {
	var req escalation.PolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	existing, err := h.svc.ListPolicies(r.Context(), false)
	if err != nil {
		mapServiceError(w, err)
		return
	}

	levels := make([]escalation.Level, len(req.Levels))
	for i, l := range req.Levels {
		levels[i] = escalation.Level{Order: i, DelaySeconds: l.DelaySeconds, ChannelIDs: l.ChannelIDs}
	}
	candidate := &escalation.Policy{
		Filters: req.Filters,
		Levels:  levels,
	}

	warnings := escalation.DetectOverlap(candidate, existing)
	if warnings == nil {
		warnings = []escalation.OverlapWarning{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"overlapping": warnings,
	})
}
