package v1

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kolapsis/pulseboard/internal/webhook"
)

// WebhookHandler handles webhook subscription CRUD endpoints.
type WebhookHandler struct {
	store  webhook.WebhookSubscriptionStore
	logger *slog.Logger
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(store webhook.WebhookSubscriptionStore, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{store: store, logger: logger}
}

// HandleListWebhooks handles GET /api/v1/webhooks.
func (h *WebhookHandler) HandleListWebhooks(w http.ResponseWriter, r *http.Request) {
	subs, err := h.store.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list webhooks")
		return
	}
	if subs == nil {
		subs = []*webhook.WebhookSubscription{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": subs,
	})
}

type createWebhookRequest struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Secret     string   `json:"secret,omitempty"`
	EventTypes []string `json:"event_types,omitempty"`
}

// HandleCreateWebhook handles POST /api/v1/webhooks.
func (h *WebhookHandler) HandleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req createWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	// Validate name
	if req.Name == "" || len(req.Name) > 100 {
		WriteError(w, http.StatusBadRequest, "invalid_input", "Name is required (1-100 characters)")
		return
	}

	// Validate URL (must be HTTPS)
	if req.URL == "" || !strings.HasPrefix(req.URL, "https://") {
		WriteError(w, http.StatusBadRequest, "invalid_input", "URL must be a valid HTTPS URL")
		return
	}

	// Default event types
	if len(req.EventTypes) == 0 {
		req.EventTypes = []string{"*"}
	}

	// Validate event types
	for _, et := range req.EventTypes {
		if !webhook.ValidEventTypes[et] {
			WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid event type: "+et)
			return
		}
	}

	sub := &webhook.WebhookSubscription{
		ID:         uuid.New().String(),
		Name:       req.Name,
		URL:        req.URL,
		Secret:     req.Secret,
		EventTypes: req.EventTypes,
		IsActive:   true,
		CreatedAt:  time.Now().UTC(),
	}

	if err := h.store.Create(r.Context(), sub); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create webhook")
		return
	}

	// Don't return secret in response
	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          sub.ID,
		"name":        sub.Name,
		"url":         sub.URL,
		"event_types": sub.EventTypes,
		"is_active":   true,
		"created_at":  sub.CreatedAt,
	})
}

// HandleDeleteWebhook handles DELETE /api/v1/webhooks/{id}.
func (h *WebhookHandler) HandleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "invalid_input", "Webhook ID is required")
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		WriteError(w, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleTestWebhook handles POST /api/v1/webhooks/{id}/test.
func (h *WebhookHandler) HandleTestWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	testSub, err := h.store.GetByID(r.Context(), id)
	if err != nil || testSub == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}

	// Send a test payload synchronously
	payload := webhook.WebhookEvent{
		Type:      "test",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data: map[string]interface{}{
			"message": "PulseBoard webhook test",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to marshal test payload")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, testSub.URL, bytes.NewReader(body))
	if err != nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
		})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PulseBoard-Event", "test")
	req.Header.Set("X-PulseBoard-Delivery", uuid.New().String())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "delivered",
		"http_status": resp.StatusCode,
	})
}
