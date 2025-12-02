package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kolapsis/pulseboard/cmd/pulseboard/web"
	"github.com/kolapsis/pulseboard/internal/alert"
	v1 "github.com/kolapsis/pulseboard/internal/api/v1"
	"github.com/kolapsis/pulseboard/internal/certificate"
	"github.com/kolapsis/pulseboard/internal/container"
	"github.com/kolapsis/pulseboard/internal/docker"
	"github.com/kolapsis/pulseboard/internal/endpoint"
	"github.com/kolapsis/pulseboard/internal/heartbeat"
	_ "github.com/kolapsis/pulseboard/internal/kubernetes"
	"github.com/kolapsis/pulseboard/internal/resource"
	pbruntime "github.com/kolapsis/pulseboard/internal/runtime"
	"github.com/kolapsis/pulseboard/internal/status"
	"github.com/kolapsis/pulseboard/internal/store/sqlite"
	"github.com/kolapsis/pulseboard/internal/update"
	"github.com/kolapsis/pulseboard/internal/webhook"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger.Info("PulseBoard starting", "version", version, "commit", commit, "build_date", buildDate)
	v1.SetBuildVersion(version)

	// Configuration from environment
	addr := envOrDefault("PULSEBOARD_ADDR", "127.0.0.1:8080")
	dbPath := envOrDefault("PULSEBOARD_DB", "./pulseboard.db")

	// K8s namespace config (used when runtime is Kubernetes)
	k8sNamespaces := os.Getenv("PULSEBOARD_K8S_NAMESPACES")
	k8sExcludeNamespaces := os.Getenv("PULSEBOARD_K8S_EXCLUDE_NAMESPACES")
	if k8sNamespaces != "" {
		logger.Info("K8s namespace allowlist configured", "namespaces", k8sNamespaces)
	}
	if k8sExcludeNamespaces != "" {
		logger.Info("K8s namespace blocklist configured", "exclude_namespaces", k8sExcludeNamespaces)
	}
	// Suppress unused warnings — these will be passed to K8s runtime constructor in Phase 5
	_ = k8sNamespaces
	_ = k8sExcludeNamespaces

	// Graceful shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Database ---
	db, err := sqlite.Open(dbPath, logger)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := sqlite.Migrate(db.ReadDB(), logger); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	db.StartWriter(ctx)

	store := sqlite.NewContainerStore(db)
	epStore := sqlite.NewEndpointStore(db)
	hbStore := sqlite.NewHeartbeatStore(db)
	certStore := sqlite.NewCertificateStore(db)
	resStore := sqlite.NewResourceStore(db)
	alertStore := sqlite.NewAlertStore(db)
	channelStore := sqlite.NewChannelStore(db)
	silenceStore := sqlite.NewSilenceStore(db)
	statusCompStore := sqlite.NewStatusComponentStore(db)
	webhookStore := sqlite.NewWebhookStore(db)
	updateStore := sqlite.NewUpdateStore(db)

	// --- Runtime detection ---
	rt, err := pbruntime.Detect(ctx, logger)
	if err != nil {
		logger.Error("failed to detect container runtime", "error", err)
		os.Exit(1)
	}
	defer rt.Close()

	// Connect to runtime (with retry)
	if err := rt.Connect(ctx); err != nil {
		logger.Error("failed to connect to runtime", "runtime", rt.Name(), "error", err)
		os.Exit(1)
	}

	// --- Services ---
	svc := container.NewService(store, logger)
	if lf, ok := rt.(container.LogFetcher); ok {
		svc.SetLogFetcher(lf)
	}
	svc.SetRestartChecker(alert.NewRestartDetector(store, logger))
	svc.SetDiscoverer(rt)
	uptimeCalc := container.NewUptimeCalculator(store)

	// --- Resource monitoring ---
	resSvc := resource.NewService(resStore, rt, svc, logger)

	// --- Certificate monitoring ---
	certSvc := certificate.NewService(certStore, logger)

	// --- Endpoint monitoring ---
	var epSvc *endpoint.Service

	// Wire check result callback (including certificate auto-detection)
	checkEngine := endpoint.NewCheckEngine(func(endpointID int64, result endpoint.CheckResult) {
		epSvc.ProcessCheckResult(ctx, endpointID, result)

		// Auto-detect TLS certificates from HTTPS endpoint checks
		if len(result.TLSPeerCertificates) > 0 {
			ep, err := epSvc.GetEndpoint(ctx, endpointID)
			if err == nil && ep != nil && certificate.IsHTTPS(ep.Target) {
				certSvc.ProcessAutoDetectedCerts(ctx, endpointID, ep.Target, result.TLSPeerCertificates)
			}
		}
	}, logger)
	epSvc = endpoint.NewService(epStore, checkEngine, logger)
	alertDetector := alert.NewEndpointAlertDetector()
	epSvc.Start(ctx)

	// --- Heartbeat monitoring ---
	hbSvc := heartbeat.NewService(hbStore, logger, nil)
	hbSvc.SetBaseURL(envOrDefault("PULSEBOARD_BASE_URL", "http://localhost:"+addr[1:]))
	hbSvc.StartDeadlineChecker(ctx)

	// --- Alert engine ---
	alertEngine := alert.NewEngine(alertStore, channelStore, silenceStore, logger)
	notifier := alert.NewNotifier(channelStore, logger)
	alertEngine.SetNotifier(notifier)

	// --- HTTP server ---
	broker := v1.NewSSEBroker(logger)

	// Wire SSE broadcaster to alert engine
	alertEngine.SetBroadcaster(alert.NewSSEBroadcasterFunc(func(eventType string, data interface{}) {
		broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
	}))

	alertEngine.Start(ctx)
	notifier.Start(ctx)

	// --- Public Status Page ---
	statusBroker := v1.NewSSEBroker(logger)
	statusSvc := status.NewService(statusCompStore, logger)
	statusSvc.SetMonitorStatusProvider(func(ctx context.Context, monitorType string, monitorID int64) string {
		switch monitorType {
		case "container":
			c, err := svc.GetContainer(ctx, monitorID)
			if err != nil || c == nil {
				return status.StatusOperational
			}
			if c.State == container.StateRunning {
				if c.HealthStatus != nil && *c.HealthStatus == container.HealthUnhealthy {
					return status.StatusDegraded
				}
				return status.StatusOperational
			}
			return status.StatusMajorOutage
		case "endpoint":
			ep, err := epSvc.GetEndpoint(ctx, monitorID)
			if err != nil || ep == nil {
				return status.StatusOperational
			}
			switch ep.Status {
			case endpoint.StatusUp:
				return status.StatusOperational
			case endpoint.StatusDown:
				return status.StatusMajorOutage
			default:
				return status.StatusOperational
			}
		case "heartbeat":
			hb, err := hbSvc.GetHeartbeat(ctx, monitorID)
			if err != nil || hb == nil {
				return status.StatusOperational
			}
			switch hb.Status {
			case heartbeat.StatusUp:
				return status.StatusOperational
			case heartbeat.StatusDown:
				return status.StatusMajorOutage
			default:
				return status.StatusDegraded
			}
		case "certificate":
			cert, err := certSvc.GetMonitor(ctx, monitorID)
			if err != nil || cert == nil {
				return status.StatusOperational
			}
			switch cert.Status {
			case certificate.StatusValid:
				return status.StatusOperational
			case certificate.StatusExpiring:
				return status.StatusDegraded
			default:
				return status.StatusMajorOutage
			}
		}
		return status.StatusOperational
	})
	statusSvc.SetBroadcaster(func(eventType string, data interface{}) {
		statusBroker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
	})

	statusHandler := status.NewHandler(statusSvc, statusBroker, logger)

	router := v1.NewRouter(broker, rt, svc, uptimeCalc, epSvc, hbSvc, certSvc, resSvc, logger, v1.AlertOpts{
		AlertStore:   alertStore,
		ChannelStore: channelStore,
		SilenceStore: silenceStore,
		Notifier:     notifier,
	}, v1.APIConfig{
		WebhookStore: webhookStore,
	}, v1.StatusAdminOpts{
		Components: statusCompStore,
		StatusSvc:  statusSvc,
		Broker:     statusBroker,
	})

	// --- UI redesign endpoints ---
	uptimeDailyStore := sqlite.NewUptimeDailyStore(db)
	router.RegisterUIRoutes(v1.UIConfig{
		UptimeDaily:      uptimeDailyStore,
		LogStreamer:      rt,
		ContainerSvc:     svc,
		ResourceTopSvc:   resSvc,
		SparklineFetcher: epStore,
	})

	// --- Webhook dispatcher ---
	webhookDispatcher := webhook.NewDispatcher(webhookStore, notifier, logger)
	webhookObserverCh := make(chan v1.SSEEvent, 64)
	broker.AddObserver(webhookObserverCh)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-webhookObserverCh:
				if !ok {
					return
				}
				webhookDispatcher.HandleEvent(ctx, evt.Type, evt.Data)
			}
		}
	}()

	// --- Wire alert event forwarding from all monitoring services ---
	alertCh := alertEngine.EventChannel()

	// sendAlert forwards an alert event to the alert engine.
	sendAlert := func(evt alert.Event) {
		alertCh <- evt
	}

	// Container restart and health alerts
	svc.SetEventCallback(func(eventType string, data interface{}) {
		broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		switch eventType {
		case "container.restart_alert":
			if ra, ok := data.(*alert.RestartAlert); ok && ra != nil {
				sendAlert(alert.Event{
					Source:     alert.SourceContainer,
					AlertType:  "restart_loop",
					Severity:   alert.SeverityWarning,
					Message:    fmt.Sprintf("Container %s exceeded restart threshold (%d/%d)", ra.ContainerName, ra.RestartCount, ra.Threshold),
					EntityType: "container",
					EntityID:   ra.ContainerID,
					EntityName: ra.ContainerName,
					Details: map[string]any{
						"restart_count": ra.RestartCount,
						"threshold":     ra.Threshold,
					},
					Timestamp: ra.Timestamp,
				})
			}
		case "container.health_changed":
			m, ok := data.(map[string]interface{})
			if !ok {
				return
			}
			prev, _ := m["previous_health"].(*container.HealthStatus)
			newH, _ := m["health_status"].(container.HealthStatus)
			if prev != nil && *prev == container.HealthHealthy && newH == container.HealthUnhealthy {
				sendAlert(alert.Event{
					Source:     alert.SourceContainer,
					AlertType:  "health_unhealthy",
					Severity:   alert.SeverityWarning,
					Message:    "Container became unhealthy",
					EntityType: "container",
					EntityID:   toInt64(m["id"]),
					Details:    m,
					Timestamp:  time.Now(),
				})
			} else if prev != nil && *prev == container.HealthUnhealthy && newH == container.HealthHealthy {
				sendAlert(alert.Event{
					Source:     alert.SourceContainer,
					AlertType:  "health_unhealthy",
					Severity:   alert.SeverityInfo,
					IsRecover:  true,
					Message:    "Container recovered to healthy",
					EntityType: "container",
					EntityID:   toInt64(m["id"]),
					Details:    m,
					Timestamp:  time.Now(),
				})
			}
		}
	})

	// Endpoint alerts
	epSvc.SetAlertCallback(func(ep *endpoint.Endpoint, result endpoint.CheckResult) (string, interface{}) {
		a := alertDetector.EvaluateCheckResult(ep, result)
		if a == nil {
			return "", nil
		}
		if a.Type == "alert" {
			sendAlert(alert.Event{
				Source:     alert.SourceEndpoint,
				AlertType:  "consecutive_failure",
				Severity:   alert.SeverityCritical,
				Message:    fmt.Sprintf("Endpoint %s failed %d consecutive checks", a.Target, a.Failures),
				EntityType: "endpoint",
				EntityID:   a.EndpointID,
				EntityName: a.ContainerName,
				Details: map[string]any{
					"target":     a.Target,
					"failures":   a.Failures,
					"threshold":  a.Threshold,
					"last_error": a.LastError,
				},
				Timestamp: a.Timestamp,
			})
			return "endpoint.alert", map[string]interface{}{
				"endpoint_id":          a.EndpointID,
				"container_name":       a.ContainerName,
				"target":               a.Target,
				"consecutive_failures": a.Failures,
				"threshold":            a.Threshold,
				"last_error":           a.LastError,
				"timestamp":            a.Timestamp,
			}
		}
		sendAlert(alert.Event{
			Source:     alert.SourceEndpoint,
			AlertType:  "consecutive_failure",
			Severity:   alert.SeverityInfo,
			IsRecover:  true,
			Message:    fmt.Sprintf("Endpoint %s recovered after %d consecutive successes", a.Target, a.Successes),
			EntityType: "endpoint",
			EntityID:   a.EndpointID,
			EntityName: a.ContainerName,
			Details: map[string]any{
				"target":    a.Target,
				"successes": a.Successes,
				"threshold": a.Threshold,
			},
			Timestamp: a.Timestamp,
		})
		return "endpoint.recovery", map[string]interface{}{
			"endpoint_id":           a.EndpointID,
			"container_name":        a.ContainerName,
			"target":                a.Target,
			"consecutive_successes": a.Successes,
			"threshold":             a.Threshold,
			"timestamp":             a.Timestamp,
		}
	})

	// Heartbeat alerts
	hbSvc.SetAlertCallback(func(h *heartbeat.Heartbeat, alertType string, details map[string]interface{}) {
		isRecover := alertType == "recovery"
		severity := alert.SeverityCritical
		msg := fmt.Sprintf("Heartbeat '%s' missed deadline", h.Name)
		if isRecover {
			severity = alert.SeverityInfo
			msg = fmt.Sprintf("Heartbeat '%s' recovered", h.Name)
		}
		hbAlertType := "deadline_missed"
		if t, ok := details["alert_type"].(string); ok {
			hbAlertType = t
		}
		sendAlert(alert.Event{
			Source:     alert.SourceHeartbeat,
			AlertType:  hbAlertType,
			Severity:   severity,
			IsRecover:  isRecover,
			Message:    msg,
			EntityType: "heartbeat",
			EntityID:   h.ID,
			EntityName: h.Name,
			Details:    details,
			Timestamp:  time.Now(),
		})
	})

	// Certificate alerts
	certSvc.SetEventCallback(func(eventType string, data interface{}) {
		broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		m, ok := data.(map[string]interface{})
		if !ok {
			return
		}
		switch eventType {
		case "certificate.alert":
			certAlertType, _ := m["alert_type"].(string)
			sendAlert(alert.Event{
				Source:     alert.SourceCertificate,
				AlertType:  certAlertType,
				Severity:   alert.SeverityCritical,
				Message:    fmt.Sprintf("Certificate alert (%s) for %v:%v", certAlertType, m["hostname"], m["port"]),
				EntityType: "certificate",
				EntityID:   toInt64(m["monitor_id"]),
				EntityName: toString(m["hostname"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		case "certificate.recovery":
			sendAlert(alert.Event{
				Source:     alert.SourceCertificate,
				AlertType:  "expiring",
				Severity:   alert.SeverityInfo,
				IsRecover:  true,
				Message:    fmt.Sprintf("Certificate renewed for %v", m["hostname"]),
				EntityType: "certificate",
				EntityID:   toInt64(m["monitor_id"]),
				EntityName: toString(m["hostname"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		}
	})

	// Resource alerts
	resSvc.SetEventCallback(func(eventType string, data interface{}) {
		broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		m, ok := data.(map[string]interface{})
		if !ok {
			return
		}
		switch eventType {
		case "resource.alert":
			resAlertType, _ := m["alert_type"].(string)
			sendAlert(alert.Event{
				Source:     alert.SourceResource,
				AlertType:  resAlertType + "_threshold",
				Severity:   alert.SeverityWarning,
				Message:    fmt.Sprintf("Resource %s threshold exceeded for container %v", resAlertType, m["container_name"]),
				EntityType: "container",
				EntityID:   toInt64(m["container_id"]),
				EntityName: toString(m["container_name"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		case "resource.recovery":
			recoveredType, _ := m["recovered_type"].(string)
			sendAlert(alert.Event{
				Source:     alert.SourceResource,
				AlertType:  recoveredType + "_threshold",
				Severity:   alert.SeverityInfo,
				IsRecover:  true,
				Message:    fmt.Sprintf("Resource usage returned to normal for container %v", m["container_name"]),
				EntityType: "container",
				EntityID:   toInt64(m["container_id"]),
				EntityName: toString(m["container_name"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		}
	})

	// --- Top-level mux combining admin router, public status page, and SPA ---
	topMux := http.NewServeMux()
	statusHandler.Register(topMux)
	topMux.Handle("/api/", router.Handler())
	topMux.Handle("/ping/", router.Handler())
	topMux.Handle("/", spaHandler(router.Handler(), logger))

	srv := &http.Server{
		Addr:         addr,
		Handler:      withRequestTimeout(topMux, 10*time.Second),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0, // Disabled globally: SSE streams require unbounded writes; non-SSE routes use per-request timeouts via withRequestTimeout.
		IdleTimeout:  120 * time.Second,
	}

	// --- Startup: Reconciliation ---
	logger.Info("running startup container reconciliation")
	if err := svc.Reconcile(ctx, rt); err != nil {
		logger.Error("startup reconciliation failed", "error", err)
		// Non-fatal: continue with existing state
	}

	// Discover endpoint labels from all containers (Docker-specific: uses DiscoverAllWithLabels)
	if dr, ok := rt.(*docker.Runtime); ok {
		logger.Info("syncing endpoint labels from discovered containers")
		if results, err := dr.DiscoverAllWithLabels(ctx); err == nil {
			for _, r := range results {
				epSvc.SyncEndpoints(ctx, r.Container.Name, r.Container.ExternalID, r.Labels,
					r.Container.OrchestrationGroup, r.Container.OrchestrationUnit)
				certSvc.SyncFromLabels(ctx, r.Container.ExternalID, r.Labels)
			}
			logger.Info("endpoint discovery complete", "active_checks", checkEngine.ActiveCount())
		} else {
			logger.Error("endpoint label discovery failed", "error", err)
		}
	}

	// --- Background: Event stream ---
	eventCh := rt.StreamEvents(ctx)
	go func() {
		for evt := range eventCh {
			svc.ProcessEvent(ctx, container.ContainerEvent{
				Action:       evt.Action,
				ExternalID:   evt.ExternalID,
				Name:         evt.Name,
				ExitCode:     evt.ExitCode,
				HealthStatus: evt.HealthStatus,
				ErrorDetail:  evt.ErrorDetail,
				Timestamp:    evt.Timestamp,
				Labels:       evt.Labels,
			})

			// Endpoint lifecycle events
			switch evt.Action {
			case "start":
				name := evt.Name
				if len(name) > 0 && name[0] == '/' {
					name = name[1:]
				}
				epSvc.HandleContainerStart(ctx, name, evt.ExternalID, evt.Labels,
					evt.Labels["com.docker.compose.project"],
					evt.Labels["com.docker.compose.service"])
				certSvc.SyncFromLabels(ctx, evt.ExternalID, evt.Labels)
			case "stop", "die", "kill":
				epSvc.HandleContainerStop(ctx, evt.ExternalID)
			case "destroy":
				epSvc.HandleContainerDestroy(ctx, evt.ExternalID)
				certSvc.HandleContainerDestroy(ctx, evt.ExternalID)
			}
		}
	}()

	// --- Update intelligence ---
	registryClient := update.NewRegistryClient()
	updateScanner := update.NewScanner(registryClient, updateStore, logger)
	containerAdapter := update.NewContainerServiceAdapter(svc)
	updateSvc := update.NewService(updateStore, updateScanner, containerAdapter, logger)
	router.RegisterUpdateRoutes(updateSvc, updateStore)
	go updateSvc.Start(ctx)

	// --- Resource collector ---
	go resSvc.Start(ctx)

	// --- Certificate scheduler ---
	certSvc.StartScheduler(ctx)

	// --- Background: Retention cleanup ---
	sqlite.StartRetentionCleanupWithOpts(ctx, store, db, logger, sqlite.RetentionOpts{
		EndpointStore:    epStore,
		HeartbeatStore:   hbStore,
		CertificateStore: certStore,
		ResourceStore:    resStore,
	})

	// Alert retention cleanup (90 days)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				before := time.Now().Add(-90 * 24 * time.Hour)
				deleted, err := alertStore.DeleteAlertsOlderThan(ctx, before)
				if err != nil {
					logger.Error("alert retention cleanup failed", "error", err)
				} else if deleted > 0 {
					logger.Info("alert retention cleanup", "deleted", deleted)
				}
			}
		}
	}()

	// Update intelligence retention cleanup (30 days)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				before := time.Now().Add(-30 * 24 * time.Hour)
				deleted, err := updateStore.CleanupExpired(ctx, before)
				if err != nil {
					logger.Error("update retention cleanup failed", "error", err)
				} else if deleted > 0 {
					logger.Info("update retention cleanup", "deleted", deleted)
				}
			}
		}
	}()

	// --- Start HTTP server ---
	go func() {
		logger.Info("starting PulseBoard HTTP server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	// --- Wait for shutdown ---
	<-ctx.Done()
	logger.Info("shutting down PulseBoard")

	// Stop endpoint check engine
	epSvc.Stop()
	logger.Info("endpoint check engine stopped")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	}

	logger.Info("PulseBoard stopped")
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// spaHandler returns an http.Handler that serves the embedded SPA frontend.
// API and ping routes are delegated to the API handler; everything else is
// served from the embedded filesystem, with a fallback to index.html for
// client-side routing.
func spaHandler(apiHandler http.Handler, logger *slog.Logger) http.Handler {
	distFS, err := fs.Sub(web.FS, "dist")
	if err != nil {
		logger.Warn("SPA assets not embedded, frontend will not be served")
		return apiHandler
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// API and ping routes → delegate to API router
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ping/") {
			apiHandler.ServeHTTP(w, r)
			return
		}

		// Try to serve static file
		f, err := fs.Stat(distFS, strings.TrimPrefix(path, "/"))
		if err == nil && !f.IsDir() {
			// Immutable cache for hashed assets
			if strings.HasPrefix(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for client-side routing
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// isStreamingPath reports whether path corresponds to an SSE or streaming endpoint.
func isStreamingPath(path string) bool {
	// Exact matches first.
	if path == "/api/v1/containers/events" || path == "/status/events" {
		return true
	}
	// Log streaming: /api/v1/containers/{id}/logs/stream
	if strings.HasPrefix(path, "/api/v1/containers/") && strings.HasSuffix(path, "/logs/stream") {
		return true
	}
	return false
}

// withRequestTimeout wraps non-streaming handlers with http.TimeoutHandler so
// that ordinary REST requests are bounded even though the server-level
// WriteTimeout is disabled (required for SSE).
func withRequestTimeout(h http.Handler, timeout time.Duration) http.Handler {
	wrapped := http.TimeoutHandler(h, timeout, "request timeout")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isStreamingPath(r.URL.Path) {
			h.ServeHTTP(w, r)
			return
		}
		wrapped.ServeHTTP(w, r)
	})
}
