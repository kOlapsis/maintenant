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
	"log/slog"
	"net/http"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/license"
	"github.com/kolapsis/maintenant/internal/resource"
	pbruntime "github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/update"
	"github.com/kolapsis/maintenant/internal/webhook"
)

// ErrorResponse represents the standard JSON error format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error code and human-readable message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HandlerDeps holds all dependencies needed to build the API handler.
type HandlerDeps struct {
	// Core services
	Broker       *SSEBroker
	Runtime      pbruntime.Runtime
	Containers   *container.Service
	Uptime       *container.UptimeCalculator
	Endpoints    *endpoint.Service
	Heartbeats   *heartbeat.Service
	Certificates *certificate.Service
	Resources    *resource.Service
	Logger       *slog.Logger

	// Alert pipeline
	AlertStore   alert.AlertStore
	ChannelStore alert.ChannelStore
	SilenceStore alert.SilenceStore
	Notifier     *alert.Notifier

	// Status page admin
	StatusComponents  status.ComponentStore
	StatusIncidents   status.IncidentStore
	StatusSubscribers status.SubscriberStore
	StatusMaintenance status.MaintenanceStore
	StatusSvc         *status.Service
	StatusBroker      *SSEBroker

	// Webhooks
	WebhookStore webhook.WebhookSubscriptionStore

	// UI extras
	UptimeDaily      UptimeDailyFetcher
	LogStreamer       LogStreamer
	ResourceTopSvc   ResourceTopService
	SparklineFetcher SparklineDataFetcher

	// Update intelligence
	UpdateSvc        *update.Service
	UpdateStore      update.UpdateStore
	ContainerAdapter ContainerInfoProvider

	// Security
	SecuritySvc *security.Service
	Scorer      *security.Scorer
	AckStore    security.AcknowledgmentStore

	// License
	LicenseMgr *license.LicenseManager

	// Swarm
	SwarmCluster        func() *swarm.SwarmCluster
	SwarmDiscovery      func() *swarm.ServiceDiscovery
	SwarmDetector       func() *swarm.Detector
	SwarmNodeStore      swarm.NodeStore
	SwarmUpdateTracker  *swarm.UpdateTracker
	SwarmCrashLoop      *swarm.CrashLoopDetector

	// HTTP config
	CORSOrigins      string // comma-separated origins or "*"
	MaxBodySize      int64  // 0 = 1MB default
	BuildVersion     string
	OrganisationName string
}

// Router sets up the /api/v1 route group.
type Router struct {
	mux              *http.ServeMux
	broker           *SSEBroker
	logger           *slog.Logger
	runtime          pbruntime.Runtime
	containerHandler *ContainerHandler
	corsOrigins      []string
	maxBodySize      int64
	buildVersion     string
	organisationName string
}

