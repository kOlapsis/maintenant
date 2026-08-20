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

package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/alert/maintenance"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/license"
	"github.com/kolapsis/maintenant/internal/mcp"
	"github.com/kolapsis/maintenant/internal/ratelimit"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/telemetry"
	"github.com/kolapsis/maintenant/internal/update"
	"github.com/kolapsis/maintenant/internal/webhook"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// App holds all application services and manages their lifecycle.
type App struct {
	cfg    Config
	logger *slog.Logger

	// Infrastructure
	db *store.DB
	rt runtime.Runtime

	// Core services
	containerSvc       *container.Service
	endpointSvc        *endpoint.Service
	heartbeatSvc       *heartbeat.Service
	certSvc            *certificate.Service
	resourceSvc        *resource.Service
	securitySvc        *security.Service
	updateSvc          *update.Service
	statusSvc          *status.Service
	subscriberSvc      *status.SubscriberService
	personalizationSvc *status.PersonalizationService

	// Alert pipeline
	alertEngine     *alert.Engine
	notifier        *alert.Notifier
	escalationStore *store.EscalationStore
	escalationSvc   *escalation.Service

	// HTTP
	broker        *v1.SSEBroker
	statusBroker  *v1.SSEBroker
	router        *v1.Router
	statusHandler *status.Handler
	srv           *http.Server

	// Stores (needed for retention cleanup and reconciliation)
	alertStore     alert.AlertStore
	updateStore    update.UpdateStore
	containerStore *store.ContainerStore
	epStore        *store.EndpointStore
	hbStore        *store.HeartbeatStore
	certStore      *store.CertificateStore
	resStore       *store.ResourceStore
	agentStore     *store.AgentStore
	agentSessions  *agentserver.Sessions
	agentSrv       *agentserver.Server
	// shuttingDown suppresses agent-disconnect alerts during graceful shutdown,
	// where every stream ends at once and would otherwise page for the whole fleet.
	shuttingDown    atomic.Bool
	statusCompStore *store.StatusComponentStoreImpl

	// Background services
	checkEngine    *endpoint.CheckEngine
	maintScheduler *status.MaintenanceScheduler
	scorer         *security.Scorer
	rl             *ratelimit.Limiter
	apiRL          *ratelimit.Limiter
	licenseMgr     *license.Manager
	mcpServer      *gomcp.Server
	// degradedPlanLogged keeps the multi-host degradation to one line: the
	// helper is consulted at three call sites during a single startup.
	degradedPlanLogged sync.Once

	// Telemetry
	telemetrySvc *telemetry.Service

	// Webhook
	webhookDispatcher *webhook.Dispatcher

	// Swarm
	swarmDetector       *swarm.Detector
	swarmCluster        *swarm.SwarmCluster
	swarmDiscovery      *swarm.ServiceDiscovery
	swarmEvents         *swarm.EventProcessor
	swarmNodeStore      *store.SwarmNodeStore
	swarmTopologyStore  *store.SwarmTopologyStore
	swarmIngest         *swarm.IngestService
	swarmNodeSvc        *swarm.NodeService
	swarmCrashLoop      *swarm.CrashLoopDetector
	swarmUpdateTracker  *swarm.UpdateTracker
	swarmTaskTracker    *swarm.TaskTracker
	swarmReplicaChecker *swarm.ReplicaHealthChecker

	// Kubernetes
	k8sStore  *store.KubernetesStore
	k8sIngest *kubernetes.IngestService
}

// sseBroadcaster adapts the SSEBroker to the agentserver.EventBroadcaster interface.
type sseBroadcaster struct {
	broker *v1.SSEBroker
}

func (b *sseBroadcaster) BroadcastEvent(eventType string, data any) {
	b.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
}

