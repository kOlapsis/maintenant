package status

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Handler serves the public status page API and SSE endpoints.
type Handler struct {
	service    *Service
	sseHandler http.Handler
	logger     *slog.Logger
}

// NewHandler creates a new public status page handler.
// sseHandler should be an SSEBroker that implements http.Handler for /status/events.
func NewHandler(service *Service, sseHandler http.Handler, logger *slog.Logger) *Handler {
	return &Handler{
		service:    service,
		sseHandler: sseHandler,
		logger:     logger,
	}
}

// Middleware wraps an http.Handler (e.g. rate limiter).
type Middleware func(http.Handler) http.Handler

// Register registers the status API and SSE routes directly on the given mux.
// The status page itself is served by the SPA; the backend only provides
// the JSON API and SSE event stream.
func (h *Handler) Register(mux *http.ServeMux, mw Middleware) {
	mux.Handle("GET /status/api", mw(http.HandlerFunc(h.HandleStatusAPI)))
	mux.Handle("GET /status/events", mw(h.sseHandler))
}

// StatusAPIResponse is the JSON snapshot of current status.
type StatusAPIResponse struct {
	GlobalStatus  string             `json:"global_status"`
	GlobalMessage string             `json:"global_message"`
	UpdatedAt     time.Time          `json:"updated_at"`
	Groups        []APIGroupResponse `json:"groups"`
}

// APIGroupResponse is a component group in the JSON API.
type APIGroupResponse struct {
	Name       string              `json:"name"`
	Components []APIComponentBrief `json:"components"`
}

// APIComponentBrief is a brief component in the JSON API.
type APIComponentBrief struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
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
		GlobalStatus:  data.GlobalStatus,
		GlobalMessage: data.GlobalMessage,
		UpdatedAt:     time.Now().UTC(),
	}

	for _, g := range data.Groups {
		ag := APIGroupResponse{Name: g.Name}
		for _, c := range g.Components {
			ag.Components = append(ag.Components, APIComponentBrief{
				ID:     c.ID,
				Name:   c.DisplayName,
				Status: c.EffectiveStatus,
			})
		}
		resp.Groups = append(resp.Groups, ag)
	}

	if len(data.Ungrouped) > 0 {
		ag := APIGroupResponse{Name: "Other"}
		for _, c := range data.Ungrouped {
			ag.Components = append(ag.Components, APIComponentBrief{
				ID:     c.ID,
				Name:   c.DisplayName,
				Status: c.EffectiveStatus,
			})
		}
		resp.Groups = append(resp.Groups, ag)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(resp)
}