// NewRouter creates a new API v1 router from the unified HandlerDeps.
func NewRouter(d HandlerDeps) *Router {
	maxBody := d.MaxBodySize
	if maxBody <= 0 {
		maxBody = 1048576 // 1 MB default
	}

	r := &Router{
		mux:              http.NewServeMux(),
		broker:           d.Broker,
		logger:           d.Logger,
		runtime:          d.Runtime,
		corsOrigins:      parseCORSOrigins(d.CORSOrigins),
		maxBodySize:      maxBody,
		buildVersion:     d.BuildVersion,
		organisationName: d.OrganisationName,
	}

	// Webhook management
	if d.WebhookStore != nil {
		wh := NewWebhookHandler(d.WebhookStore, d.Logger)
		r.mux.HandleFunc("GET /api/v1/webhooks", wh.HandleListWebhooks)
		r.mux.HandleFunc("POST /api/v1/webhooks", wh.HandleCreateWebhook)
		r.mux.HandleFunc("DELETE /api/v1/webhooks/{id}", wh.HandleDeleteWebhook)
		r.mux.HandleFunc("POST /api/v1/webhooks/{id}/test", wh.HandleTestWebhook)
	}

	ch := NewContainerHandler(d.Containers, d.Uptime)
	r.containerHandler = ch
	if cl, ok := d.Runtime.(ContainerNameLister); ok {
		ch.SetContainerNameLister(cl)
	}

	// Container REST endpoints
	r.mux.HandleFunc("GET /api/v1/containers", ch.HandleList)
	r.mux.HandleFunc("GET /api/v1/containers/{id}", ch.HandleGet)
	r.mux.HandleFunc("DELETE /api/v1/containers/{id}", ch.HandleDelete)
	r.mux.HandleFunc("GET /api/v1/containers/{id}/transitions", ch.HandleTransitions)
	r.mux.HandleFunc("GET /api/v1/containers/{id}/logs", ch.HandleLogs)

	// Endpoint REST endpoints
	if d.Endpoints != nil {
		eh := NewEndpointHandler(d.Endpoints, d.Containers)
		r.mux.HandleFunc("GET /api/v1/endpoints", eh.HandleListEndpoints)
		r.mux.HandleFunc("POST /api/v1/endpoints", eh.HandleCreateEndpoint)
		r.mux.HandleFunc("GET /api/v1/endpoints/{id}", eh.HandleGetEndpoint)
		r.mux.HandleFunc("PUT /api/v1/endpoints/{id}", eh.HandleUpdateEndpoint)
		r.mux.HandleFunc("DELETE /api/v1/endpoints/{id}", eh.HandleDeleteEndpoint)
		r.mux.HandleFunc("GET /api/v1/endpoints/{id}/checks", eh.HandleListChecks)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/endpoints", eh.HandleListContainerEndpoints)
	}

	// Heartbeat REST endpoints
	if d.Heartbeats != nil {
		hh := NewHeartbeatHandler(d.Heartbeats)
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
		ph := NewPingHandler(d.Heartbeats)
		r.mux.HandleFunc("GET /ping/{uuid}/start", ph.HandleStartPing)
		r.mux.HandleFunc("POST /ping/{uuid}/start", ph.HandleStartPing)
		r.mux.HandleFunc("GET /ping/{uuid}/{exit_code}", ph.HandleExitCodePing)
		r.mux.HandleFunc("POST /ping/{uuid}/{exit_code}", ph.HandleExitCodePing)
		r.mux.HandleFunc("GET /ping/{uuid}", ph.HandlePing)
		r.mux.HandleFunc("POST /ping/{uuid}", ph.HandlePing)
	}

	// Resource monitoring endpoints
	if d.Resources != nil {
		rh := NewResourceHandler(d.Resources)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/resources/current", rh.HandleGetCurrent)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/resources/history", requireEnterprise(rh.HandleGetHistory))
		r.mux.HandleFunc("GET /api/v1/resources/summary", rh.HandleGetSummary)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/resources/alerts", rh.HandleGetAlertConfig)
		r.mux.HandleFunc("PUT /api/v1/containers/{id}/resources/alerts", rh.HandleUpsertAlertConfig)
	}

	// Certificate REST endpoints
	if d.Certificates != nil {
		certH := NewCertificateHandler(d.Certificates)
		r.mux.HandleFunc("GET /api/v1/certificates", certH.HandleList)
		r.mux.HandleFunc("POST /api/v1/certificates", certH.HandleCreate)
		r.mux.HandleFunc("GET /api/v1/certificates/{id}", certH.HandleGet)
		r.mux.HandleFunc("PUT /api/v1/certificates/{id}", certH.HandleUpdate)
		r.mux.HandleFunc("DELETE /api/v1/certificates/{id}", certH.HandleDelete)
		r.mux.HandleFunc("GET /api/v1/certificates/{id}/checks", certH.HandleListChecks)
	}

	// Alert engine endpoints
	if d.AlertStore != nil {
		ah := NewAlertHandler(d.AlertStore, d.ChannelStore, d.SilenceStore, d.Notifier, d.Broker)
		// Alert history
		r.mux.HandleFunc("GET /api/v1/alerts", ah.HandleListAlerts)
		r.mux.HandleFunc("GET /api/v1/alerts/active", ah.HandleGetActiveAlerts)
		r.mux.HandleFunc("GET /api/v1/alerts/{id}", ah.HandleGetAlert)
		r.mux.HandleFunc("POST /api/v1/alerts/{id}/acknowledge", ah.HandleAcknowledgeAlert)
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
	if d.StatusComponents != nil {
		sh := NewStatusAdminHandler(d.StatusComponents, d.StatusIncidents, d.StatusSubscribers, d.StatusMaintenance, d.StatusSvc, d.StatusBroker)
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
		// Incidents
		if d.StatusIncidents != nil {
			r.mux.HandleFunc("GET /api/v1/status/incidents", sh.HandleListIncidents)
			r.mux.HandleFunc("POST /api/v1/status/incidents", sh.HandleCreateIncident)
			r.mux.HandleFunc("PUT /api/v1/status/incidents/{id}", sh.HandleUpdateIncident)
			r.mux.HandleFunc("DELETE /api/v1/status/incidents/{id}", sh.HandleDeleteIncident)
			r.mux.HandleFunc("POST /api/v1/status/incidents/{id}/updates", sh.HandlePostUpdate)
		}
		// Maintenance windows
		if d.StatusMaintenance != nil {
			r.mux.HandleFunc("GET /api/v1/status/maintenance", sh.HandleListMaintenance)
			r.mux.HandleFunc("POST /api/v1/status/maintenance", sh.HandleCreateMaintenance)
			r.mux.HandleFunc("PUT /api/v1/status/maintenance/{id}", sh.HandleUpdateMaintenance)
			r.mux.HandleFunc("DELETE /api/v1/status/maintenance/{id}", sh.HandleDeleteMaintenance)
		}
		// Subscribers
		if d.StatusSubscribers != nil {
			r.mux.HandleFunc("GET /api/v1/status/subscribers", sh.HandleListSubscribers)
		}
		// SMTP config (Pro only)
		r.mux.HandleFunc("GET /api/v1/status/smtp", requireEnterprise(sh.HandleGetSmtpConfig))
		r.mux.HandleFunc("PUT /api/v1/status/smtp", requireEnterprise(sh.HandleUpdateSmtpConfig))
		r.mux.HandleFunc("POST /api/v1/status/smtp/test", requireEnterprise(sh.HandleTestSmtp))
	}

	// UI extras
	r.registerUIRoutes(d)

	// Runtime status endpoint
	r.mux.HandleFunc("GET /api/v1/runtime/status", func(w http.ResponseWriter, req *http.Request) {
		label := "Containers"
		if d.Runtime != nil && d.Runtime.Name() == "kubernetes" {
			label = "Workloads"
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"runtime":   d.Runtime.Name(),
			"connected": d.Runtime.IsConnected(),
			"label":     label,
		})
	})

	// Edition endpoint — exposes CE/Pro feature flags for frontend gating
	smtpConfigured := d.Notifier != nil && d.Notifier.SMTPConfigured()
	r.mux.HandleFunc("GET /api/v1/edition", r.handleGetEdition(smtpConfigured))

	// Health endpoint
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)

	// SSE endpoint
	r.mux.Handle("GET /api/v1/containers/events", d.Broker)

	// License
	r.registerLicenseRoutes(d.LicenseMgr)

	// Update intelligence
	r.registerUpdateRoutes(d)

	// Security insights
	r.registerSecurityRoutes(d)

	// Security posture
	r.registerPostureRoutes(d)

	// Swarm monitoring
	r.registerSwarmRoutes(d)

	return r
}