// New creates and wires all application services.
func New(cfg Config, logger *slog.Logger) (*App, error) {
	a := &App{
		cfg:    cfg,
		logger: logger,
	}

	if cfg.K8sNamespaces != "" {
		logger.Info("K8s namespace allowlist configured", "namespaces", cfg.K8sNamespaces)
	}
	if cfg.K8sExcludeNS != "" {
		logger.Info("K8s namespace blocklist configured", "exclude_namespaces", cfg.K8sExcludeNS)
	}

	// --- Database ---
	db, err := store.Open(cfg.DBPath, logger)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	a.db = db

	if err := store.Migrate(context.Background(), db, logger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// --- Stores ---
	containerStore := store.NewContainerStore(db)
	a.containerStore = containerStore
	epStore := store.NewEndpointStore(db)
	a.epStore = epStore
	hbStore := store.NewHeartbeatStore(db)
	a.hbStore = hbStore
	certStore := store.NewCertificateStore(db)
	a.certStore = certStore
	resStore := store.NewResourceStore(db)
	a.resStore = resStore
	alertStore := store.NewAlertStore(db)
	a.alertStore = alertStore
	channelStore := store.NewChannelStore(db)
	triggerStore := store.NewTriggerStore(db)
	silenceStore := store.NewSilenceStore(db)
	statusCompStore := store.NewStatusComponentStore(db)
	a.statusCompStore = statusCompStore
	personalizationStore := store.NewPersonalizationStore(db)
	incidentStore := store.NewIncidentStore(db)
	maintenanceStore := store.NewMaintenanceStore(db)
	subscriberStore := store.NewSubscriberStore(db)
	webhookStore := store.NewWebhookStore(db)
	updateStore := store.NewUpdateStore(db)
	a.updateStore = updateStore
	agentStore := store.NewAgentStore(db)
	a.agentStore = agentStore

	// The mode gate lives in Start(), after the license manager has resolved the
	// edition. Evaluating it here would read the package default and reject every
	// edition, Pro included.

	// --- License manager ---
	license.InitPublicKey(cfg.PublicKeyB64)
	if cfg.LicenseKey != "" {
		dataDir := filepath.Dir(cfg.DBPath)
		lm, err := license.NewManager(cfg.LicenseKey, dataDir, cfg.Version, cfg.BuildDate, logger)
		if err != nil {
			logger.Warn("license manager initialization failed, running as Community Edition", "error", err)
		} else {
			a.licenseMgr = lm
			extension.CurrentEdition = lm.Edition
		}
	}

	// --- Runtime detection ---
	ctx := context.Background()
	rt, err := runtime.Detect(ctx, logger)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("detect container runtime: %w", err)
	}
	a.rt = rt

	if err := rt.TryConnect(ctx); err != nil {
		logger.Warn("container runtime unavailable, starting in degraded mode", "runtime", rt.Name())
		rt.SetDisconnected()
	}

	// Swarm topology stores + ingest are created unconditionally: a remote agent
	// may report a swarm even when this server's own runtime is docker or
	// kubernetes. The local runtime reconciles into the same tables under the
	// LocalAgent id (see lifecycle reconcile loop).
	a.swarmNodeStore = store.NewSwarmNodeStore(db)
	a.swarmTopologyStore = store.NewSwarmTopologyStore(db)
	a.swarmIngest = swarm.NewIngestService(a.swarmTopologyStore, a.swarmNodeStore, logger)

	// Kubernetes topology store + ingest, also created unconditionally so a
	// remote kubernetes agent can report into per-agent tables regardless of the
	// server's own runtime.
	a.k8sStore = store.NewKubernetesStore(db)
	a.k8sIngest = kubernetes.NewIngestService(a.k8sStore, logger)

	// --- Swarm detection (only when runtime is connected) ---
	if rt.IsConnected() {
		if dr, ok := rt.(*docker.Runtime); ok {
			detector := swarm.NewDetector(dr.Client(), logger)
			a.swarmDetector = detector
			result, err := detector.Detect(ctx)
			if err != nil {
				logger.Warn("Swarm detection failed, continuing without Swarm support", "error", err)
			} else if result.Active && result.IsManager {
				a.swarmCluster = &swarm.SwarmCluster{
					ID:        result.ClusterID,
					IsManager: result.IsManager,
				}
				a.swarmDiscovery = swarm.NewServiceDiscovery(dr.Client(), logger)
				a.swarmDiscovery.SetNetworkResolver(func(ctx context.Context, networkID string) (string, string, error) {
					net, err := dr.Client().NetworkInspect(ctx, networkID)
					if err != nil {
						return "", "", err
					}
					return net.Name, net.Scope, nil
				})
				a.swarmEvents = swarm.NewEventProcessor(a.swarmDiscovery, logger)

				// Node health, crash-loop detection and update tracking follow the
				// swarm dashboard capability.
				if extension.Allows(extension.CapSwarmDashboard) {
					a.swarmNodeSvc = swarm.NewNodeService(dr.Client(), a.swarmNodeStore, logger)
					a.swarmCrashLoop = swarm.NewCrashLoopDetector(logger)
					a.swarmUpdateTracker = swarm.NewUpdateTracker(dr.Client(), logger)
					a.swarmTaskTracker = swarm.NewTaskTracker(dr.Client(), logger)
					a.swarmReplicaChecker = swarm.NewReplicaHealthChecker(logger)
				}
			}
		}
	}

	// --- Services ---
	var logFetcher container.LogFetcher
	if lf, ok := rt.(container.LogFetcher); ok {
		logFetcher = lf
	}
	a.containerSvc = container.NewService(container.Deps{
		Store:          containerStore,
		Logger:         logger,
		LogFetcher:     logFetcher,
		RestartChecker: alert.NewRestartDetector(containerStore, logger),
		Discoverer:     rt,
		AgentRuntime:   agentRuntimeResolver{store: agentStore},
	})
	uptimeCalc := container.NewUptimeCalculator(containerStore)

	a.securitySvc = security.NewService(security.Deps{Logger: logger})
	a.resourceSvc = resource.NewService(resource.Deps{
		Store:        resStore,
		Runtime:      rt,
		ContainerSvc: a.containerSvc,
		Logger:       logger,
		RawWindow:    cfg.Retention.Snapshots,
	})
	// --- License checkers for quota enforcement ---
	// The checkers are always injected, built from the single declaration of the
	// caps: -1 means unlimited, so there is no sentinel to invent and no edition
	// branch here. Leaving them nil in one edition let the service defaults drift
	// away from the values the interface reports.
	certLicenseChecker := &certificate.DefaultLicenseChecker{MaxCertificates: extension.Limit(extension.ResourceCertificates)}
	endpointLicenseChecker := &endpoint.DefaultLicenseChecker{MaxEndpoints: extension.Limit(extension.ResourceEndpoints)}
	heartbeatLicenseChecker := &heartbeat.DefaultLicenseChecker{MaxHeartbeats: extension.Limit(extension.ResourceHeartbeats)}

	a.certSvc = certificate.NewService(certificate.Deps{
		Store:          certStore,
		Logger:         logger,
		LicenseChecker: certLicenseChecker,
	})

	// --- Endpoint monitoring ---
	a.checkEngine = endpoint.NewCheckEngine(func(endpointID string, result endpoint.CheckResult) {
		a.endpointSvc.ProcessCheckResult(ctx, endpointID, result)
		if len(result.TLSPeerCertificates) > 0 {
			ep, err := a.endpointSvc.GetEndpoint(ctx, endpointID)
			if err == nil && ep != nil && certificate.IsHTTPS(ep.Target) {
				a.certSvc.ProcessAutoDetectedCerts(ctx, endpointID, ep.Target, result.TLSPeerCertificates, result.TLSOCSPResponse)
			}
		}
	}, logger)
	a.endpointSvc = endpoint.NewService(endpoint.Deps{
		Store:          epStore,
		Engine:         a.checkEngine,
		Logger:         logger,
		LicenseChecker: endpointLicenseChecker,
	})
	alertDetector := alert.NewEndpointAlertDetector()

	// --- Heartbeat monitoring ---
	a.heartbeatSvc = heartbeat.NewService(heartbeat.Deps{
		Store:          hbStore,
		Logger:         logger,
		LicenseChecker: heartbeatLicenseChecker,
		BaseURL:        cfg.BaseURL,
	})

	// --- SMTP ---
	var smtpSender *alert.SMTPSender
	if cfg.SMTP.Host != "" {
		smtpSender = alert.NewSMTPSender(alert.SMTPConfig{
			Host:     cfg.SMTP.Host,
			Port:     cfg.SMTP.Port,
			Username: cfg.SMTP.Username,
			Password: cfg.SMTP.Password,
			From:     cfg.SMTP.From,
		})
		logger.Info("SMTP sender configured", "host", cfg.SMTP.Host)
	}

	// --- Alert engine ---
	a.notifier = alert.NewNotifier(channelStore, logger, cfg.AllowPrivateWebhooks)
	if smtpSender != nil {
		a.notifier.SetSMTPSender(smtpSender)
	}

	// --- SSE brokers ---
	a.broker = v1.NewSSEBroker(logger)
	a.statusBroker = v1.NewSSEBroker(logger)

	// Emit per-agent topology change events so connected clients scoped to a
	// remote agent refetch its Workloads/Pods/Services/Tasks live.
	topologyBroadcast := func(eventType string, data any) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
	}
	a.swarmIngest.SetBroadcaster(topologyBroadcast)
	a.k8sIngest.SetBroadcaster(topologyBroadcast)

	// Agent session registry (depends on broker)
	a.agentSessions = agentserver.NewSessions(logger, &sseBroadcaster{broker: a.broker})

	// Agent gRPC server (Pro-gated at Start time).
	a.agentSrv = agentserver.New(agentserver.Deps{
		AgentStore:  a.agentStore,
		Sessions:    a.agentSessions,
		Broadcaster: &sseBroadcaster{broker: a.broker},
		Limiter:     agentserver.NewLimiter(cfg.MultiHost.AgentRateLimitPerSecond),
		Dispatcher: agentserver.NewDispatcher(agentserver.DispatchDeps{
			Container:   a.containerSvc,
			Inventory:   a.containerSvc,
			Resource:    a.resourceSvc,
			Endpoint:    a.endpointSvc,
			Certificate: a.certSvc,
			Heartbeat:   a.heartbeatSvc,
			Swarm:       a.swarmIngest,
			Kubernetes:  a.k8sIngest,
			// Provision endpoint/cert monitors from a remote container's labels
			// (the agent probes them itself; the server never dials them).
			LabelSync: func(ctx context.Context, agentID, containerName, externalID string, labels map[string]string) {
				a.endpointSvc.SyncAgentEndpoints(ctx, agentID, containerName, externalID, labels)
				a.certSvc.SyncAgentCerts(ctx, agentID, externalID, labels)
			},
		}),
		Logger: logger.With("component", "agentserver"),
	})

	a.alertEngine = alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: channelStore,
		SilenceStore: silenceStore,
		TriggerStore: triggerStore,
		Logger:       logger,
		Notifier:     a.notifier,
		Broadcaster: alert.NewSSEBroadcasterFunc(func(eventType string, data any) {
			a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		}),
	})

	// --- Public Status Page ---
	a.subscriberSvc = status.NewSubscriberService(subscriberStore, nil, cfg.BaseURL, logger)
	a.statusSvc = status.NewService(status.Deps{
		Components:  statusCompStore,
		Logger:      logger,
		Incidents:   incidentStore,
		Maintenance: maintenanceStore,
		Subscribers: a.subscriberSvc,
		Broadcaster: func(eventType string, data any) {
			a.statusBroker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		},
	})
	a.wireStatusProvider()
	a.maintScheduler = status.NewMaintenanceScheduler(maintenanceStore, statusCompStore, incidentStore, a.statusSvc, logger)
	a.personalizationSvc = status.NewPersonalizationService(personalizationStore, logger.With("component", "personalization"))
	personalizationPublicHandler := status.NewPersonalizationPublicHandler(a.personalizationSvc, logger)
	a.statusHandler = status.NewHandler(a.statusSvc, a.statusBroker, logger)
	a.statusHandler.SetPersonalizationHandler(personalizationPublicHandler)

	// --- Webhook dispatcher ---
	a.webhookDispatcher = webhook.NewDispatcher(webhookStore, a.notifier, logger)

	// --- Update intelligence ---
	registryClient := update.NewRegistryClient()
	updateScanner := update.NewScanner(registryClient, updateStore, logger)
	containerAdapter := update.NewContainerServiceAdapter(a.containerSvc)
	// Wire live label fetching from Docker so maintenant.update.tag-include/exclude labels
	// are available at scan time (labels are not persisted in SQLite).
	if dr, ok := a.rt.(*docker.Runtime); ok {
		containerAdapter.WithLabelFetcher(&dockerLabelFetcher{rt: dr})
	}

	var updateEnricher update.Enricher
	if extension.Allows(extension.CapCVEEnrichment) {
		cveClient := update.NewCVEClient(updateStore, logger.With("component", "cve"))
		changelogResolver := update.NewChangelogResolver(registryClient, logger.With("component", "changelog"))
		riskEngine := update.NewRiskEngine()
		ecosystemResolver := update.NewEcosystemResolver(registryClient, logger.With("component", "ecosystem"))
		updateEnricher = update.NewProEnricher(updateStore, cveClient, changelogResolver, riskEngine, ecosystemResolver, logger.With("component", "enricher"))
		logger.Info("update enrichment pipeline enabled (Pro)")
	}
	a.updateSvc = update.NewService(update.Deps{
		Store:      updateStore,
		Scanner:    updateScanner,
		Containers: containerAdapter,
		Logger:     logger,
		Enricher:   updateEnricher,
	})

	// --- Security posture scoring ---
	ackStore := store.NewAcknowledgmentStore(db)
	a.scorer = security.NewScorer(security.ScorerDeps{
		Certs:     &CertPostureAdapter{CertSvc: a.certSvc},
		CVEs:      &CVEPostureAdapter{Store: updateStore},
		Updates:   &UpdatePostureAdapter{Store: updateStore},
		Security:  a.securitySvc,
		Acks:      ackStore,
		Threshold: cfg.SecurityScoreThreshold,
	})

	if cfg.SecurityScoreThreshold > 0 {
		logger.Info("security posture threshold configured", "threshold", cfg.SecurityScoreThreshold)
	}

	// --- Escalation policies ---
	a.escalationStore = store.NewEscalationStore(db)

	// Maintenance suppressor: real implementation in Pro, noop in CE.
	var suppressor alert.MaintenanceSuppressor = extension.NoopMaintenanceSuppressor{}
	if extension.Allows(extension.CapMaintenanceWindows) {
		suppressor = maintenance.NewSuppressor(maintenanceStore, logger.With("component", "maintenance-suppressor"))
	}
	// Must be called before alertEngine.Start (invoked in App.Start).
	a.alertEngine.SetMaintenanceSuppressor(suppressor)

	a.escalationSvc = escalation.NewService(
		a.escalationStore,
		channelStore,
		extension.CurrentEdition,
		suppressor,
		logger.With("component", "escalation"),
	)

	// Concrete escalator runner: only wired in Pro. In CE the engine
	// keeps its built-in noopEscalator, which means the 60s evaluation ticker
	// (alert.Engine.Start) does not start either. SetEscalator must run before
	// alertEngine.Start (called later in App.Start).
	if extension.Allows(extension.CapAlertEscalation) {
		runner := escalation.NewRunner(escalation.RunnerDeps{
			Store:        a.escalationStore,
			AlertStore:   alertStore,
			ChannelStore: channelStore,
			Notifier:     a.notifier,
			Suppressor:   suppressor,
			Service:      a.escalationSvc,
			Logger:       logger.With("component", "escalation-runner"),
		})
		a.alertEngine.SetEscalator(runner)
		logger.Info("escalation runner enabled (Pro)")
	}

	// --- Wire alert callbacks ---
	a.wireAlertCallbacks(alertDetector)
	a.wireUpdateCallback()
	a.wirePostureCallbacks()
	a.wireSwarmCallbacks()
	a.wireAgentLifecycleAlerts()

	// --- Router ---
	uptimeDailyStore := store.NewUptimeDailyStore(db)
	a.router = v1.NewRouter(v1.HandlerDeps{
		// Core services
		Broker:       a.broker,
		Runtime:      rt,
		Containers:   a.containerSvc,
		Uptime:       uptimeCalc,
		Endpoints:    a.endpointSvc,
		Heartbeats:   a.heartbeatSvc,
		Certificates: a.certSvc,
		Resources:    a.resourceSvc,
		Logger:       logger,
		// Alert pipeline
		AlertStore:    alertStore,
		ChannelStore:  channelStore,
		TriggerStore:  triggerStore,
		SilenceStore:  silenceStore,
		Notifier:      a.notifier,
		Escalator:     a.alertEngine.Escalator(),
		EscalationSvc: a.escalationSvc,
		// Status page admin
		StatusComponents:   statusCompStore,
		StatusIncidents:    incidentStore,
		StatusSubscribers:  subscriberStore,
		StatusMaintenance:  maintenanceStore,
		StatusSvc:          a.statusSvc,
		StatusBroker:       a.statusBroker,
		PersonalizationSvc: a.personalizationSvc,
		// Webhooks
		WebhookStore: webhookStore,
		// UI extras
		UptimeDaily:      uptimeDailyStore,
		LogStreamer:      rt,
		ResourceTopSvc:   a.resourceSvc,
		SparklineFetcher: epStore,
		// Update intelligence
		UpdateSvc:        a.updateSvc,
		UpdateStore:      updateStore,
		ContainerAdapter: containerAdapter,
		// Security
		SecuritySvc: a.securitySvc,
		Scorer:      a.scorer,
		AckStore:    ackStore,
		// License
		LicenseMgr: a.licenseMgr,
		// Swarm
		SwarmCluster:        func() *swarm.SwarmCluster { return a.swarmCluster },
		SwarmDiscovery:      func() *swarm.ServiceDiscovery { return a.swarmDiscovery },
		SwarmDetector:       func() *swarm.Detector { return a.swarmDetector },
		SwarmNodeStore:      a.swarmNodeStoreAsInterface(),
		SwarmUpdateTracker:  a.swarmUpdateTracker,
		SwarmCrashLoop:      a.swarmCrashLoop,
		SwarmReplicaChecker: a.swarmReplicaChecker,
		SwarmTopologyStore:  a.swarmTopologyStore,
		// Kubernetes (per-agent store-backed reads)
		KubernetesStore: a.k8sStore,
		// Multi-host agents (Pro)
		AgentStore:          a.agentStore,
		AgentSessions:       a.agentSessions,
		GRPCPublicURL:       cfg.MultiHost.GRPCPublicURL,
		GRPCListen:          cfg.MultiHost.GRPCListen,
		AgentStaleThreshold: time.Duration(cfg.MultiHost.AgentStaleThresholdSeconds) * time.Second,
		// HTTP config
		CORSOrigins:          cfg.CORSOrigins,
		MaxBodySize:          cfg.MaxBodySize,
		BuildVersion:         cfg.Version,
		OrganisationName:     cfg.OrgName,
		AllowPrivateWebhooks: cfg.AllowPrivateWebhooks,
	})

	// --- Rate limiters ---
	// Public surfaces (/ping/, /status/, /mcp) take the tight bucket.
	a.rl = ratelimit.New(10, 20)
	// /api/ gets its own, far looser one: a dashboard load fans out dozens of
	// parallel calls, so the tight bucket would 429 ordinary use. This is a
	// flood ceiling, not a quota — it must never be reachable by the UI.
	a.apiRL = ratelimit.New(50, 200)

	// --- MCP Server ---
	mcpSvc := &mcp.Services{
		Containers:    a.containerSvc,
		Endpoints:     a.endpointSvc,
		Heartbeats:    a.heartbeatSvc,
		Certificates:  a.certSvc,
		Resources:     a.resourceSvc,
		Alerts:        alertStore,
		Channels:      channelStore,
		Triggers:      triggerStore,
		Escalator:     a.alertEngine.Escalator(),
		Updates:       a.updateSvc,
		Incidents:     incidentStore,
		Maintenance:   maintenanceStore,
		Runtime:       rt,
		LogFetcher:    rt,
		EscalationSvc: a.escalationSvc,
		Agents:        a.agentStore,
		Sessions:      a.agentSessions,
		AgentLogs:     a.agentSessions,
		// Security & supply-chain (read-only)
		SecuritySvc: a.securitySvc,
		Scorer:      a.scorer,
		UpdateStore: updateStore,
		// Orchestrators (read-only)
		Kubernetes:     a.k8sStore,
		SwarmCluster:   func() *swarm.SwarmCluster { return a.swarmCluster },
		SwarmDiscovery: func() *swarm.ServiceDiscovery { return a.swarmDiscovery },
		SwarmTopology:  a.swarmTopologyStore,
		SwarmNodes:     a.swarmNodeStore,
		Version:        cfg.Version,
		Logger:         logger.With("component", "mcp"),
	}
	a.mcpServer = mcp.NewServer(mcpSvc)

	// --- Telemetry (SHM SDK, opt-out via MAINTENANT_DISABLE_TELEMETRY) ---
	a.telemetrySvc = telemetry.New(telemetry.Config{
		Disabled:   cfg.DisableTelemetry,
		AppVersion: cfg.Version,
	}, telemetry.Deps{
		Containers:       containerStore,
		Endpoints:        epStore,
		Heartbeats:       hbStore,
		Certificates:     certStore,
		Webhooks:         webhookStore,
		StatusComponents: statusCompStore,
		Edition:          telemetry.EditionFunc(extension.CurrentEdition),
	}, logger.With("component", "telemetry"))

	// --- Build HTTP server ---
	a.srv = a.buildHTTPServer()

	return a, nil
}

