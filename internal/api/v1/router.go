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
	"log/slog"
	"net/http"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/license"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/update"
	"github.com/kolapsis/maintenant/internal/webhook"
)

// AgentSessions provides access to live agent stream state.
type AgentSessions interface {
	IsConnected(agentID string) bool
	Close(agentID, reason string)
}

// ErrorResponse represents the standard JSON error format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error code and human-readable message.
// ErrorDetail is the body of an error response. The four optional fields carry
// what an edition refusal needs so the interface can build its message and its
// upgrade link without parsing the text. Every other API error keeps the shape
// it has always had — the fields are omitted when empty.
type ErrorDetail struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	Feature         string `json:"feature,omitempty"`          // EDITION_REQUIRED
	Resource        string `json:"resource,omitempty"`         // QUOTA_EXCEEDED, HOST_LIMIT_REACHED
	Limit           *int   `json:"limit,omitempty"`            // QUOTA_EXCEEDED, HOST_LIMIT_REACHED
	RequiredEdition string `json:"required_edition,omitempty"` // both
}

// HandlerDeps holds all dependencies needed to build the API handler.
type HandlerDeps struct {
	// Core services
	Broker       *SSEBroker
	Runtime      runtime.Runtime
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
	TriggerStore alert.TriggerStore
	SilenceStore alert.SilenceStore
	Notifier     *alert.Notifier
	Escalator    alert.Escalator

	// Status page admin
	StatusComponents   status.ComponentStore
	StatusIncidents    status.IncidentStore
	StatusSubscribers  status.SubscriberStore
	StatusMaintenance  status.MaintenanceStore
	StatusSvc          *status.Service
	StatusBroker       *SSEBroker
	PersonalizationSvc *status.PersonalizationService

	// Webhooks
	WebhookStore webhook.WebhookSubscriptionStore

	// UI extras
	UptimeDaily      UptimeDailyFetcher
	LogStreamer      LogStreamer
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

	// Escalation policies
	EscalationSvc *escalation.Service

	// License
	LicenseMgr *license.Manager

	// Swarm
	SwarmCluster        func() *swarm.SwarmCluster
	SwarmDiscovery      func() *swarm.ServiceDiscovery
	SwarmDetector       func() *swarm.Detector
	SwarmNodeStore      swarm.NodeStore
	SwarmUpdateTracker  *swarm.UpdateTracker
	SwarmCrashLoop      *swarm.CrashLoopDetector
	SwarmReplicaChecker *swarm.ReplicaHealthChecker
	SwarmTopologyStore  *store.SwarmTopologyStore

	// Kubernetes (per-agent store-backed reads)
	KubernetesStore *store.KubernetesStore

	// Multi-host agents (Pro)
	AgentStore          *store.AgentStore
	AgentSessions       AgentSessions
	GRPCPublicURL       string
	GRPCListen          string
	AgentStaleThreshold time.Duration

	// HTTP config
	CORSOrigins          string // comma-separated origins or "*"
	MaxBodySize          int64  // 0 = 1MB default
	BuildVersion         string
	OrganisationName     string
	AllowPrivateWebhooks bool // dev only: skip HTTPS + SSRF check on webhook URLs
	StatusURL            string
}

// agentStoreDirectory adapts the sqlite agent store to AgentDirectory so the
// container handler can resolve agent_id → hostname/label for remote containers.
type agentStoreDirectory struct {
	store *store.AgentStore
}

func (d agentStoreDirectory) AgentNames(ctx context.Context) (map[string]AgentName, error) {
	agents, err := d.store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	m := make(map[string]AgentName, len(agents))
	for _, a := range agents {
		m[a.AgentID] = AgentName{Hostname: a.Hostname, Label: a.Label}
	}
	return m, nil
}

