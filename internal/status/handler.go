// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"mime"
	"net/http"
	"net/mail"
	"time"

	"github.com/kolapsis/maintenant/internal/ratelimit"
)

// Handler serves the public status page API and SSE endpoints.
type Handler struct {
	service         *Service
	sseHandler      http.Handler
	logger          *slog.Logger
	personalization *PersonalizationPublicHandler
	indexHTML       []byte
	subscribeRL     *ratelimit.Limiter
	pageURL         string
}

// NewHandler creates a new public status page handler.
// sseHandler should be an SSEBroker that implements http.Handler for /status/events.
// pageURL is the public address of the page, as PageURL resolves it.
func NewHandler(service *Service, sseHandler http.Handler, logger *slog.Logger, subscribeRL *ratelimit.Limiter, pageURL string) *Handler {
	return &Handler{
		service:     service,
		sseHandler:  sseHandler,
		logger:      logger,
		subscribeRL: subscribeRL,
		pageURL:     pageURL,
	}
}

// SetIndexHTML provides the Vue SPA index.html used to serve the status page.
func (h *Handler) SetIndexHTML(data []byte) {
	h.indexHTML = data
}

// SetPersonalizationHandler attaches the personalization public handler.
func (h *Handler) SetPersonalizationHandler(ph *PersonalizationPublicHandler) {
	h.personalization = ph
}

// Middleware wraps an http.Handler (e.g. rate limiter).
type Middleware func(http.Handler) http.Handler

// Register registers the status page, API, and SSE routes directly on the given mux.
// /status/ serves the Vue SPA index.html — Traefik rewrites the status subdomain root
// to /status/ so that Vue Router can initialise at the correct route.
func (h *Handler) Register(mux *http.ServeMux, mw Middleware) {
	outer := mw
	if outer == nil {
		outer = func(next http.Handler) http.Handler { return next }
	}
	mw = func(next http.Handler) http.Handler { return outer(limitBody(maxStatusBody, next)) }

	// Page HTML — catch-all for /status/ (more specific patterns below take precedence).
	mux.Handle("GET /status/", mw(http.HandlerFunc(h.HandleStatusPage)))
	// API & SSE endpoints.
	mux.Handle("GET /status/api", mw(http.HandlerFunc(h.HandleStatusAPI)))
	mux.Handle("GET /status/events", mw(h.sseHandler))
	mux.Handle("GET /status/feed.atom", mw(http.HandlerFunc(h.HandleAtomFeed)))
	mux.Handle("POST /status/subscribe", mw(http.HandlerFunc(h.HandleSubscribe)))
	mux.Handle("GET /status/confirm", mw(http.HandlerFunc(h.HandleConfirm)))
	mux.Handle("GET /status/unsubscribe", mw(http.HandlerFunc(h.HandleUnsubscribe)))
	if h.personalization != nil {
		mux.Handle("GET /status/settings.json", mw(http.HandlerFunc(h.personalization.HandleSettingsJSON)))
	}
}

const statusMetaTag = `<meta name="maintenant-status" content="true">`

// HandleStatusPage serves the Vue SPA index.html for the /status/ path.
// A meta tag is injected so that Vue Router initialises at /status when the
// page is accessed from the dedicated status subdomain (browser path is "/").
// It must not be an inline <script>: the CSP hashes only the scripts already
// present in the embedded index.html, so an injected script gets blocked.
func (h *Handler) HandleStatusPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/status/" && r.URL.Path != "/status" {
		http.NotFound(w, r)
		return
	}
	if h.indexHTML == nil {
		http.Error(w, "Status page not available", http.StatusServiceUnavailable)
		return
	}
	html := bytes.Replace(h.indexHTML, []byte("</head>"), []byte(statusMetaTag+"</head>"), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write(html)
}

// StatusAPIResponse is the JSON snapshot of current status.
type StatusAPIResponse struct {
	GlobalStatus           string              `json:"global_status"`
	GlobalMessage          string              `json:"global_message"`
	UpdatedAt              time.Time           `json:"updated_at"`
	Components             []APIComponentBrief `json:"components"`
	ActiveIncidents        []APIIncidentBrief  `json:"active_incidents"`
	UpcomingMaint          []APIMaintBrief     `json:"upcoming_maintenance"`
	PersonalizationVersion int64               `json:"personalization_version,omitempty"`
	SubscriptionsEnabled   bool                `json:"subscriptions_enabled"`
}

// APIComponentBrief is a brief component in the JSON API.
type APIComponentBrief struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Status   string       `json:"status"`
	Monitors []MonitorRef `json:"monitors"`
}

// APIIncidentBrief is a brief incident in the JSON API.
type APIIncidentBrief struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Severity     string          `json:"severity"`
	Status       string          `json:"status"`
	Components   []string        `json:"components"`
	CreatedAt    time.Time       `json:"created_at"`
	LatestUpdate *APIUpdateBrief `json:"latest_update,omitempty"`
}

// APIUpdateBrief is a brief incident update in the JSON API.
type APIUpdateBrief struct {
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// APIMaintBrief is a brief maintenance window in the JSON API.
type APIMaintBrief struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	Components []string  `json:"components"`
}