// registerUIRoutes registers optional UI endpoints (daily uptime, log streaming, top resources).
func (r *Router) registerUIRoutes(d HandlerDeps) {
	if d.UptimeDaily != nil {
		udh := NewUptimeDailyHandler(d.UptimeDaily)
		r.mux.HandleFunc("GET /api/v1/endpoints/{id}/uptime/daily", udh.HandleEndpointDailyUptime)
		r.mux.HandleFunc("GET /api/v1/heartbeats/{id}/uptime/daily", udh.HandleHeartbeatDailyUptime)
	}

	if d.LogStreamer != nil && d.Containers != nil {
		lsh := NewLogStreamHandler(d.LogStreamer, d.Containers)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/logs/stream", lsh.HandleLogStream)
	}

	if d.ResourceTopSvc != nil {
		rth := NewResourceTopHandler(d.ResourceTopSvc)
		r.mux.HandleFunc("GET /api/v1/resources/top", rth.HandleGetTopConsumers)
	}

	if d.SparklineFetcher != nil {
		sh := NewSparklineHandler(d.SparklineFetcher)
		r.mux.HandleFunc("GET /api/v1/dashboard/sparklines", sh.HandleGetSparklines)
	}
}

func (r *Router) registerLicenseRoutes(mgr *license.LicenseManager) {
	if mgr == nil {
		return
	}
	r.mux.HandleFunc("GET /api/v1/license/status", func(w http.ResponseWriter, req *http.Request) {
		state := mgr.State()
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status":      state.Status,
			"plan":        state.Plan,
			"message":     state.Message,
			"verified_at": state.VerifiedAt,
			"expires_at":  state.ExpiresAt,
		})
	})
}

func (r *Router) registerUpdateRoutes(d HandlerDeps) {
	if d.UpdateSvc == nil {
		return
	}

	uh := NewUpdateHandler(d.UpdateSvc, d.UpdateStore, d.ContainerAdapter)
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

	// CVE routes (edition-gated in handler)
	ch := NewCVEHandler(d.UpdateStore)
	r.mux.HandleFunc("GET /api/v1/cve", ch.HandleListCVEs)
	r.mux.HandleFunc("GET /api/v1/cve/{container_id}", ch.HandleGetContainerCVEs)

	// Risk scoring routes (Pro only)
	rh := NewRiskHandler(d.UpdateStore)
	r.mux.HandleFunc("GET /api/v1/risk", requireEnterprise(rh.HandleListRiskScores))
	r.mux.HandleFunc("GET /api/v1/risk/{container_id}", requireEnterprise(rh.HandleGetContainerRisk))
	r.mux.HandleFunc("GET /api/v1/risk/{container_id}/history", requireEnterprise(rh.HandleGetRiskHistory))
}

