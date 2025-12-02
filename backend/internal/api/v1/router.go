package v1

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/kolapsis/pulseboard/internal/alert"
	"github.com/kolapsis/pulseboard/internal/pro"
	"github.com/kolapsis/pulseboard/internal/certificate"
	"github.com/kolapsis/pulseboard/internal/container"
	"github.com/kolapsis/pulseboard/internal/endpoint"
	"github.com/kolapsis/pulseboard/internal/heartbeat"
	"github.com/kolapsis/pulseboard/internal/resource"
	pbruntime "github.com/kolapsis/pulseboard/internal/runtime"
	"github.com/kolapsis/pulseboard/internal/status"
	"github.com/kolapsis/pulseboard/internal/update"
	"github.com/kolapsis/pulseboard/internal/webhook"
)

// corsAllowedOrigins holds the parsed CORS allowed origins (cached at init time).
// nil means same-origin only (no CORS headers), ["*"] means wildcard.
var corsAllowedOrigins []string

// maxBodySize holds the maximum request body size in bytes for POST/PUT requests.
var maxBodySize int64 = 1048576 // 1 MB default

func init() {
	if raw := os.Getenv("PULSEBOARD_CORS_ORIGINS"); raw != "" {
		if raw == "*" {
			corsAllowedOrigins = []string{"*"}
		} else {
			parts := strings.Split(raw, ",")
			for _, p := range parts {
				if trimmed := strings.TrimSpace(p); trimmed != "" {
					corsAllowedOrigins = append(corsAllowedOrigins, trimmed)
				}
			}
		}
	}

	if raw := os.Getenv("PULSEBOARD_MAX_BODY_SIZE"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			maxBodySize = n
		}
	}
}

// ErrorResponse represents the standard JSON error format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error code and human-readable message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AlertOpts holds the alert-related dependencies for the router.
type AlertOpts struct {
	AlertStore   alert.AlertStore
	ChannelStore alert.ChannelStore
	SilenceStore alert.SilenceStore
	Notifier     *alert.Notifier
}

// StatusAdminOpts holds the status page admin dependencies for the router.
type StatusAdminOpts struct {
	Components status.ComponentStore
	StatusSvc  *status.Service
	Broker     *SSEBroker
}

// APIConfig holds the webhook dependencies for the router.
type APIConfig struct {
	WebhookStore webhook.WebhookSubscriptionStore
}

// UIConfig holds dependencies for UI redesign endpoints.
type UIConfig struct {
	UptimeDaily      UptimeDailyFetcher
	LogStreamer      LogStreamer
	ContainerSvc     *container.Service
	ResourceTopSvc   ResourceTopService
	SparklineFetcher SparklineDataFetcher
}

// buildVersion is set at startup via SetBuildVersion.
var buildVersion string

// SetBuildVersion stores the application version for the health endpoint.
func SetBuildVersion(v string) { buildVersion = v }

// Router sets up the /api/v1 route group.
type Router struct {
	mux     *http.ServeMux
	broker  *SSEBroker
	logger  *slog.Logger
	runtime pbruntime.Runtime
}

// RegisterUIRoutes registers UI redesign endpoints (daily uptime, log streaming, top resources).
func (r *Router) RegisterUIRoutes(cfg UIConfig) {
	if cfg.UptimeDaily != nil {
		udh := NewUptimeDailyHandler(cfg.UptimeDaily)
		r.mux.HandleFunc("GET /api/v1/endpoints/{id}/uptime/daily", udh.HandleEndpointDailyUptime)
		r.mux.HandleFunc("GET /api/v1/heartbeats/{id}/uptime/daily", udh.HandleHeartbeatDailyUptime)
	}

	if cfg.LogStreamer != nil && cfg.ContainerSvc != nil {
		lsh := NewLogStreamHandler(cfg.LogStreamer, cfg.ContainerSvc)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/logs/stream", lsh.HandleLogStream)
	}

	if cfg.ResourceTopSvc != nil {
		rth := NewResourceTopHandler(cfg.ResourceTopSvc)
		r.mux.HandleFunc("GET /api/v1/resources/top", rth.HandleGetTopConsumers)
	}

	if cfg.SparklineFetcher != nil {
		sh := NewSparklineHandler(cfg.SparklineFetcher)
		r.mux.HandleFunc("GET /api/v1/dashboard/sparklines", sh.HandleGetSparklines)
	}
}