// RunMCPStdio runs the MCP server over stdin/stdout, then returns.
func (a *App) RunMCPStdio(ctx context.Context) error {
	a.logger.Info("starting MCP server in stdio mode")
	return a.mcpServer.Run(ctx, &gomcp.StdioTransport{})
}

// multihostPlanAllowed reports whether the multi-host plan may run.
//
// It is not the same question as extension.Allows(CapMultihost). A Personal
// instance whose update window closed falls back to Community, and the mode
// gate is the only refusal to start in the product: applying it here would take
// a whole fleet's monitoring down over an unpaid renewal. The plan keeps
// running, the Personal features do not: every route behind
// requireCapability(CapMultihost) still refuses, so no new host can be
// enrolled. Agents already enrolled keep streaming, since the gRPC server
// carries no capability check.
func (a *App) multihostPlanAllowed() bool {
	licenseStatus := ""
	if a.licenseMgr != nil {
		licenseStatus = a.licenseMgr.State().Status
	}

	granted := extension.Allows(extension.CapMultihost)
	if !multihostPlanPermitted(granted, licenseStatus) {
		return false
	}

	if !granted {
		a.degradedPlanLogged.Do(func() {
			a.logger.Error("update window closed: the multi-host plan keeps running, but enrolling and managing hosts is now refused",
				"mode", a.cfg.Mode,
				"edition", extension.CurrentEdition(),
				"updates_until", a.licenseMgr.State().UpdatesUntil,
			)
		})
	}
	return true
}

