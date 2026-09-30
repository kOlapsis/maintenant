// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"time"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
)

// StatusAdminHandler handles admin endpoints for the public status page.
type StatusAdminHandler struct {
	components  status.ComponentStore
	incidents   status.IncidentStore
	subscribers status.SubscriberStore
	maintenance status.MaintenanceStore
	maintRunner status.MaintenanceRunner
	statusSvc   *status.Service
	mailer      status.Mailer
}

// NewStatusAdminHandler creates a status admin handler; a nil mailer means SMTP is not configured, a nil runner that no window ever runs.
func NewStatusAdminHandler(
	components status.ComponentStore,
	incidents status.IncidentStore,
	subscribers status.SubscriberStore,
	maintenance status.MaintenanceStore,
	maintRunner status.MaintenanceRunner,
	statusSvc *status.Service,
	mailer status.Mailer,
) *StatusAdminHandler {
	return &StatusAdminHandler{
		components:  components,
		incidents:   incidents,
		subscribers: subscribers,
		maintenance: maintenance,
		maintRunner: maintRunner,
		statusSvc:   statusSvc,
		mailer:      mailer,
	}
}

// --- Status Components ---

func (h *StatusAdminHandler) HandleListComponents(w http.ResponseWriter, r *http.Request) {
	components, err := h.components.ListComponents(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	for i := range components {
		components[i].DerivedStatus = h.statusSvc.DeriveComponentStatus(r.Context(), &components[i])
		if components[i].StatusOverride != nil {
			components[i].EffectiveStatus = *components[i].StatusOverride
		} else {
			components[i].EffectiveStatus = components[i].DerivedStatus
		}
	}
	WriteJSON(w, http.StatusOK, components)
}

func (h *StatusAdminHandler) HandleCreateComponent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CompositionMode string              `json:"composition_mode"`
		Monitors        []status.MonitorRef `json:"monitors"`
		MatchAllType    string              `json:"match_all_type"`
		DisplayName     string              `json:"display_name"`
		DisplayOrder    int                 `json:"display_order"`
		Visible         *bool               `json:"visible"`
		AutoIncident    bool                `json:"auto_incident"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}

	// Default to explicit mode if not specified.
	if req.CompositionMode == "" {
		req.CompositionMode = "explicit"
	}

	if req.CompositionMode != "explicit" && req.CompositionMode != "match-all" {
		WriteError(w, http.StatusBadRequest, "validation", "composition_mode must be 'explicit' or 'match-all'")
		return
	}
	if req.DisplayName == "" {
		WriteError(w, http.StatusBadRequest, "validation", "display_name is required")
		return
	}

	validTypes := map[string]bool{"container": true, "endpoint": true, "heartbeat": true, "certificate": true}

	if req.CompositionMode == "explicit" {
		if req.MatchAllType != "" {
			WriteError(w, http.StatusBadRequest, "validation", "match_all_type must be null in explicit mode")
			return
		}
		if len(req.Monitors) == 0 {
			WriteError(w, http.StatusBadRequest, "validation", "explicit-mode components require at least one monitor")
			return
		}
		for _, m := range req.Monitors {
			if !validTypes[m.Type] {
				WriteError(w, http.StatusBadRequest, "validation", "invalid monitor type: "+m.Type)
				return
			}
		}
	} else { // match-all
		if !validTypes[req.MatchAllType] {
			WriteError(w, http.StatusBadRequest, "validation", "match_all_type must be one of container, endpoint, heartbeat, certificate")
			return
		}
		if len(req.Monitors) > 0 {
			WriteError(w, http.StatusBadRequest, "validation", "monitors must be empty in match-all mode")
			return
		}
	}

	// Component cap, read from the single declaration. -1 means unlimited.
	if limit := extension.Limit(extension.ResourceStatusComponents); limit >= 0 {
		existing, err := h.components.ListComponents(r.Context())
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if len(existing) >= limit {
			refuseQuota(w, extension.ResourceStatusComponents)
			return
		}
	}

	visible := true
	if req.Visible != nil {
		visible = *req.Visible
	}

	c := &status.Component{
		CompositionMode: status.CompositionMode(req.CompositionMode),
		Monitors:        req.Monitors,
		MatchAllType:    req.MatchAllType,
		DisplayName:     req.DisplayName,
		DisplayOrder:    req.DisplayOrder,
		Visible:         visible,
		AutoIncident:    req.AutoIncident,
	}
	if _, err := h.components.CreateComponent(r.Context(), c); err != nil {
		slog.Error("failed to create status component", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal", "Failed to create component")
		return
	}
	c.EffectiveStatus = h.statusSvc.DeriveComponentStatus(r.Context(), c)
	h.statusSvc.AnnounceComponentChange(r.Context(), event.StatusComponentCreated, c.ID, c.Visible)
	WriteJSON(w, http.StatusCreated, c)
}

func (h *StatusAdminHandler) HandleUpdateComponent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid component ID")
		return
	}
	existing, err := h.components.GetComponent(r.Context(), id)
	if err != nil || existing == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Component not found")
		return
	}
	wasVisible := existing.Visible
	var req struct {
		CompositionMode *string             `json:"composition_mode"`
		Monitors        []status.MonitorRef `json:"monitors"`
		MatchAllType    *string             `json:"match_all_type"`
		DisplayName     *string             `json:"display_name"`
		DisplayOrder    *int                `json:"display_order"`
		Visible         *bool               `json:"visible"`
		StatusOverride  *string             `json:"status_override"`
		AutoIncident    *bool               `json:"auto_incident"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}

	// Mode immutability.
	if req.CompositionMode != nil && *req.CompositionMode != string(existing.CompositionMode) {
		WriteError(w, http.StatusBadRequest, "validation", "composition_mode is immutable; delete and recreate to change")
		return
	}
	if req.MatchAllType != nil && *req.MatchAllType != existing.MatchAllType {
		WriteError(w, http.StatusBadRequest, "validation", "match_all_type is immutable; delete and recreate to change")
		return
	}

	// Match-all: cannot edit monitors.
	if existing.CompositionMode == status.CompositionMatchAll && len(req.Monitors) > 0 {
		WriteError(w, http.StatusBadRequest, "validation", "monitors cannot be edited in match-all mode")
		return
	}

	// Explicit: if monitors provided, validate.
	if existing.CompositionMode == status.CompositionExplicit && req.Monitors != nil {
		if len(req.Monitors) == 0 {
			WriteError(w, http.StatusBadRequest, "validation", "explicit-mode components require at least one monitor")
			return
		}
		validTypes := map[string]bool{"container": true, "endpoint": true, "heartbeat": true, "certificate": true}
		for _, m := range req.Monitors {
			if !validTypes[m.Type] {
				WriteError(w, http.StatusBadRequest, "validation", "invalid monitor type: "+m.Type)
				return
			}
		}
		existing.Monitors = req.Monitors
	}

	if req.DisplayName != nil {
		existing.DisplayName = *req.DisplayName
	}
	if req.DisplayOrder != nil {
		existing.DisplayOrder = *req.DisplayOrder
	}
	if req.Visible != nil {
		existing.Visible = *req.Visible
	}
	if req.StatusOverride != nil {
		if *req.StatusOverride == "" {
			existing.StatusOverride = nil
		} else {
			if err := status.CheckComponentStatus("status_override", *req.StatusOverride); err != nil {
				WriteError(w, http.StatusBadRequest, "validation", err.Error())
				return
			}
			existing.StatusOverride = req.StatusOverride
		}
	}
	if req.AutoIncident != nil {
		existing.AutoIncident = *req.AutoIncident
	}

	if err := h.components.UpdateComponent(r.Context(), existing); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	existing.DerivedStatus = h.statusSvc.DeriveComponentStatus(r.Context(), existing)
	if existing.StatusOverride != nil {
		existing.EffectiveStatus = *existing.StatusOverride
	} else {
		existing.EffectiveStatus = existing.DerivedStatus
	}
	h.statusSvc.AnnounceComponentChange(r.Context(), event.StatusComponentUpdated, existing.ID, wasVisible || existing.Visible)
	WriteJSON(w, http.StatusOK, existing)
}

