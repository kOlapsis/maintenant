// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/mcp"
	"github.com/kolapsis/maintenant/internal/outbound"
	"github.com/kolapsis/maintenant/internal/ratelimit"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/telemetry"
	"github.com/kolapsis/maintenant/internal/trust"
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

	// Instance visibility: this process's row in the instances table and the
	// storage state served by the health diagnostic (FR-012, FR-020).
	instanceStore  *store.InstanceStore
	instanceID     string
	instanceRecord store.Instance
	storage        *storageState
	rt             runtime.Runtime

	// Core services
	containerSvc       *container.Service
	endpointSvc        *endpoint.Service
	heartbeatSvc       *heartbeat.Service
	outboundSvc        *outbound.Service
	certSvc            *certificate.Service
	resourceSvc        *resource.Service
	securitySvc        *security.Service
	updateSvc          *update.Service
	statusSvc          *status.Service
	subscriberSvc      *status.SubscriberService
	personalizationSvc status.PersonalizationManager
	statusMailer       status.Mailer

	// Alert pipeline
	alertEngine     *alert.Engine
	notifier        *alert.Notifier
	downDetector    *alert.DownDetector
	escalationStore *store.EscalationStore
	escalationSvc   escalation.Service

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
	uptimeStore    *store.UptimeDailyStore
	agentStore     *store.AgentStore
	agentSessions  extpoint.AgentSessions
	serveAgents    func(ctx context.Context, cfg extpoint.GRPCConfig) error
	eolSvc         *eol.Service
	// shuttingDown suppresses agent-disconnect alerts during graceful shutdown,
	// where every stream ends at once and would otherwise page for the whole fleet.
	shuttingDown    atomic.Bool
	statusCompStore *store.StatusComponentStoreImpl

	// Closed when the retention cleanup has stopped writing. Shutdown waits on
	// it before closing the database.
	retentionStopped <-chan struct{}

	// Background services
	checkEngine    *endpoint.CheckEngine
	maintScheduler status.MaintenanceRunner
	scorer         security.PostureScorer
	rl             *ratelimit.Limiter
	apiRL          *ratelimit.Limiter
	subscribeRL    *ratelimit.Limiter
	licenseMgr     extension.EditionSource
	ext            extpoint.Set
	mcpServer      *gomcp.Server
	// degradedPlanLogged keeps the multi-host degradation to one line: the
	// helper is consulted at three call sites during a single startup.
	degradedPlanLogged sync.Once

	// Telemetry
	telemetrySvc *telemetry.Service

	// Webhook
	webhookDispatcher *webhook.Dispatcher

	// Swarm
	swarmDetector      *swarm.Detector
	swarmRecheckNow    chan struct{}
	swarmCluster       atomic.Pointer[swarm.SwarmCluster]
	swarmMgr           atomic.Pointer[swarmManager]
	swarmNodeStore     *store.SwarmNodeStore
	swarmTopologyStore *store.SwarmTopologyStore
	swarmIngest        *swarm.IngestService

	// Kubernetes
	k8sStore  *store.KubernetesStore
	k8sIngest *kubernetes.IngestService
	k8sAlerts *kubernetes.K8sAlertChecker
}

// sseBroadcaster adapts the SSEBroker to the extpoint.EventBroadcaster interface.
type sseBroadcaster struct {
	broker *v1.SSEBroker
}

func (b *sseBroadcaster) BroadcastEvent(eventType string, data any) {
	b.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
}