func (r *Router) registerSecurityRoutes(d HandlerDeps) {
	if d.SecuritySvc == nil {
		return
	}
	sh := NewSecurityHandler(d.SecuritySvc, d.Containers)
	r.mux.HandleFunc("GET /api/v1/security/insights", sh.HandleListInsights)
	r.mux.HandleFunc("GET /api/v1/security/insights/{container_id}", sh.HandleGetContainerInsights)
	r.mux.HandleFunc("GET /api/v1/security/summary", sh.HandleGetSummary)

	// Wire security provider into container handler for enriched list responses
	if r.containerHandler != nil {
		r.containerHandler.SetSecurityProvider(d.SecuritySvc)
	}
}

func (r *Router) registerPostureRoutes(d HandlerDeps) {
	if d.Scorer == nil {
		return
	}
	ph := NewPostureHandler(d.Scorer, d.Containers, d.AckStore)

	// Posture endpoints
	r.mux.HandleFunc("GET /api/v1/security/posture", requireEnterprise(ph.HandleGetPosture))
	r.mux.HandleFunc("GET /api/v1/security/posture/containers", requireEnterprise(ph.HandleListContainerPostures))
	r.mux.HandleFunc("GET /api/v1/security/posture/containers/{container_id}", requireEnterprise(ph.HandleGetContainerPosture))

	// Acknowledgment endpoints
	r.mux.HandleFunc("POST /api/v1/security/acknowledgments", requireEnterprise(ph.HandleCreateAcknowledgment))
	r.mux.HandleFunc("GET /api/v1/security/acknowledgments", requireEnterprise(ph.HandleListAcknowledgments))
	r.mux.HandleFunc("DELETE /api/v1/security/acknowledgments/{id}", requireEnterprise(ph.HandleDeleteAcknowledgment))
}

func (r *Router) registerSwarmRoutes(d HandlerDeps) {
	if d.SwarmCluster == nil {
		return
	}
	sh := NewSwarmHandler(d.SwarmCluster, d.SwarmDiscovery, d.SwarmDetector, d.SwarmNodeStore, d.SwarmUpdateTracker, d.SwarmCrashLoop)

	// CE endpoints
	r.mux.HandleFunc("GET /api/v1/swarm/info", sh.HandleGetInfo)
	r.mux.HandleFunc("GET /api/v1/swarm/services", sh.HandleListServices)
	r.mux.HandleFunc("GET /api/v1/swarm/services/{serviceID}", sh.HandleGetService)

	// Enterprise node endpoints
	r.mux.HandleFunc("GET /api/v1/swarm/nodes", requireEnterprise(sh.HandleListNodes))
	r.mux.HandleFunc("GET /api/v1/swarm/nodes/{nodeID}", requireEnterprise(sh.HandleGetNodeDetail))

	// Enterprise update status and dashboard endpoints
	r.mux.HandleFunc("GET /api/v1/swarm/services/{serviceID}/update-status", requireEnterprise(sh.HandleGetUpdateStatus))
	r.mux.HandleFunc("GET /api/v1/swarm/dashboard", requireEnterprise(sh.HandleGetDashboard))
}

// Handler returns the HTTP handler with the full middleware chain applied.
// Middleware order (outermost to innermost): panicRecovery → requestLogger → requestID → cors → bodyLimit → mux
func (r *Router) Handler() http.Handler {
	var h http.Handler = r.mux
	h = bodyLimit(r.maxBodySize, h)
	h = cors(r.corsOrigins, h)
	h = requestID(h)
	h = requestLogger(h, r.logger)
	h = panicRecovery(h, r.logger)
	return h
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

// handleHealth returns the health check response.
func (r *Router) handleHealth(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]string{"status": "ok"}
	if r.buildVersion != "" {
		resp["version"] = r.buildVersion
	}
	WriteJSON(w, http.StatusOK, resp)
}

// handleGetEdition returns a handler for the current edition and feature flags.
func (r *Router) handleGetEdition(smtpConfigured bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		isEnterprise := extension.CurrentEdition() == extension.Enterprise
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"edition":           string(extension.CurrentEdition()),
			"organisation_name": r.organisationName,
			"features": map[string]bool{
				"cve_enrichment":      isEnterprise,
				"risk_scoring":        isEnterprise,
				"changelog":           isEnterprise,
				"incidents":           isEnterprise,
				"maintenance_windows": isEnterprise,
				"subscribers":         isEnterprise,
				"smtp":                smtpConfigured && isEnterprise,
				"slack":               isEnterprise,
				"teams":               isEnterprise,
				"resource_history":    isEnterprise,
				"alert_escalation":    isEnterprise,
				"alert_routing":       true,
				"alert_entity_routing": isEnterprise,
				"security_posture":    isEnterprise,
				"swarm_dashboard":     isEnterprise,
			},
		})
	}
}