// NewRouter creates a new API v1 router with SSE middleware.
func NewRouter(broker *SSEBroker, rt pbruntime.Runtime, svc *container.Service, uptime *container.UptimeCalculator, epSvc *endpoint.Service, hbSvc *heartbeat.Service, certSvc *certificate.Service, resSvc *resource.Service, logger *slog.Logger, alertOpts AlertOpts, apiCfg APIConfig, statusOpts ...StatusAdminOpts) *Router {
	r := &Router{
		mux:     http.NewServeMux(),
		broker:  broker,
		logger:  logger,
		runtime: rt,
	}

	// Webhook management
	if apiCfg.WebhookStore != nil {
		wh := NewWebhookHandler(apiCfg.WebhookStore, logger)
		r.mux.HandleFunc("GET /api/v1/webhooks", wh.HandleListWebhooks)
		r.mux.HandleFunc("POST /api/v1/webhooks", wh.HandleCreateWebhook)
		r.mux.HandleFunc("DELETE /api/v1/webhooks/{id}", wh.HandleDeleteWebhook)
		r.mux.HandleFunc("POST /api/v1/webhooks/{id}/test", wh.HandleTestWebhook)
	}

	ch := NewContainerHandler(svc, uptime)
	if cl, ok := rt.(ContainerNameLister); ok {
		ch.SetContainerNameLister(cl)
	}

	// Container REST endpoints
	r.mux.HandleFunc("GET /api/v1/containers", ch.HandleList)
	r.mux.HandleFunc("GET /api/v1/containers/{id}", ch.HandleGet)
	r.mux.HandleFunc("GET /api/v1/containers/{id}/transitions", ch.HandleTransitions)
	r.mux.HandleFunc("GET /api/v1/containers/{id}/logs", ch.HandleLogs)

	// Endpoint REST endpoints
	if epSvc != nil {
		eh := NewEndpointHandler(epSvc, svc)
		r.mux.HandleFunc("GET /api/v1/endpoints", eh.HandleListEndpoints)
		r.mux.HandleFunc("GET /api/v1/endpoints/{id}", eh.HandleGetEndpoint)
		r.mux.HandleFunc("GET /api/v1/endpoints/{id}/checks", eh.HandleListChecks)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/endpoints", eh.HandleListContainerEndpoints)
	}

	// Heartbeat REST endpoints
	if hbSvc != nil {
		hh := NewHeartbeatHandler(hbSvc)
		r.mux.HandleFunc("GET /api/v1/heartbeats", hh.HandleList)
		r.mux.HandleFunc("POST /api/v1/heartbeats", hh.HandleCreate)
		r.mux.HandleFunc("GET /api/v1/heartbeats/{id}", hh.HandleGet)
		r.mux.HandleFunc("PUT /api/v1/heartbeats/{id}", hh.HandleUpdate)
		r.mux.HandleFunc("DELETE /api/v1/heartbeats/{id}", hh.HandleDelete)
		r.mux.HandleFunc("POST /api/v1/heartbeats/{id}/pause", hh.HandlePause)
		r.mux.HandleFunc("POST /api/v1/heartbeats/{id}/resume", hh.HandleResume)
		r.mux.HandleFunc("GET /api/v1/heartbeats/{id}/executions", hh.HandleListExecutions)
		r.mux.HandleFunc("GET /api/v1/heartbeats/{id}/pings", hh.HandleListPings)

		// Public ping endpoints (top-level, no auth)
		ph := NewPingHandler(hbSvc)
		r.mux.HandleFunc("GET /ping/{uuid}/start", ph.HandleStartPing)
		r.mux.HandleFunc("POST /ping/{uuid}/start", ph.HandleStartPing)
		r.mux.HandleFunc("GET /ping/{uuid}/{exit_code}", ph.HandleExitCodePing)
		r.mux.HandleFunc("POST /ping/{uuid}/{exit_code}", ph.HandleExitCodePing)
		r.mux.HandleFunc("GET /ping/{uuid}", ph.HandlePing)
		r.mux.HandleFunc("POST /ping/{uuid}", ph.HandlePing)
	}

	// Resource monitoring endpoints
	if resSvc != nil {
		rh := NewResourceHandler(resSvc)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/resources/current", rh.HandleGetCurrent)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/resources/history", rh.HandleGetHistory)
		r.mux.HandleFunc("GET /api/v1/resources/summary", rh.HandleGetSummary)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/resources/alerts", rh.HandleGetAlertConfig)
		r.mux.HandleFunc("PUT /api/v1/containers/{id}/resources/alerts", rh.HandleUpsertAlertConfig)
	}

	// Certificate REST endpoints
	if certSvc != nil {
		certH := NewCertificateHandler(certSvc)
		r.mux.HandleFunc("GET /api/v1/certificates", certH.HandleList)
		r.mux.HandleFunc("POST /api/v1/certificates", certH.HandleCreate)
		r.mux.HandleFunc("GET /api/v1/certificates/{id}", certH.HandleGet)
		r.mux.HandleFunc("PUT /api/v1/certificates/{id}", certH.HandleUpdate)
		r.mux.HandleFunc("DELETE /api/v1/certificates/{id}", certH.HandleDelete)
		r.mux.HandleFunc("GET /api/v1/certificates/{id}/checks", certH.HandleListChecks)
	}

	// Alert engine endpoints
	if alertOpts.AlertStore != nil {
		ah := NewAlertHandler(alertOpts.AlertStore, alertOpts.ChannelStore, alertOpts.SilenceStore, alertOpts.Notifier, broker)
		// Alert history
		r.mux.HandleFunc("GET /api/v1/alerts", ah.HandleListAlerts)
		r.mux.HandleFunc("GET /api/v1/alerts/active", ah.HandleGetActiveAlerts)
		r.mux.HandleFunc("GET /api/v1/alerts/{id}", ah.HandleGetAlert)
		// Notification channels
		r.mux.HandleFunc("GET /api/v1/channels", ah.HandleListChannels)
		r.mux.HandleFunc("POST /api/v1/channels", ah.HandleCreateChannel)
		r.mux.HandleFunc("PUT /api/v1/channels/{id}", ah.HandleUpdateChannel)
		r.mux.HandleFunc("DELETE /api/v1/channels/{id}", ah.HandleDeleteChannel)
		r.mux.HandleFunc("POST /api/v1/channels/{id}/test", ah.HandleTestChannel)
		// Routing rules
		r.mux.HandleFunc("POST /api/v1/channels/{id}/rules", ah.HandleCreateRoutingRule)
		r.mux.HandleFunc("DELETE /api/v1/channels/{id}/rules/{rule_id}", ah.HandleDeleteRoutingRule)
		// Silence rules
		r.mux.HandleFunc("GET /api/v1/silence", ah.HandleListSilenceRules)
		r.mux.HandleFunc("POST /api/v1/silence", ah.HandleCreateSilenceRule)
		r.mux.HandleFunc("DELETE /api/v1/silence/{id}", ah.HandleCancelSilenceRule)
	}

	// Status page admin endpoints
	if len(statusOpts) > 0 {
		so := statusOpts[0]
		sh := NewStatusAdminHandler(so.Components, so.StatusSvc, so.Broker)
		// Component groups
		r.mux.HandleFunc("GET /api/v1/status/groups", sh.HandleListGroups)
		r.mux.HandleFunc("POST /api/v1/status/groups", sh.HandleCreateGroup)
		r.mux.HandleFunc("PUT /api/v1/status/groups/{id}", sh.HandleUpdateGroup)
		r.mux.HandleFunc("DELETE /api/v1/status/groups/{id}", sh.HandleDeleteGroup)
		// Status components
		r.mux.HandleFunc("GET /api/v1/status/components", sh.HandleListComponents)
		r.mux.HandleFunc("POST /api/v1/status/components", sh.HandleCreateComponent)
		r.mux.HandleFunc("PUT /api/v1/status/components/{id}", sh.HandleUpdateComponent)
		r.mux.HandleFunc("DELETE /api/v1/status/components/{id}", sh.HandleDeleteComponent)
	}

	// Runtime status endpoint
	r.mux.HandleFunc("GET /api/v1/runtime/status", func(w http.ResponseWriter, req *http.Request) {
		label := "Containers"
		if rt != nil && rt.Name() == "kubernetes" {
			label = "Workloads"
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"runtime":   rt.Name(),
			"connected": rt.IsConnected(),
			"label":     label,
		})
	})

	// Edition endpoint — exposes CE/Pro feature flags for frontend gating
	r.mux.HandleFunc("GET /api/v1/edition", handleGetEdition)

	// Health endpoint
	r.mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, req *http.Request) {
		resp := map[string]string{"status": "ok"}
		if buildVersion != "" {
			resp["version"] = buildVersion
		}
		WriteJSON(w, http.StatusOK, resp)
	})

	// SSE endpoint
	r.mux.Handle("GET /api/v1/containers/events", broker)

	// Wire SSE broadcasting from container service events
	svc.SetEventCallback(func(eventType string, data interface{}) {
		broker.Broadcast(SSEEvent{Type: eventType, Data: data})
	})

	// Wire SSE broadcasting from endpoint service events
	if epSvc != nil {
		epSvc.SetEventCallback(func(eventType string, data interface{}) {
			broker.Broadcast(SSEEvent{Type: eventType, Data: data})
		})
	}

	// Wire SSE broadcasting from heartbeat service events
	if hbSvc != nil {
		hbSvc.SetEventCallback(func(eventType string, data interface{}) {
			broker.Broadcast(SSEEvent{Type: eventType, Data: data})
		})
	}

	// Wire SSE broadcasting from certificate service events
	if certSvc != nil {
		certSvc.SetEventCallback(func(eventType string, data interface{}) {
			broker.Broadcast(SSEEvent{Type: eventType, Data: data})
		})
	}

	// Wire SSE broadcasting from resource service events
	if resSvc != nil {
		resSvc.SetEventCallback(func(eventType string, data interface{}) {
			broker.Broadcast(SSEEvent{Type: eventType, Data: data})
		})
	}

	return r
}