func (h *StatusAdminHandler) HandleDeleteComponent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid component ID")
		return
	}
	existing, err := h.components.GetComponent(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := h.components.DeleteComponent(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.statusSvc.AnnounceComponentChange(r.Context(), event.StatusComponentDeleted, id, existing != nil && existing.Visible)
	w.WriteHeader(http.StatusNoContent)
}

// --- Incidents ---

func (h *StatusAdminHandler) HandleListIncidents(w http.ResponseWriter, r *http.Request) {
	opts := status.ListIncidentsOpts{
		Status:   r.URL.Query().Get("status"),
		Severity: r.URL.Query().Get("severity"),
	}
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		opts.Limit = l
	}
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil {
		opts.Offset = o
	}
	incidents, total, err := h.incidents.ListIncidents(r.Context(), opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"incidents": incidents,
		"total":     total,
	})
}

func (h *StatusAdminHandler) HandleCreateIncident(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title        string   `json:"title"`
		Severity     string   `json:"severity"`
		Status       string   `json:"status"`
		ComponentIDs []string `json:"component_ids"`
		Message      string   `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	if req.Title == "" || req.Severity == "" {
		WriteError(w, http.StatusBadRequest, "validation", "title and severity are required")
		return
	}
	if req.Status == "" {
		req.Status = status.IncidentInvestigating
	}
	if err := status.CheckSeverity("severity", req.Severity); err != nil {
		WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if err := status.CheckIncidentStatus("status", req.Status); err != nil {
		WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if !h.componentIDsExist(w, r, req.ComponentIDs) {
		return
	}
	inc := &status.Incident{
		Title:    req.Title,
		Severity: req.Severity,
		Status:   req.Status,
	}
	id, err := h.incidents.CreateIncident(r.Context(), inc, req.ComponentIDs, req.Message)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	created, _ := h.incidents.GetIncident(r.Context(), id)
	if created != nil {
		inc = created
	}
	h.statusSvc.AnnounceIncident(r.Context(), inc, req.Message)

	WriteJSON(w, http.StatusCreated, inc)
}

func (h *StatusAdminHandler) HandlePostUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid incident ID")
		return
	}
	inc, err := h.incidents.GetIncident(r.Context(), id)
	if err != nil || inc == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Incident not found")
		return
	}
	var req struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	if req.Status == "" || req.Message == "" {
		WriteError(w, http.StatusBadRequest, "validation", "status and message are required")
		return
	}
	if err := status.CheckIncidentStatus("status", req.Status); err != nil {
		WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	upd := &status.IncidentUpdate{
		IncidentID: id,
		Status:     req.Status,
		Message:    req.Message,
	}
	updateID, err := h.incidents.CreateUpdate(r.Context(), upd)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	upd.ID = updateID
	h.statusSvc.AnnounceIncidentUpdate(r.Context(), inc, upd)

	WriteJSON(w, http.StatusCreated, upd)
}

func (h *StatusAdminHandler) HandleUpdateIncident(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid incident ID")
		return
	}
	inc, err := h.incidents.GetIncident(r.Context(), id)
	if err != nil || inc == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Incident not found")
		return
	}
	var req struct {
		Title        *string  `json:"title"`
		Severity     *string  `json:"severity"`
		ComponentIDs []string `json:"component_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	if req.Title != nil {
		inc.Title = *req.Title
	}
	if req.Severity != nil {
		if err := status.CheckSeverity("severity", *req.Severity); err != nil {
			WriteError(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		inc.Severity = *req.Severity
	}
	if !h.componentIDsExist(w, r, req.ComponentIDs) {
		return
	}
	if err := h.incidents.UpdateIncident(r.Context(), inc, req.ComponentIDs); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	updated, _ := h.incidents.GetIncident(r.Context(), id)
	if updated != nil {
		inc = updated
	}
	WriteJSON(w, http.StatusOK, inc)
}

func (h *StatusAdminHandler) HandleDeleteIncident(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid incident ID")
		return
	}
	if err := h.incidents.DeleteIncident(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Maintenance Windows ---

func (h *StatusAdminHandler) HandleListMaintenance(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		limit = l
	}
	windows, err := h.maintenance.ListMaintenance(r.Context(), statusFilter, limit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, windows)
}

func (h *StatusAdminHandler) HandleCreateMaintenance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title        string   `json:"title"`
		Description  string   `json:"description"`
		StartsAt     string   `json:"starts_at"`
		EndsAt       string   `json:"ends_at"`
		ComponentIDs []string `json:"component_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	if req.Title == "" {
		WriteError(w, http.StatusBadRequest, "validation", "title is required")
		return
	}
	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation", "Invalid starts_at format")
		return
	}
	endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation", "Invalid ends_at format")
		return
	}
	if endsAt.Before(startsAt) {
		WriteError(w, http.StatusBadRequest, "validation", "ends_at must be after starts_at")
		return
	}
	if !h.componentIDsExist(w, r, req.ComponentIDs) {
		return
	}
	mw := &status.MaintenanceWindow{
		Title:       req.Title,
		Description: req.Description,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
	}
	id, err := h.maintenance.CreateMaintenance(r.Context(), mw, req.ComponentIDs)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	created, _ := h.maintenance.GetMaintenance(r.Context(), id)
	if created != nil {
		mw = created
	}
	WriteJSON(w, http.StatusCreated, mw)
}

func (h *StatusAdminHandler) HandleUpdateMaintenance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid maintenance ID")
		return
	}
	existing, err := h.maintenance.GetMaintenance(r.Context(), id)
	if err != nil || existing == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Maintenance window not found")
		return
	}
	if existing.Active {
		WriteError(w, http.StatusConflict, "conflict", "Cannot modify an active maintenance window")
		return
	}
	var req struct {
		Title        *string  `json:"title"`
		Description  *string  `json:"description"`
		StartsAt     *string  `json:"starts_at"`
		EndsAt       *string  `json:"ends_at"`
		ComponentIDs []string `json:"component_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.StartsAt != nil {
		t, err := time.Parse(time.RFC3339, *req.StartsAt)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation", "Invalid starts_at format")
			return
		}
		existing.StartsAt = t
	}
	if req.EndsAt != nil {
		t, err := time.Parse(time.RFC3339, *req.EndsAt)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation", "Invalid ends_at format")
			return
		}
		existing.EndsAt = t
	}
	if existing.EndsAt.Before(existing.StartsAt) {
		WriteError(w, http.StatusBadRequest, "validation", "ends_at must be after starts_at")
		return
	}
	if !h.componentIDsExist(w, r, req.ComponentIDs) {
		return
	}
	if err := h.maintenance.UpdateMaintenance(r.Context(), existing, req.ComponentIDs); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	updated, _ := h.maintenance.GetMaintenance(r.Context(), id)
	if updated != nil {
		existing = updated
	}
	WriteJSON(w, http.StatusOK, existing)
}