// multihostPlanPermitted is the decision itself, kept apart from the manager so
// it can be exercised directly.
func multihostPlanPermitted(capabilityGranted bool, licenseStatus string) bool {
	return capabilityGranted || licenseStatus == license.StatusUpdateWindowEnded
}

// Start begins all background services and the HTTP server.
// It blocks until ctx is canceled, then performs a graceful shutdown.
func (a *App) Start(ctx context.Context) error {
	// Checked here rather than in New(): this is the only path that listens, so
	// --mcp-stdio keeps working without OAuth credentials it has no use for.
	if err := a.cfg.ValidateHTTP(); err != nil {
		return err
	}

	a.db.StartWriter(ctx)

	if a.licenseMgr != nil {
		a.wireLicenseSubscriber(ctx)
		a.licenseMgr.Start(ctx)
	}

	// Mode gate: server mode needs multi-host. Checked here because
	// licenseMgr.Start runs the initial verification synchronously, so the
	// edition is settled — NewManager has already loaded the disk cache and Start
	// has refreshed it. Checking it in New() read the package default and
	// rejected every edition, Pro included.
	if a.cfg.Mode != "" && a.cfg.Mode != "embedded" {
		if !a.multihostPlanAllowed() {
			return fmt.Errorf("%s mode requires the %s edition (current edition: %s)",
				a.cfg.Mode, extension.MinEdition(extension.CapMultihost), extension.CurrentEdition())
		}
	}

	a.alertEngine.Start(ctx)
	// Runs here too so DB-backed monitors are swept even without a container runtime.
	a.pruneOrphanAlerts(ctx)

	if extension.Allows(extension.CapAlertEscalation) {
		go a.escalationSvc.RunRetentionLoop(ctx)
		a.logger.Info("escalation retention loop started")
	}
	a.notifier.Start(ctx)
	a.endpointSvc.Start(ctx)
	a.heartbeatSvc.StartDeadlineChecker(ctx)

	// Telemetry: best-effort. Self-exits on ctx cancellation; panics are
	// contained inside the package (FR-009/FR-011/FR-012).
	a.telemetrySvc.Start(ctx)

	// Webhook observer
	webhookObserverCh := make(chan v1.SSEEvent, 64)
	a.broker.AddObserver(webhookObserverCh)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-webhookObserverCh:
				if !ok {
					return
				}
				a.webhookDispatcher.HandleEvent(ctx, evt.Type, evt.Data)
			}
		}
	}()

	// Background services (always run, regardless of runtime availability)
	go a.rl.Start(ctx)
	go a.apiRL.Start(ctx)
	go a.resourceSvc.Start(ctx)
	go a.certSvc.Start(ctx)
	go a.maintScheduler.Start(ctx)
	go a.subscriberSvc.Start(ctx)
	go a.updateSvc.Start(ctx)

	// Agent session ring-buffer tick + stale watcher.
	if a.agentSessions != nil {
		a.agentSessions.StartRingAdvancer(ctx)
		threshold := time.Duration(a.cfg.MultiHost.AgentStaleThresholdSeconds) * time.Second
		if threshold == 0 {
			threshold = 60 * time.Second
		}
		a.agentSessions.StartStaleWatcher(ctx, 10*time.Second, threshold,
			agentserver.OfflineReportGrace, a.agentStore.StaleAgents)
	}

	// Enrollment token GC: purge unconsumed tokens older than 7 days, every hour.
	if a.agentStore != nil {
		go func() {
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := a.agentStore.GcExpiredTokens(ctx); err != nil {
						a.logger.Error("enrollment token GC failed", "err", err)
					}
				}
			}
		}()
	}

	// Swarm node periodic refresh (Pro, 60s).
	if a.swarmNodeSvc != nil {
		go a.startNodeRefresh(ctx)
	}

	// Local-runtime topology reconcile into the per-agent store (under LocalAgent),
	// so store-backed K8s/Swarm views reflect the local cluster too. Each is a
	// no-op unless the matching runtime is active.
	go a.startKubernetesReconcile(ctx)
	go a.startSwarmTopologyReconcile(ctx)

	// Swarm context recheck (60s) — detects swarm activation/deactivation.
	if a.swarmDetector != nil {
		go a.startSwarmRecheck(ctx)
	}

	// Retention cleanup
	a.startRetentionCleanup(ctx)

	// Container monitoring supervisor: wires reconcile + event stream when connected,
	// and manages reconnection in background when degraded (Phase 5).
	a.startRuntimeSupervisor(ctx)

	// Agent gRPC server — server/embedded modes only, where multi-host is open.
	if a.multihostPlanAllowed() && a.cfg.Mode != "agent" {
		if err := a.startAgentGRPC(ctx); err != nil {
			return fmt.Errorf("start agent gRPC server: %w", err)
		}
	}

	// Embedded agent (mode=server + --embedded-agent + Pro).
	// Starts a local agent goroutine that connects to the local gRPC endpoint.
	if a.cfg.Mode == "server" && a.cfg.MultiHost.EmbeddedAgent && a.multihostPlanAllowed() {
		a.startEmbeddedAgent(ctx)
	}

	// HTTP server — never in agent mode: an agent only streams to the server and
	// must not expose the UI/API. (main.go already exits before app.Start in agent
	// mode; this is a defensive invariant.)
	if a.cfg.Mode == "agent" {
		a.logger.Warn("agent mode: HTTP server disabled")
	} else {
		go func() {
			a.logger.Info("starting HTTP server", "addr", a.cfg.Addr)
			if err := a.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				a.logger.Error("HTTP server error", "error", err)
			}
		}()
	}

	// Wait for shutdown
	<-ctx.Done()
	return a.Shutdown()
}

