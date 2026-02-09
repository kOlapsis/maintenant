package status

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Handler serves the public status page and related endpoints.
type Handler struct {
	service    *Service
	sseHandler http.Handler
	tmpl       *template.Template
	logger     *slog.Logger
}

// NewHandler creates a new public status page handler.
// sseHandler should be an SSEBroker that implements http.Handler for /status/events.
func NewHandler(service *Service, sseHandler http.Handler, logger *slog.Logger) *Handler {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))

	return &Handler{
		service:    service,
		sseHandler: sseHandler,
		tmpl:       tmpl,
		logger:     logger,
	}
}

// Register registers all public status page routes on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// Serve static assets
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /status/static/", http.StripPrefix("/status/static/", http.FileServer(http.FS(staticSub))))

	// Public pages
	mux.HandleFunc("GET /status", h.HandleStatusPage)
	mux.HandleFunc("GET /status/api", h.HandleStatusAPI)
	mux.Handle("GET /status/events", h.sseHandler)
}

// HandleStatusPage renders the public status page.
func (h *Handler) HandleStatusPage(w http.ResponseWriter, r *http.Request) {
	data, err := h.service.GetPageData(r.Context())
	if err != nil {
		h.logger.Error("failed to get status page data", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30")
	if err := h.tmpl.ExecuteTemplate(w, "status.html", data); err != nil {
		h.logger.Error("failed to render status page", "error", err)
	}
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