func (h *StatusAdminHandler) HandleDeleteMaintenance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid maintenance ID")
		return
	}
	var err error
	if h.maintRunner != nil {
		err = h.maintRunner.DeleteWindow(r.Context(), id)
	} else {
		err = h.maintenance.DeleteMaintenance(r.Context(), id)
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// componentIDsExist answers 400 unless every id names a distinct existing component.
func (h *StatusAdminHandler) componentIDsExist(w http.ResponseWriter, r *http.Request, ids []string) bool {
	err := status.CheckComponentIDs(r.Context(), h.components, ids)
	var invalid *status.InvalidComponentIDsError
	switch {
	case errors.As(err, &invalid):
		WriteError(w, http.StatusBadRequest, "validation", invalid.Error())
		return false
	case err != nil:
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return false
	}
	return true
}

// --- Subscribers ---

func (h *StatusAdminHandler) HandleListSubscribers(w http.ResponseWriter, r *http.Request) {
	subs, err := h.subscribers.ListSubscribers(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	stats, err := h.subscribers.GetSubscriberStats(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	type maskedSub struct {
		ID        string    `json:"id"`
		Email     string    `json:"email"`
		Confirmed bool      `json:"confirmed"`
		CreatedAt time.Time `json:"created_at"`
	}
	masked := make([]maskedSub, 0, len(subs))
	for _, s := range subs {
		masked = append(masked, maskedSub{
			ID:        s.ID,
			Email:     maskEmail(s.Email),
			Confirmed: s.Confirmed,
			CreatedAt: s.CreatedAt,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"subscribers": masked,
		"total":       stats.Total,
		"confirmed":   stats.Confirmed,
	})
}

// --- SMTP ---

// HandleTestSmtp sends a test email to the given address through the SMTP server of the environment.
func (h *StatusAdminHandler) HandleTestSmtp(w http.ResponseWriter, r *http.Request) {
	if h.mailer == nil {
		WriteError(w, http.StatusBadRequest, "not_configured", "SMTP is not configured: set MAINTENANT_SMTP_HOST")
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
		return
	}
	addr, err := mail.ParseAddress(req.To)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation", "to must be a valid email address")
		return
	}
	if err := h.mailer.Send(r.Context(), addr.Address, "maintenant SMTP test",
		"This is a test email from the maintenant status page.\n\n"+
			"If you received it, subscription confirmations and incident notifications can be delivered.\n"); err != nil {
		WriteError(w, http.StatusBadGateway, "smtp_failed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func maskEmail(email string) string {
	for i, ch := range email {
		if ch == '@' {
			if i <= 1 {
				return email
			}
			return string(email[0]) + "***" + email[i:]
		}
	}
	return email
}