// Router sets up the /api/v1 route group.
type Router struct {
	mux              *http.ServeMux
	broker           *SSEBroker
	logger           *slog.Logger
	runtime          runtime.Runtime
	containerHandler *ContainerHandler
	corsOrigins      []string
	maxBodySize      int64
	buildVersion     string
	organisationName string
	statusURL        string
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
		statusURL:        d.StatusURL,
	}

	// Webhook management
	if d.WebhookStore != nil {
		wh := NewWebhookHandler(d.WebhookStore, d.Logger, d.AllowPrivateWebhooks)
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
	if d.Runtime != nil {
		ch.SetRuntimeChecker(d.Runtime)
		ch.SetLogFetcher(d.Runtime)
	}
	if d.AgentStore != nil {
		ch.SetAgentDirectory(agentStoreDirectory{store: d.AgentStore})
	}
	if d.AgentSessions != nil {
		ch.SetAgentSessions(d.AgentSessions)
		if lr, ok := d.AgentSessions.(AgentLogRequester); ok {
			ch.SetLogRequester(lr)
		}
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
		if d.AgentSessions != nil {
			eh.SetAgentSessions(d.AgentSessions)
		}
		r.mux.HandleFunc("GET /api/v1/endpoints", eh.HandleListEndpoints)
		r.mux.HandleFunc("POST /api/v1/endpoints", eh.HandleCreateEndpoint)
		r.mux.HandleFunc("GET /api/v1/endpoints/{id}", eh.HandleGetEndpoint)
		r.mux.HandleFunc("PUT /api/v1/endpoints/{id}", eh.HandleUpdateEndpoint)
		r.mux.HandleFunc("DELETE /api/v1/endpoints/{id}", eh.HandleDeleteEndpoint)
		r.mux.HandleFunc("GET /api/v1/endpoints/{id}/checks", eh.HandleListChecks)
		r.mux.HandleFunc("POST /api/v1/endpoints/{id}/check", eh.HandleCheckNow)
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
		if d.AgentStore != nil {
			rh.SetAgentDirectory(agentStoreDirectory{store: d.AgentStore})
		}
		r.mux.HandleFunc("GET /api/v1/containers/{id}/resources/current", rh.HandleGetCurrent)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/resources/history", requireCapability(extension.CapResourceHistory, rh.HandleGetHistory))
		r.mux.HandleFunc("GET /api/v1/resources/summary", rh.HandleGetSummary)
		r.mux.HandleFunc("GET /api/v1/resources/hosts", rh.HandleGetHosts)
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
		r.mux.HandleFunc("POST /api/v1/certificates/{id}/check", certH.HandleCheckNow)
	}

	// Alert engine endpoints
	if d.AlertStore != nil {
		ah := NewAlertHandler(d.AlertStore, d.ChannelStore, d.SilenceStore, d.Notifier, d.Broker, d.AllowPrivateWebhooks, d.Escalator)
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
		// Alert triggers (CRUD; advanced filters scopes/tags gated to Pro inside the handler)
		if d.TriggerStore != nil {
			th := NewAlertTriggerHandler(d.TriggerStore, d.ChannelStore, d.Broker)
			r.mux.HandleFunc("GET /api/v1/alert-triggers", th.HandleListTriggers)
			r.mux.HandleFunc("POST /api/v1/alert-triggers", th.HandleCreateTrigger)
			r.mux.HandleFunc("GET /api/v1/alert-triggers/{id}", th.HandleGetTrigger)
			r.mux.HandleFunc("PUT /api/v1/alert-triggers/{id}", th.HandleUpdateTrigger)
			r.mux.HandleFunc("DELETE /api/v1/alert-triggers/{id}", th.HandleDeleteTrigger)
		}
		// Silence rules
		r.mux.HandleFunc("GET /api/v1/silence", ah.HandleListSilenceRules)
		r.mux.HandleFunc("POST /api/v1/silence", ah.HandleCreateSilenceRule)
		r.mux.HandleFunc("DELETE /api/v1/silence/{id}", ah.HandleCancelSilenceRule)
	}

	// Status page admin endpoints
	if d.StatusComponents != nil {
		sh := NewStatusAdminHandler(d.StatusComponents, d.StatusIncidents, d.StatusSubscribers, d.StatusMaintenance, d.StatusSvc, d.StatusBroker)
		// Status components
		r.mux.HandleFunc("GET /api/v1/status/components", sh.HandleListComponents)
		r.mux.HandleFunc("POST /api/v1/status/components", sh.HandleCreateComponent)
		r.mux.HandleFunc("PUT /api/v1/status/components/{id}", sh.HandleUpdateComponent)
		r.mux.HandleFunc("DELETE /api/v1/status/components/{id}", sh.HandleDeleteComponent)
		// Incidents
		if d.StatusIncidents != nil {
			r.mux.HandleFunc("GET /api/v1/status/incidents", sh.HandleListIncidents)
			r.mux.HandleFunc("POST /api/v1/status/incidents", requireCapability(extension.CapIncidents, sh.HandleCreateIncident))
			r.mux.HandleFunc("PUT /api/v1/status/incidents/{id}", requireCapability(extension.CapIncidents, sh.HandleUpdateIncident))
			r.mux.HandleFunc("DELETE /api/v1/status/incidents/{id}", requireCapability(extension.CapIncidents, sh.HandleDeleteIncident))
			r.mux.HandleFunc("POST /api/v1/status/incidents/{id}/updates", requireCapability(extension.CapIncidents, sh.HandlePostUpdate))
		}
		// Maintenance windows
		if d.StatusMaintenance != nil {
			r.mux.HandleFunc("GET /api/v1/status/maintenance", sh.HandleListMaintenance)
			r.mux.HandleFunc("POST /api/v1/status/maintenance", requireCapability(extension.CapMaintenanceWindows, sh.HandleCreateMaintenance))
			r.mux.HandleFunc("PUT /api/v1/status/maintenance/{id}", requireCapability(extension.CapMaintenanceWindows, sh.HandleUpdateMaintenance))
			r.mux.HandleFunc("DELETE /api/v1/status/maintenance/{id}", requireCapability(extension.CapMaintenanceWindows, sh.HandleDeleteMaintenance))
		}
		// Subscribers
		if d.StatusSubscribers != nil {
			r.mux.HandleFunc("GET /api/v1/status/subscribers", requireCapability(extension.CapSubscribers, sh.HandleListSubscribers))
		}
		// SMTP config (Pro only)
		r.mux.HandleFunc("GET /api/v1/status/smtp", requireCapability(extension.CapSMTP, sh.HandleGetSmtpConfig))
		r.mux.HandleFunc("PUT /api/v1/status/smtp", requireCapability(extension.CapSMTP, sh.HandleUpdateSmtpConfig))
		r.mux.HandleFunc("POST /api/v1/status/smtp/test", requireCapability(extension.CapSMTP, sh.HandleTestSmtp))
	}

	// Status page personalization (Pro only)
	if d.PersonalizationSvc != nil {
		ph := NewPersonalizationHandler(d.PersonalizationSvc)
		// Settings
		r.mux.HandleFunc("GET /api/v1/status-page/settings", requireCapability(extension.CapPersonalization, ph.HandleGetSettings))
		r.mux.HandleFunc("PUT /api/v1/status-page/settings", requireCapability(extension.CapPersonalization, ph.HandlePutSettings))
		// Assets
		r.mux.HandleFunc("PUT /api/v1/status-page/assets/{role}", requireCapability(extension.CapPersonalization, ph.HandlePutAsset))
		r.mux.HandleFunc("GET /api/v1/status-page/assets/{role}", requireCapability(extension.CapPersonalization, ph.HandleGetAsset))
		r.mux.HandleFunc("DELETE /api/v1/status-page/assets/{role}", requireCapability(extension.CapPersonalization, ph.HandleDeleteAsset))
		// Footer links
		r.mux.HandleFunc("GET /api/v1/status-page/footer-links", requireCapability(extension.CapPersonalization, ph.HandleListFooterLinks))
		r.mux.HandleFunc("POST /api/v1/status-page/footer-links", requireCapability(extension.CapPersonalization, ph.HandleCreateFooterLink))
		r.mux.HandleFunc("PUT /api/v1/status-page/footer-links/order", requireCapability(extension.CapPersonalization, ph.HandleReorderFooterLinks))
		r.mux.HandleFunc("PUT /api/v1/status-page/footer-links/{id}", requireCapability(extension.CapPersonalization, ph.HandleUpdateFooterLink))
		r.mux.HandleFunc("DELETE /api/v1/status-page/footer-links/{id}", requireCapability(extension.CapPersonalization, ph.HandleDeleteFooterLink))
		// FAQ
		r.mux.HandleFunc("GET /api/v1/status-page/faq", requireCapability(extension.CapPersonalization, ph.HandleListFAQ))
		r.mux.HandleFunc("POST /api/v1/status-page/faq", requireCapability(extension.CapPersonalization, ph.HandleCreateFAQItem))
		r.mux.HandleFunc("PUT /api/v1/status-page/faq/order", requireCapability(extension.CapPersonalization, ph.HandleReorderFAQ))
		r.mux.HandleFunc("PUT /api/v1/status-page/faq/{id}", requireCapability(extension.CapPersonalization, ph.HandleUpdateFAQItem))
		r.mux.HandleFunc("DELETE /api/v1/status-page/faq/{id}", requireCapability(extension.CapPersonalization, ph.HandleDeleteFAQItem))
	}

	// Escalation policies
	r.registerEscalationRoutes(d)

	// UI extras
	r.registerUIRoutes(d)

	// Runtime status endpoint
	r.mux.HandleFunc("GET /api/v1/runtime/status", r.handleRuntimeStatus(d))

	// Edition endpoint — exposes CE/Pro feature flags for frontend gating
	smtpConfigured := d.Notifier != nil && d.Notifier.SMTPConfigured()
	r.mux.HandleFunc("GET /api/v1/edition", r.handleGetEdition(smtpConfigured, d))

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

	// Kubernetes monitoring
	r.registerKubernetesRoutes(d)

	// Multi-host agents (Pro)
	r.registerAgentRoutes(d)

	return r
}

