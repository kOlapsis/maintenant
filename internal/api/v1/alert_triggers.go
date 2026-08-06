// Copyright 2026 Benjamin Touchard (Kolapsis)
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
	"log/slog"
	"net/http"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
)

// AlertTriggerHandler handles /api/v1/alert-triggers/* endpoints.
type AlertTriggerHandler struct {
	triggerStore alert.TriggerStore
	channelStore alert.ChannelStore
	broker       *SSEBroker
}

// NewAlertTriggerHandler constructs a new handler.
func NewAlertTriggerHandler(ts alert.TriggerStore, cs alert.ChannelStore, broker *SSEBroker) *AlertTriggerHandler {
	return &AlertTriggerHandler{
		triggerStore: ts,
		channelStore: cs,
		broker:       broker,
	}
}

// triggerInput is the wire format accepted by POST/PUT.
type triggerInput struct {
	Name             string   `json:"name"`
	FilterSeverities string   `json:"filter_severities"`
	FilterSources    string   `json:"filter_sources"`
	FilterScopes     string   `json:"filter_scopes"`
	FilterTags       string   `json:"filter_tags"`
	Enabled          *bool    `json:"enabled"`
	ChannelIDs       []string `json:"channel_ids"`
}

// HandleListTriggers handles GET /api/v1/alert-triggers.
func (h *AlertTriggerHandler) HandleListTriggers(w http.ResponseWriter, r *http.Request) {
	triggers, err := h.triggerStore.ListTriggers(r.Context())
	if err != nil {
		slog.Error("list triggers", "error", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list triggers")
		return
	}
	if triggers == nil {
		triggers = []*alert.AlertTrigger{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"triggers": triggers})
}

// HandleGetTrigger handles GET /api/v1/alert-triggers/{id}.
func (h *AlertTriggerHandler) HandleGetTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid trigger ID")
		return
	}
	t, err := h.triggerStore.GetTrigger(r.Context(), id)
	if err != nil {
		slog.Error("get trigger", "error", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get trigger")
		return
	}
	if t == nil {
		WriteError(w, http.StatusNotFound, "trigger_not_found", "Trigger not found")
		return
	}
	WriteJSON(w, http.StatusOK, t)
}

// HandleCreateTrigger handles POST /api/v1/alert-triggers.
func (h *AlertTriggerHandler) HandleCreateTrigger(w http.ResponseWriter, r *http.Request) {
	var input triggerInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}
	if err := validateTriggerInput(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	if err := h.checkAdvancedFiltersGating(&input); err != nil {
		WriteError(w, http.StatusForbidden, "edition_required", err.Error())
		return
	}
	if err := h.checkChannelsExist(r, input.ChannelIDs); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	t := &alert.AlertTrigger{
		Name:             input.Name,
		FilterSeverities: input.FilterSeverities,
		FilterSources:    input.FilterSources,
		FilterScopes:     input.FilterScopes,
		FilterTags:       input.FilterTags,
		Enabled:          enabled,
		ChannelIDs:       input.ChannelIDs,
	}

	if _, err := h.triggerStore.InsertTrigger(r.Context(), t); err != nil {
		slog.Error("insert trigger", "error", err)
		if isUniqueErr(err) {
			WriteError(w, http.StatusConflict, "name_conflict", "Trigger name already in use")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create trigger")
		return
	}

	// Hydrate timestamps via re-read for response consistency.
	stored, _ := h.triggerStore.GetTrigger(r.Context(), t.ID)
	if stored != nil {
		t = stored
	}

	if h.broker != nil {
		h.broker.Broadcast(SSEEvent{Type: event.TriggerCreated, Data: t})
	}
	WriteJSON(w, http.StatusCreated, t)
}

// HandleUpdateTrigger handles PUT /api/v1/alert-triggers/{id}.
func (h *AlertTriggerHandler) HandleUpdateTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid trigger ID")
		return
	}

	existing, err := h.triggerStore.GetTrigger(r.Context(), id)
	if err != nil {
		slog.Error("update trigger: lookup", "error", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to lookup trigger")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "trigger_not_found", "Trigger not found")
		return
	}

	var input triggerInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}
	if err := validateTriggerInput(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	if err := h.checkAdvancedFiltersGating(&input); err != nil {
		WriteError(w, http.StatusForbidden, "edition_required", err.Error())
		return
	}
	if err := h.checkChannelsExist(r, input.ChannelIDs); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	existing.Name = input.Name
	existing.FilterSeverities = input.FilterSeverities
	existing.FilterSources = input.FilterSources
	existing.FilterScopes = input.FilterScopes
	existing.FilterTags = input.FilterTags
	existing.Enabled = enabled
	existing.ChannelIDs = input.ChannelIDs

	if err := h.triggerStore.UpdateTrigger(r.Context(), existing); err != nil {
		slog.Error("update trigger", "error", err)
		if isUniqueErr(err) {
			WriteError(w, http.StatusConflict, "name_conflict", "Trigger name already in use")
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update trigger")
		return
	}

	stored, _ := h.triggerStore.GetTrigger(r.Context(), existing.ID)
	if stored != nil {
		existing = stored
	}
	if h.broker != nil {
		h.broker.Broadcast(SSEEvent{Type: event.TriggerUpdated, Data: existing})
	}
	WriteJSON(w, http.StatusOK, existing)
}

// HandleDeleteTrigger handles DELETE /api/v1/alert-triggers/{id}.
func (h *AlertTriggerHandler) HandleDeleteTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid trigger ID")
		return
	}
	existing, err := h.triggerStore.GetTrigger(r.Context(), id)
	if err != nil {
		slog.Error("delete trigger: lookup", "error", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to lookup trigger")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "trigger_not_found", "Trigger not found")
		return
	}
	if err := h.triggerStore.DeleteTrigger(r.Context(), id); err != nil {
		slog.Error("delete trigger", "error", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete trigger")
		return
	}
	if h.broker != nil {
		h.broker.Broadcast(SSEEvent{Type: event.TriggerDeleted, Data: map[string]string{"id": id}})
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateTriggerInput enforces field-level rules common to create and update.
func validateTriggerInput(t *triggerInput) error {
	if t.Name == "" {
		return errors.New("field=name: required")
	}
	if len(t.Name) > 120 {
		return errors.New("field=name: must be 120 characters or fewer")
	}
	if len(t.ChannelIDs) == 0 {
		return errors.New("field=channel_ids: at least one channel is required")
	}
	return nil
}

// checkAdvancedFiltersGating returns an error when CE is used with Pro-only filters.
func (h *AlertTriggerHandler) checkAdvancedFiltersGating(t *triggerInput) error {
	if t.FilterScopes == "" && t.FilterTags == "" {
		return nil
	}
	if extension.Allows(extension.CapAlertAdvancedFilters) {
		return nil
	}
	return errors.New("advanced filters (scopes, tags) require the " +
		titleEdition(extension.MinEdition(extension.CapAlertAdvancedFilters)) + " edition")
}

// checkChannelsExist verifies that each channel_id points at an existing row.
func (h *AlertTriggerHandler) checkChannelsExist(r *http.Request, ids []string) error {
	for _, id := range ids {
		ch, err := h.channelStore.GetChannel(r.Context(), id)
		if err != nil {
			return errors.New("failed to validate channel_ids")
		}
		if ch == nil {
			return errors.New("field=channel_ids: channel " + id + " does not exist")
		}
	}
	return nil
}

// isUniqueErr is a best-effort check for the SQLite UNIQUE constraint failure.
func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "UNIQUE constraint failed")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