// HandleStatusAPI serves the JSON status snapshot.
func (h *Handler) HandleStatusAPI(w http.ResponseWriter, r *http.Request) {
	data, err := h.service.GetPageData(r.Context())
	if err != nil {
		h.logger.Error("failed to get status API data", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	resp := StatusAPIResponse{
		GlobalStatus:         data.GlobalStatus,
		GlobalMessage:        data.GlobalMessage,
		UpdatedAt:            time.Now().UTC(),
		SubscriptionsEnabled: h.service.SubscriptionsEnabled(),
	}
	if h.personalization != nil {
		resp.PersonalizationVersion = h.personalization.GetVersion(r)
	}

	for _, c := range data.Components {
		resp.Components = append(resp.Components, APIComponentBrief{
			ID:       c.ID,
			Name:     c.DisplayName,
			Status:   c.EffectiveStatus,
			Monitors: c.Monitors,
		})
	}

	for _, inc := range data.ActiveIncidents {
		brief := APIIncidentBrief{
			ID:         inc.ID,
			Title:      inc.Title,
			Severity:   inc.Severity,
			Status:     inc.Status,
			Components: PublicComponentNames(inc.Components),
			CreatedAt:  inc.CreatedAt,
		}
		if len(inc.Updates) > 0 {
			u := inc.Updates[0]
			brief.LatestUpdate = &APIUpdateBrief{
				Status:    u.Status,
				Message:   u.Message,
				CreatedAt: u.CreatedAt,
			}
		}
		resp.ActiveIncidents = append(resp.ActiveIncidents, brief)
	}

	for _, mw := range data.Maintenance {
		resp.UpcomingMaint = append(resp.UpcomingMaint, APIMaintBrief{
			ID:         mw.ID,
			Title:      mw.Title,
			StartsAt:   mw.StartsAt,
			EndsAt:     mw.EndsAt,
			Components: PublicComponentNames(mw.Components),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(resp)
}

// --- Subscription endpoints ---

// maxStatusBody bounds every public status-page request body.
const maxStatusBody = 4 << 10

// maxEmailLength is the longest address RFC 5321 lets a mailbox be.
const maxEmailLength = 254

// limitBody caps the request body of a public route.
func limitBody(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

// HandleSubscribe processes a new email subscription request.
func (h *Handler) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	if !h.service.SubscriptionsEnabled() {
		writeSubscriptionsUnavailable(w)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxStatusBody)

	if !h.subscribeRL.AllowRequest(r) {
		h.subscribeRL.Reject(w)
		return
	}

	var req struct {
		Email string `json:"email"`
	}

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		mediaType = ""
	}
	switch mediaType {
	case "application/json":
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeJSONError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body too large")
				return
			}
			writeJSONError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON")
			return
		}
	case "application/x-www-form-urlencoded":
		if err := r.ParseForm(); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeJSONError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body too large")
				return
			}
			writeJSONError(w, http.StatusBadRequest, "invalid_body", "Invalid form")
			return
		}
		req.Email = r.PostFormValue("email")
	default:
		writeJSONError(w, http.StatusUnsupportedMediaType, "unsupported_media_type",
			"Content-Type must be application/json or application/x-www-form-urlencoded")
		return
	}

	if len(req.Email) > maxEmailLength {
		writeJSONError(w, http.StatusBadRequest, "invalid_email", "Email is too long")
		return
	}
	addr, err := mail.ParseAddress(req.Email)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_email", "Email is not a valid address")
		return
	}
	req.Email = addr.Address

	if err := h.service.subscribers.Subscribe(r.Context(), req.Email); err != nil {
		h.logger.Error("subscribe failed", "error", err)
		if errors.Is(err, ErrSubscriptionsDisabled) {
			writeSubscriptionsUnavailable(w)
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "subscription_failed", "Subscription failed")
		return
	}

	// New, pending or already confirmed: the answer is the same, so it reveals nobody's subscription.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "confirmation_sent"})
}

func writeSubscriptionsUnavailable(w http.ResponseWriter) {
	writeJSONError(w, http.StatusServiceUnavailable, "subscriptions_unavailable",
		"Email subscriptions are not available on this status page")
}

// writeJSONError answers with the error shape of the rest of the API.
func writeJSONError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// HandleConfirm processes a subscription confirmation.
func (h *Handler) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeSimpleHTML(w, http.StatusBadRequest, "Error", "Missing confirmation token.")
		return
	}

	if !h.service.SubscriptionsEnabled() {
		writeSimpleHTML(w, http.StatusServiceUnavailable, "Error", "Subscriptions not available.")
		return
	}

	if err := h.service.subscribers.Confirm(r.Context(), token); err != nil {
		writeSimpleHTML(w, http.StatusBadRequest, "Error", err.Error())
		return
	}

	writeSimpleHTML(w, http.StatusOK, "Confirmed", "Your subscription has been confirmed. You will receive email notifications for status changes.")
}

// HandleUnsubscribe processes an unsubscribe request.
func (h *Handler) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeSimpleHTML(w, http.StatusBadRequest, "Error", "Missing unsubscribe token.")
		return
	}

	if h.service.subscribers == nil {
		writeSimpleHTML(w, http.StatusServiceUnavailable, "Error", "Subscriptions not available.")
		return
	}

	if err := h.service.subscribers.Unsubscribe(r.Context(), token); err != nil {
		writeSimpleHTML(w, http.StatusBadRequest, "Error", err.Error())
		return
	}

	writeSimpleHTML(w, http.StatusOK, "Unsubscribed", "You have been unsubscribed and will no longer receive notifications.")
}

// writeSimpleHTML renders a minimal HTML page for confirm/unsubscribe results.
func writeSimpleHTML(w http.ResponseWriter, statusCode int, title, message string) {
	// Escaped: message can carry an error string derived from request input.
	t, m := html.EscapeString(title), html.EscapeString(message)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>` + t + `</title>
<style>body{font-family:system-ui,sans-serif;max-width:480px;margin:80px auto;text-align:center;color:#333}h1{font-size:1.5rem}</style>
</head><body><h1>` + t + `</h1><p>` + m + `</p></body></html>`))
}