// handleRuntimeStatus reports the active container runtime context (docker,
// swarm, or kubernetes) and runtime-specific metadata. For a Swarm manager it
// includes service_count so the dashboard can keep the Docker "Containers" view
// available while no service is deployed.
func (r *Router) handleRuntimeStatus(d HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		runtimeName := d.Runtime.Name()
		ctx := "docker"
		label := "Containers"
		metadata := map[string]interface{}{}

		switch runtimeName {
		case "kubernetes":
			ctx = "kubernetes"
			label = "Workloads"
			metadata["namespace_count"] = 0
			metadata["node_count"] = 0
		default:
			// Check for Swarm mode.
			if detector := d.SwarmDetector(); detector != nil {
				result := detector.Result()
				if result.Active && result.IsManager {
					ctx = "swarm"
					label = "Services"

					metadata["cluster_id"] = result.ClusterID
					metadata["is_manager"] = true
					if cluster := d.SwarmCluster(); cluster != nil {
						metadata["manager_count"] = cluster.ManagerCount
						metadata["worker_count"] = cluster.WorkerCount
					} else {
						metadata["manager_count"] = 0
						metadata["worker_count"] = 0
					}

					// Expose the deployed-service count so the dashboard can keep the
					// Docker "Containers" view available on an empty Swarm manager.
					metadata["service_count"] = 0
					if disc := d.SwarmDiscovery(); disc != nil {
						metadata["service_count"] = len(disc.ListServices())
					}
				}
			}
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"runtime":     runtimeName,
			"context":     ctx,
			"connected":   d.Runtime.IsConnected(),
			"label":       label,
			"detected_at": time.Now().UTC().Format(time.RFC3339),
			"metadata":    metadata,
		})
	}
}