// Shutdown performs a graceful shutdown of all services.
func (a *App) Shutdown() error {
	a.shuttingDown.Store(true)
	a.logger.Info("shutting down maintenant")

	a.endpointSvc.Stop()
	a.logger.Info("endpoint check engine stopped")

	// 10s shutdown grace covers the SHM SDK's 10s HTTP timeout in flight,
	// so an in-progress telemetry snapshot does not extend the deadline (FR-011).
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.srv.Shutdown(shutdownCtx); err != nil {
		a.logger.Error("HTTP server shutdown error", "error", err)
	}

	if a.licenseMgr != nil {
		a.licenseMgr.Stop()
	}

	_ = a.rt.Close()
	_ = a.db.Close()

	a.logger.Info("maintenant stopped")
	return nil
}

// startEmbeddedAgent launches a local agent goroutine connecting to the local gRPC endpoint.
// If the agent is not yet enrolled, a short-lived enrollment token is auto-created.
// Called only when mode=server, --embedded-agent, and Pro license are all active.
func (a *App) startEmbeddedAgent(ctx context.Context) {
	dataDir := filepath.Dir(a.cfg.DBPath)
	agentDataDir := filepath.Join(dataDir, "embedded-agent")
	if err := os.MkdirAll(agentDataDir, 0o700); err != nil {
		a.logger.Error("embedded agent: failed to create data directory", "err", err)
		return
	}

	id, err := agent.LoadOrCreate(agentDataDir)
	if err != nil {
		a.logger.Error("embedded agent: failed to load identity", "err", err)
		return
	}

	var enrollToken string
	if !id.Registered {
		cleartext, hash, tokenID, prefix, err := agent.NewToken()
		if err != nil {
			a.logger.Error("embedded agent: failed to generate enrollment token", "err", err)
			return
		}
		t := &agent.EnrollmentToken{
			TokenID:     tokenID,
			TokenHash:   hash,
			TokenPrefix: prefix,
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(5 * time.Minute),
		}
		if err := a.agentStore.InsertToken(ctx, t); err != nil {
			a.logger.Error("embedded agent: failed to create enrollment token", "err", err)
			return
		}
		// Held in memory just long enough to hand to the agent goroutine below.
		enrollToken = cleartext
	}

	grpcURL := "grpcs://" + a.cfg.MultiHost.GRPCListen
	agentCfg := agent.AgentConfig{
		DataDir:            agentDataDir,
		ServerURL:          grpcURL,
		EnrollmentToken:    enrollToken,
		Label:              "embedded",
		AgentVersion:       a.cfg.Version,
		InsecureSkipVerify: true, // loopback TLS
	}

	go func() {
		// Short delay to let the gRPC server open its listener.
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		if err := agent.Run(ctx, agentCfg, a.logger.With("component", "embedded-agent")); err != nil && !errors.Is(err, context.Canceled) {
			a.logger.Error("embedded agent exited", "err", err)
		}
	}()

	a.logger.Info("embedded agent scheduled", "grpc_url", grpcURL)
}

