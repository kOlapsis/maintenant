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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/license"
	pbmcp "github.com/kolapsis/maintenant/internal/mcp"
	"github.com/kolapsis/maintenant/internal/ratelimit"
	"github.com/kolapsis/maintenant/internal/resource"
	pbruntime "github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store/sqlite"
	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/update"
	"github.com/kolapsis/maintenant/internal/webhook"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// App holds all application services and manages their lifecycle.
type App struct {
	cfg    Config
	logger *slog.Logger

	// Infrastructure
	db *sqlite.DB
	rt pbruntime.Runtime

	// Core services
	containerSvc  *container.Service
	endpointSvc   *endpoint.Service
	heartbeatSvc  *heartbeat.Service
	certSvc       *certificate.Service
	resourceSvc   *resource.Service
	securitySvc   *security.Service
	updateSvc     *update.Service
	statusSvc     *status.Service
	subscriberSvc *status.SubscriberService

	// Alert pipeline
	alertEngine *alert.Engine
	notifier    *alert.Notifier

	// HTTP
	broker        *v1.SSEBroker
	statusBroker  *v1.SSEBroker
	router        *v1.Router
	statusHandler *status.Handler
	srv           *http.Server

	// Stores (needed for retention cleanup and reconciliation)
	alertStore     alert.AlertStore
	updateStore    update.UpdateStore
	containerStore *sqlite.ContainerStore
	epStore        *sqlite.EndpointStore
	hbStore        *sqlite.HeartbeatStore
	certStore      *sqlite.CertificateStore
	resStore       *sqlite.ResourceStore

	// Background services
	checkEngine    *endpoint.CheckEngine
	maintScheduler *status.MaintenanceScheduler
	scorer         *security.Scorer
	rl             *ratelimit.Limiter
	licenseMgr     *license.LicenseManager
	mcpServer      *gomcp.Server

	// Webhook
	webhookDispatcher *webhook.Dispatcher

	// Swarm
	swarmDetector      *swarm.Detector
	swarmCluster       *swarm.SwarmCluster
	swarmDiscovery     *swarm.ServiceDiscovery
	swarmEvents        *swarm.EventProcessor
	swarmNodeStore     *sqlite.SwarmNodeStore
	swarmNodeSvc       *swarm.NodeService
	swarmCrashLoop      *swarm.CrashLoopDetector
	swarmUpdateTracker  *swarm.UpdateTracker
	swarmTaskTracker    *swarm.TaskTracker
	swarmReplicaChecker *swarm.ReplicaHealthChecker
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
	db, err := sqlite.Open(cfg.DBPath, logger)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	a.db = db

	if err := sqlite.Migrate(db.ReadDB(), logger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// --- Stores ---
	store := sqlite.NewContainerStore(db)
	a.containerStore = store
	epStore := sqlite.NewEndpointStore(db)
	a.epStore = epStore
	hbStore := sqlite.NewHeartbeatStore(db)
	a.hbStore = hbStore
	certStore := sqlite.NewCertificateStore(db)
	a.certStore = certStore
	resStore := sqlite.NewResourceStore(db)
	a.resStore = resStore
	alertStore := sqlite.NewAlertStore(db)
	a.alertStore = alertStore
	channelStore := sqlite.NewChannelStore(db)
	silenceStore := sqlite.NewSilenceStore(db)
	statusCompStore := sqlite.NewStatusComponentStore(db)
	incidentStore := sqlite.NewIncidentStore(db)
	maintenanceStore := sqlite.NewMaintenanceStore(db)
	subscriberStore := sqlite.NewSubscriberStore(db)
	webhookStore := sqlite.NewWebhookStore(db)
	updateStore := sqlite.NewUpdateStore(db)
	a.updateStore = updateStore

	// --- License manager ---
	license.InitPublicKey(cfg.PublicKeyB64)
	if cfg.LicenseKey != "" {
		dataDir := filepath.Dir(cfg.DBPath)
		lm, err := license.NewLicenseManager(cfg.LicenseKey, dataDir, cfg.Version, logger)
		if err != nil {
			logger.Warn("license manager initialization failed, running as Community Edition", "error", err)
		} else {
			a.licenseMgr = lm
			extension.CurrentEdition = func() extension.Edition {
				if lm.IsProEnabled() {
					return extension.Enterprise
				}
				return extension.Community
			}
		}
	}

	// --- Runtime detection ---
	ctx := context.Background()
	rt, err := pbruntime.Detect(ctx, logger)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("detect container runtime: %w", err)
	}
	a.rt = rt

	if err := rt.Connect(ctx); err != nil {
		_ = rt.Close()
		_ = db.Close()
		return nil, fmt.Errorf("connect to runtime %s: %w", rt.Name(), err)
	}

	// --- Swarm detection ---
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

			// Enterprise: node health monitoring, crash-loop detection, update tracking
			if extension.CurrentEdition() == extension.Enterprise {
				a.swarmNodeStore = sqlite.NewSwarmNodeStore(db)
				a.swarmNodeSvc = swarm.NewNodeService(dr.Client(), a.swarmNodeStore, logger)
				a.swarmCrashLoop = swarm.NewCrashLoopDetector(logger)
				a.swarmUpdateTracker = swarm.NewUpdateTracker(dr.Client(), logger)
				a.swarmTaskTracker = swarm.NewTaskTracker(dr.Client(), logger)
				a.swarmReplicaChecker = swarm.NewReplicaHealthChecker(logger)
			}
		}
	}

	// --- Services ---
	var logFetcher container.LogFetcher
	if lf, ok := rt.(container.LogFetcher); ok {
		logFetcher = lf
	}
	a.containerSvc = container.NewService(container.Deps{
		Store:          store,
		Logger:         logger,
		LogFetcher:     logFetcher,
		RestartChecker: alert.NewRestartDetector(store, logger),
		Discoverer:     rt,
	})
	uptimeCalc := container.NewUptimeCalculator(store)

	a.securitySvc = security.NewService(security.Deps{Logger: logger})
	a.resourceSvc = resource.NewService(resource.Deps{
		Store:        resStore,
		Runtime:      rt,
		ContainerSvc: a.containerSvc,
		Logger:       logger,
	})
	a.certSvc = certificate.NewService(certificate.Deps{
		Store:  certStore,
		Logger: logger,
	})

	// --- Endpoint monitoring ---
	a.checkEngine = endpoint.NewCheckEngine(func(endpointID int64, result endpoint.CheckResult) {
		a.endpointSvc.ProcessCheckResult(ctx, endpointID, result)
		if len(result.TLSPeerCertificates) > 0 {
			ep, err := a.endpointSvc.GetEndpoint(ctx, endpointID)
			if err == nil && ep != nil && certificate.IsHTTPS(ep.Target) {
				a.certSvc.ProcessAutoDetectedCerts(ctx, endpointID, ep.Target, result.TLSPeerCertificates)
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
	a.notifier = alert.NewNotifier(channelStore, logger)
	if smtpSender != nil {
		a.notifier.SetSMTPSender(smtpSender)
	}

	// --- SSE brokers ---
	a.broker = v1.NewSSEBroker(logger)
	a.statusBroker = v1.NewSSEBroker(logger)

	a.alertEngine = alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: channelStore,
		SilenceStore: silenceStore,
		Logger:       logger,
		Notifier:     a.notifier,
		Broadcaster: alert.NewSSEBroadcasterFunc(func(eventType string, data interface{}) {
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
		Broadcaster: func(eventType string, data interface{}) {
			a.statusBroker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		},
	})
	a.wireStatusProvider()
	a.maintScheduler = status.NewMaintenanceScheduler(maintenanceStore, statusCompStore, incidentStore, a.statusSvc, logger)
	a.statusHandler = status.NewHandler(a.statusSvc, a.statusBroker, logger)

	// --- Webhook dispatcher ---
	a.webhookDispatcher = webhook.NewDispatcher(webhookStore, a.notifier, logger)

	// --- Update intelligence ---
	registryClient := update.NewRegistryClient()
	updateScanner := update.NewScanner(registryClient, updateStore, logger)
	containerAdapter := update.NewContainerServiceAdapter(a.containerSvc)

	var updateEnricher update.Enricher
	if extension.CurrentEdition() == extension.Enterprise {
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
	ackStore := sqlite.NewAcknowledgmentStore(db)
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

	// --- Wire alert callbacks ---
	a.wireAlertCallbacks(alertDetector)
	a.wireUpdateCallback()
	a.wirePostureCallbacks()
	a.wireSwarmCallbacks()

	// --- Router ---
	uptimeDailyStore := sqlite.NewUptimeDailyStore(db)
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
		AlertStore:   alertStore,
		ChannelStore: channelStore,
		SilenceStore: silenceStore,
		Notifier:     a.notifier,
		// Status page admin
		StatusComponents:  statusCompStore,
		StatusIncidents:   incidentStore,
		StatusSubscribers: subscriberStore,
		StatusMaintenance: maintenanceStore,
		StatusSvc:         a.statusSvc,
		StatusBroker:      a.statusBroker,
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
		SwarmCluster:   func() *swarm.SwarmCluster { return a.swarmCluster },
		SwarmDiscovery: func() *swarm.ServiceDiscovery { return a.swarmDiscovery },
		SwarmDetector:  func() *swarm.Detector { return a.swarmDetector },
		SwarmNodeStore:      a.swarmNodeStoreAsInterface(),
		SwarmUpdateTracker:  a.swarmUpdateTracker,
		SwarmCrashLoop:      a.swarmCrashLoop,
		SwarmReplicaChecker: a.swarmReplicaChecker,
		// HTTP config
		CORSOrigins:      cfg.CORSOrigins,
		MaxBodySize:      cfg.MaxBodySize,
		BuildVersion:     cfg.Version,
		OrganisationName: cfg.OrgName,
	})

	// --- Rate limiter ---
	a.rl = ratelimit.New(10, 20)

	// --- MCP Server ---
	mcpSvc := &pbmcp.Services{
		Containers:   a.containerSvc,
		Endpoints:    a.endpointSvc,
		Heartbeats:   a.heartbeatSvc,
		Certificates: a.certSvc,
		Resources:    a.resourceSvc,
		Alerts:       alertStore,
		Updates:      a.updateSvc,
		Incidents:    extension.NoopIncidentManager{},
		Maintenance:  extension.NoopMaintenanceScheduler{},
		Runtime:      rt,
		LogFetcher:   rt,
		Version:      cfg.Version,
		Logger:       logger.With("component", "mcp"),
	}
	a.mcpServer = pbmcp.NewServer(mcpSvc)

	// --- Build HTTP server ---
	a.srv = a.buildHTTPServer()

	return a, nil
}

// RunMCPStdio runs the MCP server over stdin/stdout, then returns.
func (a *App) RunMCPStdio(ctx context.Context) error {
	a.logger.Info("starting MCP server in stdio mode")
	return a.mcpServer.Run(ctx, &gomcp.StdioTransport{})
}

// Start begins all background services and the HTTP server.
// It blocks until ctx is canceled, then performs a graceful shutdown.
func (a *App) Start(ctx context.Context) error {
	a.db.StartWriter(ctx)

	if a.licenseMgr != nil {
		a.licenseMgr.Start(ctx)
	}

	a.alertEngine.Start(ctx)
	a.notifier.Start(ctx)
	a.endpointSvc.Start(ctx)
	a.heartbeatSvc.StartDeadlineChecker(ctx)

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

	// Startup reconciliation
	a.reconcile(ctx)

	// Background services
	go a.rl.Start(ctx)
	go a.resourceSvc.Start(ctx)
	go a.certSvc.Start(ctx)
	go a.maintScheduler.Start(ctx)
	go a.subscriberSvc.Start(ctx)
	go a.updateSvc.Start(ctx)

	// Swarm node periodic refresh (Enterprise, 60s).
	if a.swarmNodeSvc != nil {
		go a.startNodeRefresh(ctx)
	}

	// Swarm context recheck (60s) — detects swarm activation/deactivation.
	if a.swarmDetector != nil {
		go a.startSwarmRecheck(ctx)
	}

	// Retention cleanup
	a.startRetentionCleanup(ctx)

	// Event stream
	a.startEventStream(ctx)

	// HTTP server
	go func() {
		a.logger.Info("starting HTTP server", "addr", a.cfg.Addr)
		if err := a.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Wait for shutdown
	<-ctx.Done()
	return a.Shutdown()
}

// Shutdown performs a graceful shutdown of all services.
func (a *App) Shutdown() error {
	a.logger.Info("shutting down maintenant")

	a.endpointSvc.Stop()
	a.logger.Info("endpoint check engine stopped")

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

// swarmNodeStoreAsInterface returns the SwarmNodeStore as a NodeStore interface, or nil if not available.
func (a *App) swarmNodeStoreAsInterface() swarm.NodeStore {
	if a.swarmNodeStore == nil {
		return nil
	}
	return a.swarmNodeStore
}