// registerEscalationRoutes registers escalation policy CRUD endpoints (Pro only).
// The overlap-probe route must be registered before {id} to avoid path ambiguity.
func (r *Router) registerEscalationRoutes(d HandlerDeps) {
	if d.EscalationSvc == nil {
		return
	}
	eh := NewEscalationHandler(d.EscalationSvc)
	r.mux.HandleFunc("POST /api/v1/escalation-policies", requireCapability(extension.CapAlertEscalation, eh.HandleCreatePolicy))
	r.mux.HandleFunc("GET /api/v1/escalation-policies", requireCapability(extension.CapAlertEscalation, eh.HandleListPolicies))
	// overlap-probe must be registered before /{id} to avoid the wildcard matching the literal segment.
	r.mux.HandleFunc("POST /api/v1/escalation-policies/overlap-probe", requireCapability(extension.CapAlertEscalation, eh.HandleOverlapProbe))
	r.mux.HandleFunc("GET /api/v1/escalation-policies/{id}", requireCapability(extension.CapAlertEscalation, eh.HandleGetPolicy))
	r.mux.HandleFunc("DELETE /api/v1/escalation-policies/{id}", requireCapability(extension.CapAlertEscalation, eh.HandleDeletePolicy))
	r.mux.HandleFunc("PUT /api/v1/escalation-policies/{id}", requireCapability(extension.CapAlertEscalation, eh.HandleUpdatePolicy))
	r.mux.HandleFunc("PATCH /api/v1/escalation-policies/{id}/active", requireCapability(extension.CapAlertEscalation, eh.HandleSetPolicyActive))
	r.mux.HandleFunc("GET /api/v1/escalation-policies/{id}/runs", requireCapability(extension.CapAlertEscalation, eh.HandleListPolicyRuns))
	r.mux.HandleFunc("GET /api/v1/escalation-runs/{run_id}", requireCapability(extension.CapAlertEscalation, eh.HandleGetRun))
	r.mux.HandleFunc("GET /api/v1/alerts/{alert_id}/escalation-runs", requireCapability(extension.CapAlertEscalation, eh.HandleListAlertRuns))
}