// RegisterUpdateRoutes registers update intelligence endpoints.
func (r *Router) RegisterUpdateRoutes(updateSvc *update.Service, updateStore update.UpdateStore) {
	if updateSvc == nil {
		return
	}

	uh := NewUpdateHandler(updateSvc, updateStore)
	r.mux.HandleFunc("GET /api/v1/updates", uh.HandleListUpdates)
	r.mux.HandleFunc("GET /api/v1/updates/summary", uh.HandleGetUpdateSummary)
	r.mux.HandleFunc("GET /api/v1/updates/dry-run", uh.HandleGetDryRun)
	r.mux.HandleFunc("GET /api/v1/updates/exclusions", uh.HandleListExclusions)
	r.mux.HandleFunc("POST /api/v1/updates/exclusions", uh.HandleCreateExclusion)
	r.mux.HandleFunc("DELETE /api/v1/updates/exclusions/{id}", uh.HandleDeleteExclusion)
	r.mux.HandleFunc("GET /api/v1/updates/scan/{scan_id}", uh.HandleGetScanStatus)
	r.mux.HandleFunc("POST /api/v1/updates/scan", uh.HandleTriggerScan)
	r.mux.HandleFunc("GET /api/v1/updates/container/{container_id}", uh.HandleGetContainerUpdate)
	r.mux.HandleFunc("POST /api/v1/updates/pin/{container_id}", uh.HandlePinVersion)
	r.mux.HandleFunc("DELETE /api/v1/updates/pin/{container_id}", uh.HandleUnpinVersion)

	// Wire SSE broadcasting
	updateSvc.SetEventCallback(func(eventType string, data interface{}) {
		r.broker.Broadcast(SSEEvent{Type: eventType, Data: data})
	})
}

