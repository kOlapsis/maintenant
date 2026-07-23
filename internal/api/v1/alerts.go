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
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/ssrf"
)

// AlertHandler handles alert-related HTTP endpoints.
type AlertHandler struct {
	alertStore           alert.AlertStore
	channelStore         alert.ChannelStore
	silenceStore         alert.SilenceStore
	notifier             *alert.Notifier
	broker               *SSEBroker
	allowPrivateWebhooks bool
	escalator            alert.Escalator
}

// NewAlertHandler creates a new alert handler.
func NewAlertHandler(alertStore alert.AlertStore, channelStore alert.ChannelStore, silenceStore alert.SilenceStore, notifier *alert.Notifier, broker *SSEBroker, allowPrivateWebhooks bool, escalator alert.Escalator) *AlertHandler {
	return &AlertHandler{
		alertStore:           alertStore,
		channelStore:         channelStore,
		silenceStore:         silenceStore,
		notifier:             notifier,
		broker:               broker,
		allowPrivateWebhooks: allowPrivateWebhooks,
		escalator:            escalator,
	}
}

// HandleListAlerts handles GET /api/v1/alerts.
func (h *AlertHandler) HandleListAlerts(w http.ResponseWriter, r *http.Request) {
	opts := alert.ListAlertsOpts{}

	opts.Source = r.URL.Query().Get("source")
	opts.Severity = r.URL.Query().Get("severity")
	opts.Status = r.URL.Query().Get("status")

	if before := r.URL.Query().Get("before"); before != "" {
		t, err := time.Parse(time.RFC3339, before)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid 'before' timestamp")
			return
		}
		opts.Before = &t
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > 200 {
			WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "limit must be 1-200")
			return
		}
		opts.Limit = n
	}

	if opts.Limit == 0 {
		opts.Limit = 50
	}

	alerts, err := h.alertStore.ListAlerts(r.Context(), opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list alerts")
		return
	}

	hasMore := len(alerts) > opts.Limit
	if hasMore {
		alerts = alerts[:opts.Limit]
	}

	if alerts == nil {
		alerts = []*alert.Alert{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"alerts":   alerts,
		"has_more": hasMore,
	})
}

// HandleGetActiveAlerts handles GET /api/v1/alerts/active.
func (h *AlertHandler) HandleGetActiveAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.alertStore.ListActiveAlerts(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list active alerts")
		return
	}

	grouped := map[string][]*alert.Alert{
		"critical": {},
		"warning":  {},
		"info":     {},
	}
	for _, a := range alerts {
		if a.AcknowledgedAt != nil {
			continue
		}
		grouped[a.Severity] = append(grouped[a.Severity], a)
	}

	WriteJSON(w, http.StatusOK, grouped)
}

// HandleGetAlert handles GET /api/v1/alerts/{id}.
func (h *AlertHandler) HandleGetAlert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid alert ID")
		return
	}

	a, err := h.alertStore.GetAlert(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get alert")
		return
	}
	if a == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "alert not found")
		return
	}

	WriteJSON(w, http.StatusOK, a)
}

// HandleAcknowledgeAlert handles POST /api/v1/alerts/{id}/acknowledge.
func (h *AlertHandler) HandleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid alert ID")
		return
	}

	var input struct {
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}
	if input.AcknowledgedBy == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "acknowledged_by is required")
		return
	}

	a, err := h.alertStore.GetAlert(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get alert")
		return
	}
	if a == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "alert not found")
		return
	}
	if a.Status != "active" || a.AcknowledgedAt != nil {
		WriteError(w, http.StatusConflict, "CONFLICT", "alert is not active")
		return
	}

	now := time.Now().UTC()
	if err := h.alertStore.AcknowledgeAlert(r.Context(), id, input.AcknowledgedBy, now); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to acknowledge alert")
		return
	}

	a.AcknowledgedAt = &now
	a.AcknowledgedBy = input.AcknowledgedBy

	h.broker.Broadcast(SSEEvent{Type: event.AlertAcknowledged, Data: a})

	if err := h.escalator.OnAlertAcknowledged(r.Context(), id, alert.Acknowledgment{By: input.AcknowledgedBy, At: now}); err != nil {
		slog.ErrorContext(r.Context(), "alert engine: OnAlertAcknowledged hook error", "error", err, "alert_id", id)
	}

	WriteJSON(w, http.StatusOK, a)
}