// registerUIRoutes registers optional UI endpoints (daily uptime, log streaming, top resources).
func (r *Router) registerUIRoutes(d HandlerDeps) {
	if d.UptimeDaily != nil {
		udh := NewUptimeDailyHandler(d.UptimeDaily)
		r.mux.HandleFunc("GET /api/v1/endpoints/{id}/uptime/daily", udh.HandleEndpointDailyUptime)
		r.mux.HandleFunc("GET /api/v1/heartbeats/{id}/uptime/daily", udh.HandleHeartbeatDailyUptime)
		r.mux.HandleFunc("GET /api/v1/containers/{id}/uptime/daily", udh.HandleContainerDailyUptime)
	}

	// Registered when either source of logs exists: a server with no local runtime
	// can still follow the logs of its agents' containers.
	logRequester, _ := d.AgentSessions.(AgentLogRequester)
	if d.Containers != nil && (d.LogStreamer != nil || logRequester != nil) {
		lsh := NewLogStreamHandler(d.LogStreamer, d.Containers)
		if d.Runtime != nil {
			lsh.SetRuntimeChecker(d.Runtime)
		}
		if logRequester != nil {
			lsh.SetLogRequester(logRequester)
		}
		if d.AgentStore != nil {
			lsh.SetAgentDirectory(agentStoreDirectory{store: d.AgentStore})
		}
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

// rfc3339OrEmpty renders a date the license may not carry. A perpetual license
// has no end date, and an instance outside no update window has neither of the
// two window dates. Serialising the zero time would emit year 1, which reads as
// a deadline long past — an empty string is the only value that cannot be
// mistaken for one.
func rfc3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// licenseStatusPayload is the wire shape of GET /api/v1/license/status. Every
// branch goes through it so the response always carries the same keys.
func licenseStatusPayload(state *license.State, edition extension.Edition) map[string]interface{} {
	return map[string]interface{}{
		"status":             state.Status,
		"edition":            string(edition),
		"plan":               state.Plan,
		"message":            state.Message,
		"verified_at":        rfc3339OrEmpty(state.VerifiedAt),
		"expires_at":         rfc3339OrEmpty(state.ExpiresAt),
		"updates_until":      rfc3339OrEmpty(state.UpdatesUntil),
		"update_grace_until": rfc3339OrEmpty(state.UpdateGraceUntil),
	}
}

func (r *Router) registerLicenseRoutes(mgr *license.Manager) {
	r.mux.HandleFunc("GET /api/v1/license/status", func(w http.ResponseWriter, req *http.Request) {
		// No license configured (Community): report an inactive state rather than
		// leaving the documented route unregistered (404).
		if mgr == nil {
			WriteJSON(w, http.StatusOK,
				licenseStatusPayload(&license.State{Status: "inactive"}, extension.Community))
			return
		}

		WriteJSON(w, http.StatusOK, licenseStatusPayload(mgr.State(), mgr.Edition()))
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
	// {container_id...} captures multiple path segments, required for Kubernetes ExternalIDs
	// which contain slashes (e.g. "default/Deployment/redis").
	r.mux.HandleFunc("GET /api/v1/updates/container/{container_id...}", uh.HandleGetContainerUpdate)
	r.mux.HandleFunc("POST /api/v1/updates/pin/{container_id...}", uh.HandlePinVersion)
	r.mux.HandleFunc("DELETE /api/v1/updates/pin/{container_id...}", uh.HandleUnpinVersion)

	// CVE routes (edition-gated in handler)
	ch := NewCVEHandler(d.UpdateStore)
	r.mux.HandleFunc("GET /api/v1/cve", ch.HandleListCVEs)
	r.mux.HandleFunc("GET /api/v1/cve/{container_id...}", ch.HandleGetContainerCVEs)

	// Risk scoring routes (Pro only)
	// Note: history uses a query param (?history=1) to avoid conflict with {container_id...}.
	rh := NewRiskHandler(d.UpdateStore)
	r.mux.HandleFunc("GET /api/v1/risk", requireCapability(extension.CapRiskScoring, rh.HandleListRiskScores))
	r.mux.HandleFunc("GET /api/v1/risk/{container_id...}", requireCapability(extension.CapRiskScoring, rh.HandleGetContainerRisk))
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
	ph := NewPostureHandler(d.Scorer, d.Containers, d.AckStore, d.AlertStore, d.SecuritySvc, d.Broker)

	// Posture endpoints
	r.mux.HandleFunc("GET /api/v1/security/posture", requireCapability(extension.CapSecurityPosture, ph.HandleGetPosture))
	r.mux.HandleFunc("GET /api/v1/security/posture/containers", requireCapability(extension.CapSecurityPosture, ph.HandleListContainerPostures))
	r.mux.HandleFunc("GET /api/v1/security/posture/containers/{container_id}", requireCapability(extension.CapSecurityPosture, ph.HandleGetContainerPosture))

	// Acknowledgment endpoints
	r.mux.HandleFunc("POST /api/v1/security/acknowledgments", requireCapability(extension.CapSecurityPosture, ph.HandleCreateAcknowledgment))
	r.mux.HandleFunc("GET /api/v1/security/acknowledgments", requireCapability(extension.CapSecurityPosture, ph.HandleListAcknowledgments))
	r.mux.HandleFunc("DELETE /api/v1/security/acknowledgments/{id}", requireCapability(extension.CapSecurityPosture, ph.HandleDeleteAcknowledgment))
}

func (r *Router) registerKubernetesRoutes(d HandlerDeps) {
	if d.KubernetesStore == nil {
		return
	}

	// Reads are store-backed (per-agent), so the routes are mounted regardless of
	// the server's own runtime. Live metrics-server data is only available when
	// the server itself runs on Kubernetes; expose it for the local scope.
	var metrics K8sMetricsProvider
	if d.Runtime != nil && d.Runtime.Name() == "kubernetes" {
		if mp, ok := d.Runtime.(K8sMetricsProvider); ok {
			metrics = mp
		}
	}

	kh := NewKubernetesHandler(d.KubernetesStore, metrics)
	if d.AgentStore != nil {
		kh.SetAgentDirectory(agentStoreDirectory{store: d.AgentStore})
	}
	if d.AgentSessions != nil {
		kh.SetAgentSessions(d.AgentSessions)
	}

	// CE endpoints
	r.mux.HandleFunc("GET /api/v1/kubernetes/namespaces", kh.HandleListNamespaces)
	r.mux.HandleFunc("GET /api/v1/kubernetes/workloads", kh.HandleListWorkloads)
	r.mux.HandleFunc("GET /api/v1/kubernetes/workloads/{id}", kh.HandleGetWorkload)
	r.mux.HandleFunc("GET /api/v1/kubernetes/pods", kh.HandleListPods)
	r.mux.HandleFunc("GET /api/v1/kubernetes/pods/{namespace}/{name}", kh.HandleGetPodDetail)

	// Pro endpoints
	r.mux.HandleFunc("GET /api/v1/kubernetes/nodes", kh.HandleListNodes)
	r.mux.HandleFunc("GET /api/v1/kubernetes/nodes/{name}/resources", kh.HandleGetNodeResources)
	r.mux.HandleFunc("GET /api/v1/kubernetes/workloads/{id}/resources", kh.HandleGetWorkloadResources)
	r.mux.HandleFunc("GET /api/v1/kubernetes/cluster", kh.HandleGetCluster)
}

func (r *Router) registerSwarmRoutes(d HandlerDeps) {
	// Mounted unconditionally: the services/tasks/nodes lists are store-backed
	// (per-agent), so they work even when this server is not itself a swarm
	// manager. The live Pro dashboards guard internally on the local
	// runtime being a swarm cluster.
	if d.SwarmTopologyStore == nil {
		return
	}
	sh := NewSwarmHandler(d.SwarmCluster, d.SwarmDiscovery, d.SwarmDetector, d.SwarmTopologyStore, d.SwarmNodeStore, d.SwarmUpdateTracker, d.SwarmCrashLoop, d.SwarmReplicaChecker, d.Containers, d.Resources)
	if d.AgentStore != nil {
		sh.SetAgentDirectory(agentStoreDirectory{store: d.AgentStore})
	}

	// CE endpoints
	r.mux.HandleFunc("GET /api/v1/swarm/info", sh.HandleGetInfo)
	r.mux.HandleFunc("GET /api/v1/swarm/services", sh.HandleListServices)
	r.mux.HandleFunc("GET /api/v1/swarm/services/{serviceID}", sh.HandleGetService)
	r.mux.HandleFunc("GET /api/v1/swarm/tasks", sh.HandleListTasks)

	// Pro node endpoints
	r.mux.HandleFunc("GET /api/v1/swarm/nodes", sh.HandleListNodes)
	r.mux.HandleFunc("GET /api/v1/swarm/nodes/{nodeID}", sh.HandleGetNodeDetail)

	// Pro update status, resources, dashboard, and cluster endpoints
	r.mux.HandleFunc("GET /api/v1/swarm/services/{serviceID}/update-status", sh.HandleGetUpdateStatus)
	r.mux.HandleFunc("GET /api/v1/swarm/services/{serviceID}/resources", sh.HandleGetServiceResources)
	r.mux.HandleFunc("GET /api/v1/swarm/dashboard", sh.HandleGetDashboard)
	r.mux.HandleFunc("GET /api/v1/swarm/cluster", sh.HandleGetCluster)
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
	_ = json.NewEncoder(w).Encode(v)
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

// WriteErrorDetail writes an error carrying the structured fields — the edition
// refusals and the quota refusals. Plain errors keep using WriteError.
func WriteErrorDetail(w http.ResponseWriter, status int, detail ErrorDetail) {
	WriteJSON(w, status, ErrorResponse{Error: detail})
}

// handleHealth returns the health check response.
func (r *Router) handleHealth(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]interface{}{
		"status": "ok",
	}
	if r.buildVersion != "" {
		resp["version"] = r.buildVersion
	}
	if r.runtime != nil {
		resp["runtime"] = map[string]interface{}{
			"name":      r.runtime.Name(),
			"connected": r.runtime.IsConnected(),
		}
	}
	WriteJSON(w, http.StatusOK, resp)
}

// handleGetEdition returns a handler for the current edition and feature flags.
func (r *Router) handleGetEdition(smtpConfigured bool, d HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		// Both objects are projected from the same registry, so they carry
		// exactly the same keys and cannot disagree with the middleware.
		catalog := extension.Catalog()
		features := make(map[string]bool, len(catalog))
		featureEditions := make(map[string]string, len(catalog))
		for c, min := range catalog {
			features[string(c)] = extension.Allows(c)
			featureEditions[string(c)] = string(min)
		}

		// smtp is the one flag that is not purely an edition decision: the
		// capability says what the edition permits, the configuration says what
		// is ready. The two refusals must stay distinguishable (FR-015).
		features[string(extension.CapSMTP)] = extension.Allows(extension.CapSMTP) && smtpConfigured

		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"edition":           string(extension.CurrentEdition()),
			"organisation_name": r.organisationName,
			"status_url":        r.statusURL,
			"features":          features,
			"feature_editions":  featureEditions,
			"quotas":            r.computeQuotas(ctx, d),
		})
	}
}

// computeQuotas returns real usage and the applicable limit for every capped
// resource, in every edition. A limit of -1 means unlimited.
func (r *Router) computeQuotas(ctx context.Context, d HandlerDeps) map[string]interface{} {
	quotas := make(map[string]interface{})

	// Usage is counted in every edition. It used to be short-circuited to 0 on
	// Pro, which reported "0 endpoints" to an instance running fifty of them.
	// Limits all come from extension.Limit, never from a literal here.

	// Agent hosts. The local runtime is never counted (CountByStatus excludes it).
	if d.AgentStore != nil {
		used, _, err := d.AgentStore.CountByStatus(ctx)
		if err != nil {
			r.logger.Error("failed to count agent hosts for quota", "error", err)
			used = 0
		}
		quotas["agent_hosts"] = map[string]interface{}{
			"used":  used,
			"limit": extension.Limit(extension.ResourceAgentHosts),
		}
	}

	if d.Endpoints != nil {
		used, err := d.Endpoints.CountActiveEndpoints(ctx)
		if err != nil {
			r.logger.Error("failed to count endpoints for quota", "error", err)
			used = 0
		}
		quotas["endpoints"] = map[string]interface{}{
			"used":  used,
			"limit": extension.Limit(extension.ResourceEndpoints),
		}
	}

	if d.Heartbeats != nil {
		used, err := d.Heartbeats.CountActiveHeartbeats(ctx)
		if err != nil {
			r.logger.Error("failed to count heartbeats for quota", "error", err)
			used = 0
		}
		quotas["heartbeats"] = map[string]interface{}{
			"used":  used,
			"limit": extension.Limit(extension.ResourceHeartbeats),
		}
	}

	// Certificates: standalone monitors only.
	if d.Certificates != nil {
		used, err := d.Certificates.CountStandaloneMonitors(ctx)
		if err != nil {
			r.logger.Error("failed to count standalone certificates for quota", "error", err)
			used = 0
		}
		quotas["certificates"] = map[string]interface{}{
			"used":  used,
			"limit": extension.Limit(extension.ResourceCertificates),
		}
	}

	if d.StatusComponents != nil {
		components, err := d.StatusComponents.ListComponents(ctx)
		if err != nil {
			r.logger.Error("failed to count status components for quota", "error", err)
			components = nil
		}
		quotas["status_components"] = map[string]interface{}{
			"used":  len(components),
			"limit": extension.Limit(extension.ResourceStatusComponents),
		}
	}

	return quotas
}

func (r *Router) registerAgentRoutes(d HandlerDeps) {
	if d.AgentStore == nil {
		return
	}
	staleThreshold := d.AgentStaleThreshold
	if staleThreshold == 0 {
		staleThreshold = 60 * time.Second
	}
	ah := NewAgentHandler(d.AgentStore, d.AgentSessions, d.Broker, d.Logger, d.GRPCPublicURL, d.GRPCListen, staleThreshold)

	// Enrollment token endpoints (order matters: specific paths before wildcards)
	r.mux.HandleFunc("GET /api/v1/agents/metrics", requireCapability(extension.CapMultihost, ah.HandleGetAgentMetrics))
	r.mux.HandleFunc("POST /api/v1/agents/enrollment-tokens", requireCapability(extension.CapMultihost, ah.HandleCreateEnrollmentToken))
	r.mux.HandleFunc("GET /api/v1/agents/enrollment-tokens", requireCapability(extension.CapMultihost, ah.HandleListEnrollmentTokens))
	r.mux.HandleFunc("GET /api/v1/agents/enrollment-tokens/{token_id}", requireCapability(extension.CapMultihost, ah.HandleGetEnrollmentToken))
	r.mux.HandleFunc("DELETE /api/v1/agents/enrollment-tokens/{token_id}", requireCapability(extension.CapMultihost, ah.HandleDeleteEnrollmentToken))

	// Agent endpoints
	r.mux.HandleFunc("GET /api/v1/agents", requireCapability(extension.CapMultihost, ah.HandleListAgents))
	r.mux.HandleFunc("GET /api/v1/agents/{id}", requireCapability(extension.CapMultihost, ah.HandleGetAgent))
	r.mux.HandleFunc("PATCH /api/v1/agents/{id}", requireCapability(extension.CapMultihost, ah.HandleUpdateAgent))
	r.mux.HandleFunc("POST /api/v1/agents/{id}/revoke", requireCapability(extension.CapMultihost, ah.HandleRevokeAgent))
	r.mux.HandleFunc("DELETE /api/v1/agents/{id}", requireCapability(extension.CapMultihost, ah.HandleDeleteAgent))
}