// Handler returns the HTTP handler with CORS and body size limit middleware applied.
func (r *Router) Handler() http.Handler {
	return corsMiddleware(bodySizeLimitMiddleware(r.mux))
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// WriteError writes a standard JSON error response.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// corsMiddleware adds CORS headers based on PULSEBOARD_CORS_ORIGINS configuration.
// If unset: no CORS headers (same-origin only). If "*": wildcard. Otherwise: origin allowlist.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(corsAllowedOrigins) > 0 {
			if corsAllowedOrigins[0] == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				origin := r.Header.Get("Origin")
				for _, allowed := range corsAllowedOrigins {
					if origin == allowed {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
						break
					}
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// bodySizeLimitMiddleware limits the request body size for POST and PUT requests.
func bodySizeLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		}
		next.ServeHTTP(w, r)
	})
}

// handleGetEdition returns the current edition and Pro feature flags.
func handleGetEdition(w http.ResponseWriter, _ *http.Request) {
	isPro := pro.CurrentEdition() == pro.Pro
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"edition": string(pro.CurrentEdition()),
		"features": map[string]bool{
			"cve_enrichment":    isPro,
			"risk_scoring":      isPro,
			"changelog":         isPro,
			"incidents":         isPro,
			"maintenance_windows": isPro,
			"subscribers":       isPro,
			"smtp":              isPro,
			"alert_escalation":  isPro,
			"alert_routing":     isPro,
			"alert_templates":   isPro,
		},
	})
}