// startAgentGRPC binds and serves the agent-facing gRPC server in a background
// goroutine. TLS is required (FR-031); if no keypair is configured a
// self-signed dev cert is generated in-memory and a warning is logged.
func (a *App) startAgentGRPC(ctx context.Context) error {
	listen := a.cfg.MultiHost.GRPCListen
	if listen == "" {
		listen = "127.0.0.1:8443"
	}

	var tlsCfg *tls.Config
	if a.cfg.MultiHost.InsecureGRPC {
		a.logger.Warn("agentserver: TLS disabled — only use behind a trusted reverse proxy (MAINTENANT_GRPC_TLS_INSECURE)")
	} else {
		hosts := collectGRPCTLSHosts(a.cfg.MultiHost.GRPCPublicURL, listen)
		var err error
		tlsCfg, err = agentserver.LoadOrGenerateTLS(
			a.cfg.MultiHost.TLSCertFile,
			a.cfg.MultiHost.TLSKeyFile,
			hosts,
			a.logger.With("component", "agentserver"),
		)
		if err != nil {
			return err
		}
	}

	a.agentSrv.StartTokenGC(ctx)
	go func() {
		if err := a.agentSrv.Start(ctx, listen, tlsCfg); err != nil && !errors.Is(err, context.Canceled) {
			a.logger.Error("agentserver: stopped", "err", err)
		}
	}()
	a.logger.Info("agent gRPC server scheduled", "listen", listen)
	return nil
}