// --- Channel CRUD handlers ---

// HandleListChannels handles GET /api/v1/channels.
func (h *AlertHandler) HandleListChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := h.channelStore.ListChannels(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list channels")
		return
	}

	if channels == nil {
		channels = []*alert.NotificationChannel{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"channels": channels,
	})
}

// validateChannelURL validates a channel's destination. Email channels carry a
// recipient address delivered over SMTP — validate the address. Every other
// type is an HTTP webhook subject to the HTTPS + SSRF rules (fast-feedback;
// skipped in dev, where the notifier's dial-time guard remains the boundary).
func (h *AlertHandler) validateChannelURL(ctx context.Context, chType, rawURL string) error {
	if chType == "email" {
		if _, err := mail.ParseAddress(rawURL); err != nil {
			return errors.New("invalid email address")
		}
		return nil
	}
	if h.allowPrivateWebhooks {
		return nil
	}
	return ssrf.ValidateURL(ctx, rawURL)
}

// HandleCreateChannel handles POST /api/v1/channels.
func (h *AlertHandler) HandleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		URL     string `json:"url"`
		Headers string `json:"headers"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	proChannelTypes := map[string]bool{"slack": true, "teams": true, "email": true}
	if proChannelTypes[input.Type] && extension.CurrentEdition() != extension.Pro {
		WriteError(w, http.StatusForbidden, "PRO_REQUIRED", "This feature requires the Pro edition")
		return
	}

	if input.Name == "" {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	if input.URL == "" {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "url is required")
		return
	}

	if input.Type == "" {
		input.Type = "webhook"
	}

	if err := h.validateChannelURL(r.Context(), input.Type, input.URL); err != nil {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	ch := &alert.NotificationChannel{
		Name:    input.Name,
		Type:    input.Type,
		URL:     input.URL,
		Headers: input.Headers,
		Enabled: input.Enabled,
	}

	id, err := h.channelStore.InsertChannel(r.Context(), ch)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			WriteError(w, http.StatusConflict, "DUPLICATE_NAME", "A channel with this name already exists")
			return
		}
		slog.Error("failed to create channel", "error", err, "name", input.Name)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create channel")
		return
	}
	ch.ID = id

	h.broker.Broadcast(SSEEvent{Type: event.ChannelCreated, Data: ch})
	WriteJSON(w, http.StatusCreated, ch)
}

// HandleUpdateChannel handles PUT /api/v1/channels/{id}.
func (h *AlertHandler) HandleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid channel ID")
		return
	}

	ch, err := h.channelStore.GetChannel(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get channel")
		return
	}
	if ch == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "channel not found")
		return
	}

	var input struct {
		Name    *string `json:"name"`
		Type    *string `json:"type"`
		URL     *string `json:"url"`
		Headers *string `json:"headers"`
		Enabled *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	if input.Name != nil {
		ch.Name = *input.Name
	}
	if input.Type != nil {
		ch.Type = *input.Type
	}
	if input.URL != nil {
		ch.URL = *input.URL
	}
	if input.Headers != nil {
		ch.Headers = *input.Headers
	}
	if input.Enabled != nil {
		ch.Enabled = *input.Enabled
	}

	proChannelTypes := map[string]bool{"slack": true, "teams": true, "email": true}
	if proChannelTypes[ch.Type] && extension.CurrentEdition() != extension.Pro {
		WriteError(w, http.StatusForbidden, "PRO_REQUIRED", "This feature requires the Pro edition")
		return
	}

	// Re-validate the destination when the URL or type changed, so an update
	// can't smuggle in a private/internal URL the create path would reject.
	if input.URL != nil || input.Type != nil {
		if err := h.validateChannelURL(r.Context(), ch.Type, ch.URL); err != nil {
			WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
	}

	if err := h.channelStore.UpdateChannel(r.Context(), ch); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update channel")
		return
	}

	h.broker.Broadcast(SSEEvent{Type: event.ChannelUpdated, Data: ch})
	WriteJSON(w, http.StatusOK, ch)
}

// HandleDeleteChannel handles DELETE /api/v1/channels/{id}.
func (h *AlertHandler) HandleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid channel ID")
		return
	}

	ch, err := h.channelStore.GetChannel(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get channel")
		return
	}
	if ch == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "channel not found")
		return
	}

	if err := h.channelStore.DeleteChannel(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete channel")
		return
	}

	h.broker.Broadcast(SSEEvent{Type: event.ChannelDeleted, Data: map[string]interface{}{"id": id}})
	w.WriteHeader(http.StatusNoContent)
}

// HandleTestChannel handles POST /api/v1/channels/{id}/test.
func (h *AlertHandler) HandleTestChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid channel ID")
		return
	}

	ch, err := h.channelStore.GetChannel(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get channel")
		return
	}
	if ch == nil {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "channel not found")
		return
	}

	proChannelTypes := map[string]bool{"slack": true, "teams": true, "email": true}
	if proChannelTypes[ch.Type] && extension.CurrentEdition() != extension.Pro {
		WriteError(w, http.StatusForbidden, "PRO_REQUIRED", "This feature requires the Pro edition")
		return
	}

	statusCode, testErr := h.notifier.SendTestWebhook(r.Context(), ch)

	if testErr != nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status": "failed",
			"error":  testErr.Error(),
		})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "delivered",
		"response_code": statusCode,
	})
}

// --- Silence Rule handlers ---

// HandleListSilenceRules handles GET /api/v1/silence.
func (h *AlertHandler) HandleListSilenceRules(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active") == "true"

	rules, err := h.silenceStore.ListSilenceRules(r.Context(), activeOnly)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list silence rules")
		return
	}

	if rules == nil {
		rules = []*alert.SilenceRule{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"rules": rules,
	})
}

// HandleCreateSilenceRule handles POST /api/v1/silence.
func (h *AlertHandler) HandleCreateSilenceRule(w http.ResponseWriter, r *http.Request) {
	var input struct {
		EntityType      string  `json:"entity_type"`
		EntityID        *string `json:"entity_id"`
		Source          string  `json:"source"`
		Reason          string  `json:"reason"`
		DurationSeconds int     `json:"duration_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	if input.DurationSeconds <= 0 {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "duration_seconds must be positive")
		return
	}

	rule := &alert.SilenceRule{
		EntityType:      input.EntityType,
		EntityID:        input.EntityID,
		Source:          input.Source,
		Reason:          input.Reason,
		StartsAt:        time.Now().UTC(),
		DurationSeconds: input.DurationSeconds,
	}

	silenceID, err := h.silenceStore.InsertSilenceRule(r.Context(), rule)
	if err != nil {
		slog.Error("failed to create silence rule", "error", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create silence rule")
		return
	}
	rule.ID = silenceID
	rule.ExpiresAt = rule.StartsAt.Add(time.Duration(rule.DurationSeconds) * time.Second)
	rule.IsActive = true

	h.broker.Broadcast(SSEEvent{Type: event.SilenceCreated, Data: rule})
	WriteJSON(w, http.StatusCreated, rule)
}

// HandleCancelSilenceRule handles DELETE /api/v1/silence/{id}.
func (h *AlertHandler) HandleCancelSilenceRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid silence rule ID")
		return
	}

	if err := h.silenceStore.CancelSilenceRule(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to cancel silence rule")
		return
	}

	h.broker.Broadcast(SSEEvent{Type: event.SilenceCancelled, Data: map[string]interface{}{"id": id}})
	w.WriteHeader(http.StatusNoContent)
}
