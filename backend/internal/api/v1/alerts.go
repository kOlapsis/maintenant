package v1

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/kolapsis/pulseboard/internal/alert"
)

// AlertHandler handles alert-related HTTP endpoints.
type AlertHandler struct {
	alertStore   alert.AlertStore
	channelStore alert.ChannelStore
	silenceStore alert.SilenceStore
	notifier     *alert.Notifier
	broker       *SSEBroker
}

// NewAlertHandler creates a new alert handler.
func NewAlertHandler(alertStore alert.AlertStore, channelStore alert.ChannelStore, silenceStore alert.SilenceStore, notifier *alert.Notifier, broker *SSEBroker) *AlertHandler {
	return &AlertHandler{
		alertStore:   alertStore,
		channelStore: channelStore,
		silenceStore: silenceStore,
		notifier:     notifier,
		broker:       broker,
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
		grouped[a.Severity] = append(grouped[a.Severity], a)
	}

	WriteJSON(w, http.StatusOK, grouped)
}

// HandleGetAlert handles GET /api/v1/alerts/{id}.
func (h *AlertHandler) HandleGetAlert(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
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
	if input.Name == "" {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	if input.URL == "" {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "url is required")
		return
	}

	// Validate webhook URL: require HTTPS and reject internal/private IPs.
	parsed, err := url.Parse(input.URL)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid URL format")
		return
	}
	if parsed.Scheme != "https" {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "webhook URL must use https scheme")
		return
	}
	hostname := parsed.Hostname()
	addrs, err := net.DefaultResolver.LookupHost(r.Context(), hostname)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "cannot resolve webhook hostname: "+hostname)
		return
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
			WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "webhook URL must not resolve to a private or internal IP address")
			return
		}
	}

	if input.Type == "" {
		input.Type = "webhook"
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
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create channel")
		return
	}
	ch.ID = id

	h.broker.Broadcast(SSEEvent{Type: EventChannelCreated, Data: ch})
	WriteJSON(w, http.StatusCreated, ch)
}

// HandleUpdateChannel handles PUT /api/v1/channels/{id}.
func (h *AlertHandler) HandleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
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

	if err := h.channelStore.UpdateChannel(r.Context(), ch); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update channel")
		return
	}

	h.broker.Broadcast(SSEEvent{Type: EventChannelUpdated, Data: ch})
	WriteJSON(w, http.StatusOK, ch)
}

// HandleDeleteChannel handles DELETE /api/v1/channels/{id}.
func (h *AlertHandler) HandleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
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

	h.broker.Broadcast(SSEEvent{Type: EventChannelDeleted, Data: map[string]interface{}{"id": id}})
	w.WriteHeader(http.StatusNoContent)
}

// HandleTestChannel handles POST /api/v1/channels/{id}/test.
func (h *AlertHandler) HandleTestChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
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

// --- Routing Rule handlers ---

// HandleCreateRoutingRule handles POST /api/v1/channels/{id}/rules.
func (h *AlertHandler) HandleCreateRoutingRule(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid channel ID")
		return
	}

	var input struct {
		SourceFilter   string `json:"source_filter"`
		SeverityFilter string `json:"severity_filter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	rule := &alert.RoutingRule{
		ChannelID:      channelID,
		SourceFilter:   input.SourceFilter,
		SeverityFilter: input.SeverityFilter,
	}

	ruleID, err := h.channelStore.InsertRoutingRule(r.Context(), rule)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create routing rule")
		return
	}
	rule.ID = ruleID

	WriteJSON(w, http.StatusCreated, rule)
}

// HandleDeleteRoutingRule handles DELETE /api/v1/channels/{id}/rules/{rule_id}.
func (h *AlertHandler) HandleDeleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	ruleID, err := strconv.ParseInt(r.PathValue("rule_id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid rule ID")
		return
	}

	if err := h.channelStore.DeleteRoutingRule(r.Context(), ruleID); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete routing rule")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
		EntityType      string `json:"entity_type"`
		EntityID        *int64 `json:"entity_id"`
		Source          string `json:"source"`
		Reason          string `json:"reason"`
		DurationSeconds int    `json:"duration_seconds"`
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
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create silence rule")
		return
	}
	rule.ID = silenceID
	rule.ExpiresAt = rule.StartsAt.Add(time.Duration(rule.DurationSeconds) * time.Second)
	rule.IsActive = true

	h.broker.Broadcast(SSEEvent{Type: EventSilenceCreated, Data: rule})
	WriteJSON(w, http.StatusCreated, rule)
}

// HandleCancelSilenceRule handles DELETE /api/v1/silence/{id}.
func (h *AlertHandler) HandleCancelSilenceRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "invalid silence rule ID")
		return
	}

	if err := h.silenceStore.CancelSilenceRule(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to cancel silence rule")
		return
	}

	h.broker.Broadcast(SSEEvent{Type: EventSilenceCancelled, Data: map[string]interface{}{"id": id}})
	w.WriteHeader(http.StatusNoContent)
}