// collectGRPCTLSHosts returns the SAN list used for the self-signed dev TLS
// cert. It pulls the host out of the public URL (when set) and the listen
// address; wildcards like 0.0.0.0/:: are filtered. Empty result is handled
// downstream by falling back to 127.0.0.1 + localhost.
func collectGRPCTLSHosts(publicURL, listen string) []string {
	var hosts []string
	add := func(raw string) {
		if raw == "" {
			return
		}
		h := raw
		if hh, _, err := net.SplitHostPort(raw); err == nil {
			h = hh
		}
		if h == "" || h == "0.0.0.0" || h == "::" || h == "[::]" {
			return
		}
		hosts = append(hosts, h)
	}

	if publicURL != "" {
		stripped := publicURL
		for _, scheme := range []string{"grpcs://", "grpc://", "https://", "http://"} {
			if rest, ok := strings.CutPrefix(stripped, scheme); ok {
				stripped = rest
				break
			}
		}
		// stripped may carry a trailing path/query — keep only the authority.
		if i := strings.IndexAny(stripped, "/?#"); i >= 0 {
			stripped = stripped[:i]
		}
		add(stripped)
	}
	add(listen)
	return hosts
}

// swarmNodeStoreAsInterface returns the SwarmNodeStore as a NodeStore interface, or nil if not available.
func (a *App) swarmNodeStoreAsInterface() swarm.NodeStore {
	if a.swarmNodeStore == nil {
		return nil
	}
	return a.swarmNodeStore
}

// dockerLabelFetcher implements update.LabelFetcher for Docker runtimes.
// It fetches live container labels at scan time so tag-include/tag-exclude labels
// are available without persisting them in SQLite.
type dockerLabelFetcher struct {
	rt *docker.Runtime
}

func (f *dockerLabelFetcher) FetchLabels(ctx context.Context) (map[string]map[string]string, error) {
	results, err := f.rt.DiscoverAllWithLabels(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch container labels: %w", err)
	}
	labels := make(map[string]map[string]string, len(results))
	for _, r := range results {
		labels[r.Container.ExternalID] = r.Labels
	}
	return labels, nil
}