// New creates and wires all application services.
func New(cfg Config, logger *slog.Logger, opts ...Option) (*App, error) {
	a := &App{
		cfg:    cfg,
		logger: logger,
	}
	for _, opt := range opts {
		opt(a)
	}

	trustedProxies, err := cfg.ParseTrustedProxies()
	if err != nil {
		return nil, err
	}
	clientIP := ratelimit.NewClientIPResolver(trustedProxies)

	// --- Rate limiters ---
	// Public surfaces (/ping/, /status/, /mcp, /oauth/) take the tight bucket.
	a.rl = ratelimit.New(10, 20, clientIP)
	// /api/ gets its own, far looser one: a dashboard load fans out dozens of
	// parallel calls, so the tight bucket would 429 ordinary use. This is a
	// flood ceiling, not a quota — it must never be reachable by the UI.
	a.apiRL = ratelimit.New(50, 200, clientIP)
	// Status-page subscriptions: five per hour and per address.
	a.subscribeRL = ratelimit.New(5.0/3600.0, 5, clientIP)

	if cfg.K8sNamespaces != "" {
		logger.Info("K8s namespace allowlist configured", "namespaces", cfg.K8sNamespaces)
	}
	if cfg.K8sExcludeNS != "" {
		logger.Info("K8s namespace blocklist configured", "exclude_namespaces", cfg.K8sExcludeNS)
	}

	// --- Database ---
	// No connection string means SQLite, in every edition and every mode.
	// A configured but unusable external database refuses to start: there is
	// no silent fallback to the local file (FR-004).
	ctx := context.Background()
	db, err := openStorage(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}
	a.db = db

	if err := store.Migrate(ctx, db, logger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	logStorageOpened(ctx, db, logger)

	if err := a.registerInstance(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
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
	lm, err := extension.NewEditionSource(extension.SourceConfig{
		LicenseKey:   cfg.LicenseKey,
		PublicKeyB64: cfg.PublicKeyB64,
		DataDir:      filepath.Dir(cfg.DBPath),
		Version:      cfg.Version,
		BuildDate:    cfg.BuildDate,
		Logger:       logger,
	})
	if err != nil {
		logger.Warn("license manager initialization failed, running as Community Edition", "error", err)
	} else if lm != nil {
		a.licenseMgr = lm
		extension.CurrentEdition = lm.Edition
	}

	// --- Runtime detection ---
	rt, err := runtime.Detect(ctx, logger)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("detect container runtime: %w", err)
	}
	a.rt = rt
	if dr, ok := rt.(*docker.Runtime); ok {
		dr.SetProxyLabels(cfg.ProxyLabels)
	}

	if err := rt.TryConnect(ctx); err != nil {
		logger.Warn("container runtime unavailable, starting in degraded mode", "runtime", rt.Name(), "error", err)
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
	a.k8sAlerts = kubernetes.NewK8sAlertChecker(logger)

	// --- Swarm detection: armed for any Docker runtime, run now if it answers ---
	if dr, ok := rt.(*docker.Runtime); ok {
		a.swarmDetector = swarm.NewDetector(dr.Client(), logger)
		a.swarmRecheckNow = make(chan struct{}, 1)
		if rt.IsConnected() {
			result, err := a.swarmDetector.Detect(ctx)
			if err != nil {
				logger.Warn("Swarm detection failed, continuing without Swarm support", "error", err)
			} else if cluster := result.Cluster(); cluster != nil {
				a.swarmCluster.Store(cluster)
				a.swarmMgr.Store(newSwarmManager(dr, a.swarmNodeStore, logger))
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
	a.certSvc = certificate.NewService(certificate.Deps{
		Store:  certStore,
		Logger: logger,
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
		Store:  epStore,
		Engine: a.checkEngine,
		Logger: logger,
	})
	alertDetector := alert.NewEndpointAlertDetector()

	// --- Heartbeat monitoring ---
	a.heartbeatSvc = heartbeat.NewService(heartbeat.Deps{
		Store:   hbStore,
		Logger:  logger,
		BaseURL: cfg.BaseURL,
	})
	a.outboundSvc = outbound.NewService(outbound.Deps{
		Store:   store.NewOutboundHeartbeatStore(db),
		Logger:  logger,
		Version: cfg.Version,
	})

	// --- Alert engine ---
	smtpCfg := extpoint.SMTPConfig{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
	}
	a.notifier = alert.NewNotifier(channelStore, logger, cfg.AllowPrivateWebhooks)
	if a.ext.Channels != nil {
		channels := a.ext.Channels(extpoint.ChannelDeps{
			HTTPClient: a.notifier.HTTPClient(),
			SMTP:       smtpCfg,
			Logger:     logger,
		})
		for chType, sender := range channels {
			a.notifier.RegisterChannel(chType, sender)
		}
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

	// --- Host operating system end of support ---
	var eolFetcher *eol.Fetcher
	if !cfg.DisableOSEOLRefresh {
		eolFetcher = &eol.Fetcher{
			Client:    &http.Client{Timeout: 15 * time.Second, Transport: trust.HTTPTransport()},
			UserAgent: "maintenant/" + cfg.Version + " (+https://maintenant.dev)",
		}
	}
	a.eolSvc, err = eol.New(eol.Deps{
		Store:     a.agentStore,
		Fetcher:   eolFetcher,
		Emit:      a.emitAlert,
		OnChanged: a.broadcastAgentUpdated,
		Logger:    logger.With("component", "os-eol"),
	})
	if err != nil {
		return nil, fmt.Errorf("os end-of-support service: %w", err)
	}

	// Agent session registry and gRPC server (served at Start time where multi-host is open).
	if a.ext.MultiHost != nil {
		mh := a.ext.MultiHost(extpoint.MultiHostDeps{
			AgentStore:         a.agentStore,
			Broadcaster:        &sseBroadcaster{broker: a.broker},
			RateLimitPerSecond: cfg.MultiHost.AgentRateLimitPerSecond,
			DemoMode:           cfg.DemoMode,
			Container:          a.containerSvc,
			Resource:           a.resourceSvc,
			Endpoint:           a.endpointSvc,
			Certificate:        a.certSvc,
			Heartbeat:          a.heartbeatSvc,
			Swarm:              a.swarmIngest,
			Kubernetes:         a.k8sIngest,
			HostOS:             a.eolSvc,
			// Provision endpoint/cert monitors from a remote container's labels
			// (the agent probes them itself; the server never dials them).
			LabelSync: func(ctx context.Context, agentID, containerName, externalID string, labels map[string]string) {
				a.endpointSvc.SyncAgentEndpoints(ctx, agentID, containerName, externalID, labels)
				a.certSvc.SyncAgentCerts(ctx, agentID, externalID, labels)
			},
			Logger: logger,
		})
		a.agentSessions = mh.Sessions
		a.serveAgents = mh.Serve
	}

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

	// Sustained container downtime. Off unless the operator sets a threshold:
	// enabling it retroactively alerts on every container already stopped.
	if cfg.ContainerDownAfter > 0 {
		a.downDetector = alert.NewDownDetector(containerStore, alertStore, cfg.ContainerDownAfter,
			logger.With("component", "container-down"))
	}

	// --- Public Status Page ---
	a.statusSvc = status.NewService(status.Deps{
		Components:  statusCompStore,
		Logger:      logger,
		Incidents:   incidentStore,
		Maintenance: maintenanceStore,
		PublicBroadcaster: func(eventType string, data any) {
			a.statusBroker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		},
		AdminBroadcaster: func(eventType string, data any) {
			a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		},
	})
	a.wireStatusProvider()
	if a.ext.StatusPage != nil {
		sp := a.ext.StatusPage(extpoint.StatusPageDeps{
			Service:         a.statusSvc,
			Components:      statusCompStore,
			Incidents:       incidentStore,
			Maintenance:     maintenanceStore,
			Subscribers:     subscriberStore,
			Personalization: personalizationStore,
			SMTP:            smtpCfg,
			BaseURL:         cfg.BaseURL,
			Logger:          logger,
		})
		a.statusSvc.SetIncidentHandler(sp.Incidents)
		a.statusSvc.SetSubscriberNotifier(sp.Notifier)
		a.maintScheduler = sp.Maintenance
		a.personalizationSvc = sp.Personalization
		a.statusMailer = sp.Mailer
	}
	a.subscriberSvc = status.NewSubscriberService(subscriberStore, a.statusMailer, cfg.BaseURL, logger)
	a.statusSvc.SetSubscriberService(a.subscriberSvc)
	personalizationPublicHandler := status.NewPersonalizationPublicHandler(personalizationStore, logger)
	a.statusHandler = status.NewHandler(a.statusSvc, a.statusBroker, logger, a.subscribeRL, status.PageURL(cfg.StatusURL, cfg.BaseURL))
	a.statusHandler.SetPersonalizationHandler(personalizationPublicHandler)

	// --- Webhook dispatcher ---
	a.webhookDispatcher = webhook.NewDispatcher(webhookStore, a.notifier, logger)

	// --- Update intelligence ---
	registryClient := update.NewRegistryClient()
	updateScanner := update.NewScanner(registryClient, updateStore, logger)
	containerAdapter := update.NewContainerServiceAdapter(a.containerSvc, logger)
	if fetcher := updateDetailsFetcher(a.rt, logger); fetcher != nil {
		containerAdapter.WithDetailsFetcher(fetcher)
	}

	var updateEnricher update.Enricher
	if a.ext.Enricher != nil {
		updateEnricher = a.ext.Enricher(extpoint.EnricherDeps{Store: updateStore, Registry: registryClient, Logger: logger})
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
	if a.ext.PostureScorer != nil {
		a.scorer = a.ext.PostureScorer(extpoint.PostureDeps{
			Certs:          &CertPostureAdapter{CertSvc: a.certSvc},
			CVEs:           &CVEPostureAdapter{Store: updateStore},
			CVEEvaluations: &CVEEvaluationPostureAdapter{Store: updateStore},
			Updates:        &UpdatePostureAdapter{Store: updateStore},
			Insights:       a.securitySvc,
			Acks:           ackStore,
			Threshold:      cfg.SecurityScoreThreshold,
		})
	}

	if cfg.SecurityScoreThreshold > 0 {
		logger.Info("security posture threshold configured", "threshold", cfg.SecurityScoreThreshold)
	}

	// --- Escalation policies ---
	a.escalationStore = store.NewEscalationStore(db)

	var suppressor alert.MaintenanceSuppressor
	if a.ext.Suppressor != nil {
		suppressor = a.ext.Suppressor(extpoint.SuppressorDeps{Windows: maintenanceStore, Logger: logger})
	}
	// Must be called before alertEngine.Start (invoked in App.Start).
	if suppressor != nil {
		a.alertEngine.SetMaintenanceSuppressor(suppressor)
	}

	// SetEscalator must run before alertEngine.Start (called later in App.Start).
	if a.ext.Escalation != nil {
		esc := a.ext.Escalation(extpoint.EscalationDeps{
			Store:      a.escalationStore,
			Alerts:     alertStore,
			Channels:   channelStore,
			Notifier:   a.notifier,
			Suppressor: suppressor,
			Logger:     logger,
		})
		a.escalationSvc = esc.Service
		if esc.Escalator != nil {
			a.alertEngine.SetEscalator(esc.Escalator)
		}
	}

	// --- Wire alert callbacks ---
	a.wireAlertCallbacks(alertDetector)
	a.wireUpdateCallback()
	if a.scorer != nil {
		a.wirePostureCallbacks()
	}
	if m := a.swarmMgr.Load(); m != nil {
		a.wireSwarmCallbacks(m)
	}
	a.wireKubernetesAlerts()
	a.wireAgentLifecycleAlerts()

	// --- Router ---
	a.uptimeStore = store.NewUptimeDailyStore(db)
	a.router = v1.NewRouter(v1.HandlerDeps{
		// Core services
		Broker:       a.broker,
		Runtime:      rt,
		Storage:      a.storage,
		Containers:   a.containerSvc,
		Uptime:       uptimeCalc,
		Endpoints:    a.endpointSvc,
		Heartbeats:   a.heartbeatSvc,
		Certificates: a.certSvc,
		Outbound:     a.outboundSvc,
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
		StatusMaintRunner:  a.maintScheduler,
		StatusSvc:          a.statusSvc,
		PersonalizationSvc: a.personalizationSvc,
		StatusMailer:       a.statusMailer,
		// Webhooks
		WebhookStore:  webhookStore,
		WebhookTester: a.webhookDispatcher,
		// UI extras
		UptimeDaily:      a.uptimeStore,
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
		SwarmCluster:       a.swarmCluster.Load,
		SwarmDiscovery:     a.currentSwarmDiscovery,
		SwarmDetector:      func() *swarm.Detector { return a.swarmDetector },
		SwarmNodeStore:     a.swarmNodeStoreAsInterface(),
		SwarmUpdateTracker: a.currentSwarmUpdateTracker,
		SwarmCrashLoop:     a.currentSwarmCrashLoop,
		SwarmTopologyStore: a.swarmTopologyStore,
		// Kubernetes (per-agent store-backed reads)
		KubernetesStore: a.k8sStore,
		// Multi-host agents
		AgentStore:          a.agentStore,
		AgentSessions:       a.agentSessions,
		GRPCPublicURL:       cfg.MultiHost.GRPCPublicURL,
		GRPCListen:          cfg.MultiHost.GRPCListen,
		AgentStaleThreshold: time.Duration(cfg.MultiHost.AgentStaleThresholdSeconds) * time.Second,
		// Host operating system end of support
		EOL: a.eolSvc,
		// HTTP config
		CORSOrigins:          cfg.CORSOrigins,
		MaxBodySize:          cfg.MaxBodySize,
		BuildVersion:         cfg.Version,
		OrganisationName:     cfg.OrgName,
		StatusURL:            cfg.StatusURL,
		AllowPrivateWebhooks: cfg.AllowPrivateWebhooks,
		TrustedProxies:       trustedProxies,
		DemoMode:             cfg.DemoMode,
	})

	// --- MCP Server ---
	mcpSvc := &mcp.Services{
		Containers:        a.containerSvc,
		Endpoints:         a.endpointSvc,
		Heartbeats:        a.heartbeatSvc,
		Certificates:      a.certSvc,
		Resources:         a.resourceSvc,
		Alerts:            alertStore,
		Channels:          channelStore,
		Triggers:          triggerStore,
		Escalator:         a.alertEngine.Escalator(),
		ChannelTester:     a.notifier,
		ChannelValidators: a.notifier,
		Updates:           a.updateSvc,
		Incidents:         incidentStore,
		IncidentAnnouncer: a.statusSvc,
		Maintenance:       maintenanceStore,
		StatusComponents:  statusCompStore,
		Runtime:           rt,
		LogFetcher:        rt,
		EscalationSvc:     a.escalationSvc,
		Agents:            a.agentStore,
		Sessions:          a.agentSessions,
		AgentLogs:         a.agentSessions,
		EOL:               a.eolSvc,
		// Security & supply-chain (read-only)
		SecuritySvc: a.securitySvc,
		Scorer:      a.scorer,
		UpdateStore: updateStore,
		// Orchestrators (read-only)
		Kubernetes:           a.k8sStore,
		SwarmCluster:         a.swarmCluster.Load,
		SwarmDiscovery:       a.currentSwarmDiscovery,
		SwarmTopology:        a.swarmTopologyStore,
		SwarmNodes:           a.swarmNodeStore,
		AllowPrivateWebhooks: cfg.AllowPrivateWebhooks,
		Broadcast: func(eventType string, data any) {
			a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		},
		Version:  cfg.Version,
		Logger:   logger.With("component", "mcp"),
		DemoMode: cfg.DemoMode,
	}
	a.mcpServer = mcp.NewServer(mcpSvc)

	// --- Telemetry (SHM SDK, opt-out via MAINTENANT_DISABLE_TELEMETRY) ---
	a.telemetrySvc = telemetry.New(telemetry.Config{
		Disabled:   cfg.DisableTelemetry,
		DataDir:    filepath.Join(filepath.Dir(cfg.DBPath), "shm"),
		AppVersion: cfg.Version,
	}, telemetry.Deps{
		Containers:       containerStore,
		Endpoints:        epStore,
		Heartbeats:       hbStore,
		Certificates:     certStore,
		Webhooks:         webhookStore,
		StatusComponents: statusCompStore,
		Edition:          telemetry.EditionFunc(extension.CurrentEdition),
		StorageEngine:    db.Engine,
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
	return capabilityGranted || licenseStatus == extension.LicenseStatusUpdateWindowEnded
}

// Start begins all background services and the HTTP server.
// It blocks until ctx is canceled, then performs a graceful shutdown.
func (a *App) Start(ctx context.Context) error {
	// Checked here rather than in New(): this is the only path that listens, so
	// --mcp-stdio keeps working without OAuth credentials it has no use for.
	if err := a.cfg.ValidateHTTP(); err != nil {
		return err
	}
	if err := a.cfg.ValidateGRPCTLS(); err != nil {
		return err
	}

	// Bound before any service starts: a taken port must fail the boot before it has side effects.
	var httpLn net.Listener
	if a.cfg.Mode != "agent" {
		ln, err := net.Listen("tcp", a.srv.Addr)
		if err != nil {
			return fmt.Errorf("listen on %s: %w", a.srv.Addr, err)
		}
		defer func() { _ = ln.Close() }()
		httpLn = ln
	}

	// Derived so an early return (e.g. a failed gRPC listener below) cancels every
	// background goroutine started with ctx, instead of leaking them until
	// the caller's own context is cancelled.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.db.StartWriter(ctx)

	// Registered after the writer starts: on SQLite every write goes through it.
	a.startInstanceHeartbeat(ctx)
	a.startStorageSupervisor(ctx)

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

	a.notifier.Start(ctx)
	a.endpointSvc.Start(ctx)
	a.heartbeatSvc.StartDeadlineChecker(ctx)
	if !a.cfg.DemoMode {
		a.outboundSvc.Start(ctx)
	}

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
	go a.subscribeRL.Start(ctx)
	go a.resourceSvc.Start(ctx)
	go a.certSvc.Start(ctx)
	if a.maintScheduler != nil {
		go a.maintScheduler.Start(ctx)
	}
	go a.subscriberSvc.Start(ctx)
	go a.updateSvc.Start(ctx)

	// Agent session ring-buffer tick + stale watcher.
	if a.agentSessions != nil {
		threshold := time.Duration(a.cfg.MultiHost.AgentStaleThresholdSeconds) * time.Second
		if threshold == 0 {
			threshold = 60 * time.Second
		}
		a.agentSessions.StartWatchers(ctx, threshold, a.agentStore.StaleAgents)
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

	// Swarm manager loops; the Kubernetes reconcile is wired with each runtime
	// connection cycle.
	if m := a.swarmMgr.Load(); m != nil {
		a.startSwarmManager(ctx, m)
	}

	// Sustained container downtime (opt-in via MAINTENANT_CONTAINER_DOWN_AFTER).
	if a.downDetector != nil {
		go a.startContainerDownCheck(ctx)
	}

	if a.scorer != nil && a.scorer.Threshold() > 0 {
		go a.startPostureCheck(ctx)
	}

	a.startOSEOL(ctx)

	a.seedRestartAlertTracking(ctx)
	go a.containerSvc.RunRestartRecoveryLoop(ctx)
	a.seedKubernetesAlertTracking(ctx)

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
	if a.serveAgents != nil && a.cfg.Mode != "agent" {
		if a.multihostPlanAllowed() {
			if err := a.serveAgents(ctx, extpoint.GRPCConfig{
				Listen:      a.cfg.MultiHost.GRPCListen,
				PublicURL:   a.cfg.MultiHost.GRPCPublicURL,
				TLSCertFile: a.cfg.MultiHost.TLSCertFile,
				TLSKeyFile:  a.cfg.MultiHost.TLSKeyFile,
				Insecure:    a.cfg.MultiHost.InsecureGRPC,
			}); err != nil {
				return fmt.Errorf("start agent gRPC server: %w", err)
			}
		} else {
			required := extension.MinEdition(extension.CapMultihost)
			a.logger.Info("agent gRPC listener not started: agents need the "+string(required)+" edition or above",
				"edition", extension.CurrentEdition(), "required_edition", required, "listen", a.cfg.MultiHost.GRPCListen)
		}
	}

	// Embedded agent (mode=server + --embedded-agent + multi-host plan).
	// Starts a local agent goroutine that connects to the local gRPC endpoint.
	if a.serveAgents != nil && a.cfg.Mode == "server" && a.cfg.MultiHost.EmbeddedAgent && a.multihostPlanAllowed() && !a.cfg.DemoMode {
		a.startEmbeddedAgent(ctx)
	}

	// HTTP server — never in agent mode: an agent only streams to the server and
	// must not expose the UI/API. (main.go already exits before app.Start in agent
	// mode; this is a defensive invariant.)
	if a.cfg.Mode == "agent" {
		a.logger.Warn("agent mode: HTTP server disabled")
	} else {
		a.logger.Info("starting HTTP server", "addr", a.cfg.Addr)
		go func() {
			if err := a.srv.Serve(httpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

	// Drop this instance's row before closing: a clean restart must not see
	// its own previous run as a peer until the stale purge catches up.
	if a.instanceStore != nil {
		if err := a.instanceStore.Deregister(shutdownCtx, a.instanceID); err != nil {
			a.logger.Warn("instance deregistration failed, a restart may report itself as a peer until the stale purge runs",
				"error", err)
		}
	}

	_ = a.rt.Close()

	// The cleanup deletes in batches and only notices the cancellation between
	// them, so closing the database under it would abort a pass mid-write.
	// Bounded by the same grace as the rest of the shutdown: a pass that is
	// still running after that is left behind rather than holding the process.
	if a.retentionStopped != nil {
		select {
		case <-a.retentionStopped:
		case <-shutdownCtx.Done():
			a.logger.Warn("retention cleanup still running at shutdown, closing the database anyway")
		}
	}

	_ = a.db.Close()

	a.logger.Info("maintenant stopped")
	return nil
}

// startEmbeddedAgent launches a local agent goroutine connecting to the local gRPC endpoint.
// If the agent is not yet enrolled, a short-lived enrollment token is auto-created.
// Called only when mode=server, --embedded-agent and the multi-host plan are all active.
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

	grpcURL := embeddedAgentURL(a.cfg.MultiHost)
	agentCfg := agent.AgentConfig{
		DataDir:             agentDataDir,
		ServerURL:           grpcURL,
		EnrollmentToken:     enrollToken,
		Label:               "embedded",
		AgentVersion:        a.cfg.Version,
		InsecureSkipVerify:  true, // loopback TLS
		ProxyLabels:         a.cfg.ProxyLabels,
		SpoolMaxMemoryBytes: a.cfg.MultiHost.AgentSpoolMaxMemoryBytes,
		SpoolMaxDiskBytes:   a.cfg.MultiHost.AgentSpoolMaxDiskBytes,
		SpoolMaxAgeSeconds:  a.cfg.MultiHost.AgentSpoolMaxAgeSeconds,
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

// swarmManager holds a Swarm manager's services, published whole so no reader sees half of it.
type swarmManager struct {
	discovery      *swarm.ServiceDiscovery
	events         *swarm.EventProcessor
	nodeSvc        *swarm.NodeService
	crashLoop      *swarm.CrashLoopDetector
	updateTracker  *swarm.UpdateTracker
	replicaChecker *swarm.ReplicaHealthChecker
	stop           context.CancelFunc
	loops          sync.WaitGroup
}

// newSwarmManager builds the services of a Swarm manager on the Docker client.
func newSwarmManager(dr *docker.Runtime, nodeStore swarm.NodeStore, logger *slog.Logger) *swarmManager {
	discovery := swarm.NewServiceDiscovery(dr.Client(), logger)
	discovery.SetNetworkResolver(func(ctx context.Context, networkID string) (string, string, error) {
		net, err := dr.Client().NetworkInspect(ctx, networkID)
		if err != nil {
			return "", "", err
		}
		return net.Name, net.Scope, nil
	})
	return &swarmManager{
		discovery:      discovery,
		events:         swarm.NewEventProcessor(discovery, logger),
		nodeSvc:        swarm.NewNodeService(dr.Client(), nodeStore, logger),
		crashLoop:      swarm.NewCrashLoopDetector(logger),
		updateTracker:  swarm.NewUpdateTracker(dr.Client(), logger),
		replicaChecker: swarm.NewReplicaHealthChecker(logger),
	}
}

func (a *App) currentSwarmDiscovery() *swarm.ServiceDiscovery {
	if m := a.swarmMgr.Load(); m != nil {
		return m.discovery
	}
	return nil
}

func (a *App) currentSwarmUpdateTracker() *swarm.UpdateTracker {
	if m := a.swarmMgr.Load(); m != nil {
		return m.updateTracker
	}
	return nil
}

func (a *App) currentSwarmCrashLoop() *swarm.CrashLoopDetector {
	if m := a.swarmMgr.Load(); m != nil {
		return m.crashLoop
	}
	return nil
}

// embeddedAgentURL dials the local gRPC listener with the scheme it serves: plaintext h2c in insecure mode, TLS otherwise.
func embeddedAgentURL(mh MultiHostConfig) string {
	if mh.InsecureGRPC {
		return "grpc://" + mh.GRPCListen
	}
	return "grpcs://" + mh.GRPCListen
}

// swarmNodeStoreAsInterface returns the SwarmNodeStore as a NodeStore interface, or nil if not available.
func (a *App) swarmNodeStoreAsInterface() swarm.NodeStore {
	if a.swarmNodeStore == nil {
		return nil
	}
	return a.swarmNodeStore
}
