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
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/uid"
)

// localTopologyReconcileInterval is how often the server re-snapshots its own
// runtime's topology into the per-agent store under the LocalAgent id.
const localTopologyReconcileInterval = 30 * time.Second

// pruneOrphanAlerts resolves active alerts whose underlying entity no longer
// exists: a container that was removed, an agent deleted while already offline,
// or a heartbeat/endpoint/certificate monitor deleted before its alert was
// resolved. Idempotent, safe to run on every startup.
func (a *App) pruneOrphanAlerts(ctx context.Context) {
	activeAlerts, err := a.alertStore.ListActiveAlerts(ctx)
	if err != nil {
		return
	}
	for _, al := range activeAlerts {
		var orphan bool
		switch al.EntityType {
		case "container":
			c, err := a.containerSvc.GetContainer(ctx, al.EntityID)
			orphan = err != nil || c == nil
		case "agent":
			_, err := a.agentStore.Get(ctx, al.EntityID)
			orphan = errors.Is(err, agent.ErrAgentNotFound)
		case "heartbeat":
			h, err := a.hbStore.GetHeartbeatByID(ctx, al.EntityID)
			orphan = err == nil && h == nil
		case "endpoint":
			ep, err := a.epStore.GetEndpointByID(ctx, al.EntityID)
			orphan = err == nil && ep == nil
		case "certificate":
			m, err := a.certStore.GetMonitorByID(ctx, al.EntityID)
			orphan = err == nil && m == nil
		}
		if orphan {
			a.alertEngine.ResolveByEntity(ctx, al.EntityType, al.EntityID)
			a.logger.Info("pruned orphan alert", "alert_id", al.ID, "entity_type", al.EntityType, "entity_id", al.EntityID)
		}
	}
}

// reconcile performs startup reconciliation and endpoint/security discovery.
// Must only be called when the runtime is connected.
func (a *App) reconcile(ctx context.Context) {
	if !a.rt.IsConnected() {
		return
	}
	a.logger.Info("running startup container reconciliation")
	if err := a.containerSvc.Reconcile(ctx, a.rt); err != nil {
		a.logger.Error("startup reconciliation failed", "error", err)
	}

	a.pruneOrphanAlerts(ctx)

	// Swarm service discovery on startup.
	if a.swarmDiscovery != nil {
		a.logger.Info("running Swarm service discovery")
		_, services, err := a.swarmDiscovery.DiscoverAll(ctx)
		if err != nil {
			a.logger.Error("Swarm service discovery failed", "error", err)
		} else {
			a.logger.Info("Swarm discovery complete", "services", len(services))
		}
	}

	// Swarm node reconciliation on startup (Pro).
	if a.swarmNodeSvc != nil {
		a.logger.Info("running Swarm node reconciliation")
		if err := a.swarmNodeSvc.Reconcile(ctx); err != nil {
			a.logger.Error("Swarm node reconciliation failed", "error", err)
		} else {
			a.logger.Info("Swarm node reconciliation complete")
		}
	}

	// Discover endpoint labels and security insights
	if dr, ok := a.rt.(*docker.Runtime); ok {
		a.logger.Info("syncing endpoint labels from discovered containers")
		if results, err := dr.DiscoverAllWithLabels(ctx); err == nil {
			dbContainers, _ := a.containerSvc.ListContainers(ctx, container.ListContainersOpts{IncludeIgnored: true})
			dbByExtID := make(map[string]*container.Container, len(dbContainers))
			for _, c := range dbContainers {
				dbByExtID[c.ExternalID] = c
			}

			now := time.Now()
			seen := make(map[string]struct{}, len(results))
			for _, r := range results {
				seen[r.Container.ExternalID] = struct{}{}
				a.endpointSvc.SyncEndpoints(ctx, r.Container.Name, r.Container.ExternalID, r.Labels,
					r.Container.OrchestrationGroup, r.Container.OrchestrationUnit)
				a.certSvc.SyncFromLabels(ctx, r.Container.ExternalID, r.Labels)

				dbC := dbByExtID[r.Container.ExternalID]
				if r.SecurityConfig != nil && dbC != nil && dbC.ID != "" {
					bindings := make([]security.PortBinding, 0, len(r.SecurityConfig.PortBindings))
					for _, pb := range r.SecurityConfig.PortBindings {
						bindings = append(bindings, security.PortBinding{
							HostIP:   pb.HostIP,
							HostPort: pb.HostPort,
							Port:     pb.ContainerPort,
							Protocol: pb.Protocol,
						})
					}
					insights := security.AnalyzeDocker(dbC.ID, dbC.Name, security.DockerSecurityConfig{
						Privileged:  r.SecurityConfig.Privileged,
						NetworkMode: r.SecurityConfig.NetworkMode,
						Bindings:    bindings,
					}, now)
					a.securitySvc.UpdateContainer(dbC.ID, dbC.Name, insights)
				}
			}
			if swept := a.endpointSvc.SweepOrphanedLabelEndpoints(ctx, seen); swept > 0 {
				a.logger.Info("retired endpoints whose container is gone", "count", swept)
			}
			a.logger.Info("endpoint discovery complete", "active_checks", a.checkEngine.ActiveCount())
		} else {
			a.logger.Error("endpoint label discovery failed", "error", err)
		}
	}
}

// startEventStream consumes runtime events and dispatches to services.
// Returns a channel that closes when the event stream ends (daemon disconnected).
func (a *App) startEventStream(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	eventCh := a.rt.StreamEvents(ctx)
	go func() {
		defer close(done)
		for evt := range eventCh {
			// Route Swarm service/node events to the Swarm event processor.
			if evt.ResourceType == runtime.ResourceService || evt.ResourceType == runtime.ResourceNode {
				if a.swarmEvents != nil {
					a.swarmEvents.ProcessEvent(ctx, evt)

					// On service update, check rolling update status (Pro).
					if evt.ResourceType == runtime.ResourceService && evt.Action == "update" && a.swarmUpdateTracker != nil {
						go a.swarmUpdateTracker.CheckService(ctx, evt.ExternalID)
					}
				}
				continue
			}

			// A `docker compose run` container carries the service's labels under
			// a generated name; monitoring it would duplicate the service it was
			// run from. Discovery skips them, and so does the event stream.
			if docker.IsOneOff(evt.Labels) {
				continue
			}

			a.containerSvc.ProcessEvent(ctx, container.ContainerEvent{
				Action:       evt.Action,
				ExternalID:   evt.ExternalID,
				Name:         evt.Name,
				ExitCode:     evt.ExitCode,
				HealthStatus: evt.HealthStatus,
				ErrorDetail:  evt.ErrorDetail,
				Timestamp:    evt.Timestamp,
				Labels:       evt.Labels,
			})

			switch evt.Action {
			case "start":
				name := evt.Name
				if len(name) > 0 && name[0] == '/' {
					name = name[1:]
				}
				a.endpointSvc.HandleContainerStart(ctx, name, evt.ExternalID, evt.Labels,
					evt.Labels["com.docker.compose.project"],
					evt.Labels["com.docker.compose.service"])
				a.certSvc.SyncFromLabels(ctx, evt.ExternalID, evt.Labels)

				if dr, ok := a.rt.(*docker.Runtime); ok {
					go ScanContainerSecurity(ctx, dr, a.containerSvc, a.securitySvc, evt.ExternalID, a.logger)
				}
			case "stop", "die", "kill":
				a.endpointSvc.HandleContainerStop(ctx, evt.ExternalID)

				// Feed Swarm task failures to crash-loop detector (Pro).
				if evt.Action == "die" && a.swarmCrashLoop != nil {
					if svcID, ok := evt.Labels["com.docker.swarm.service.id"]; ok && svcID != "" {
						svcName := evt.Labels["com.docker.swarm.service.name"]
						a.swarmCrashLoop.RecordFailure(svcID, svcName, evt.ErrorDetail)

						// Emit task_failed SSE event.
						a.broker.Broadcast(v1.SSEEvent{
							Type: event.SwarmTaskFailed,
							Data: map[string]interface{}{
								"service_id":   svcID,
								"service_name": svcName,
								"container_id": evt.ExternalID,
								"error":        evt.ErrorDetail,
								"exit_code":    evt.ExitCode,
								"timestamp":    evt.Timestamp.Format(time.RFC3339),
							},
						})
					}
				}
			case "destroy":
				a.endpointSvc.HandleContainerDestroy(ctx, evt.ExternalID)
				a.certSvc.HandleContainerDestroy(ctx, evt.ExternalID)
			}
		}
	}()
	return done
}

// startNodeRefresh runs periodic Swarm node reconciliation (Pro, 60s).
func (a *App) startNodeRefresh(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.swarmNodeSvc.Reconcile(ctx); err != nil {
				a.logger.Warn("periodic node reconciliation failed", "error", err)
			}
			// Check crash-loop recoveries.
			if a.swarmCrashLoop != nil {
				a.swarmCrashLoop.CheckRecoveries()
			}
			// Check sustained under-replication.
			if a.swarmReplicaChecker != nil && a.swarmDiscovery != nil {
				a.swarmReplicaChecker.Check(a.swarmDiscovery.ListServices())
			}
		}
	}
}

// startKubernetesReconcile periodically snapshots the server's own Kubernetes
// runtime into the per-agent store under the LocalAgent id, so the store-backed
// Workloads/Pods/Nodes views reflect the local cluster the same way they reflect
// remote agents. No-op unless the local runtime is Kubernetes.
func (a *App) startKubernetesReconcile(ctx context.Context) {
	src, ok := a.rt.(kubernetes.SnapshotSource)
	if !ok || a.k8sIngest == nil {
		return
	}
	reconcile := func() {
		snap, err := kubernetes.SnapshotFromRuntime(ctx, src)
		if err != nil {
			a.logger.Warn("local kubernetes reconcile: snapshot failed", "error", err)
			return
		}
		if err := a.k8sIngest.Reconcile(ctx, uid.LocalAgent, snap); err != nil {
			a.logger.Warn("local kubernetes reconcile: store failed", "error", err)
		}
	}
	reconcile()
	ticker := time.NewTicker(localTopologyReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

// startSwarmTopologyReconcile periodically snapshots the server's own Swarm
// services and tasks into the per-agent store under the LocalAgent id. Nodes are
// left to the Pro NodeService to avoid two writers fighting over the same
// rows. No-op unless this server is a swarm manager.
func (a *App) startSwarmTopologyReconcile(ctx context.Context) {
	if a.swarmDiscovery == nil || a.swarmIngest == nil {
		return
	}
	dr, ok := a.rt.(*docker.Runtime)
	if !ok {
		return
	}
	reconcile := func() {
		snap, err := swarm.SnapshotFromClient(ctx, a.swarmDiscovery, dr.Client())
		if err != nil {
			a.logger.Warn("local swarm reconcile: snapshot failed", "error", err)
			return
		}
		if err := a.swarmIngest.ReconcileServicesTasks(ctx, uid.LocalAgent, snap); err != nil {
			a.logger.Warn("local swarm reconcile: store failed", "error", err)
		}
	}
	reconcile()
	ticker := time.NewTicker(localTopologyReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

// startRetentionCleanup starts background retention cleanup goroutines.
func (a *App) startRetentionCleanup(ctx context.Context) {
	// Core store retention cleanup
	store.StartRetentionCleanupWithOpts(ctx, a.containerStore, a.db, a.logger, store.RetentionOpts{
		EndpointStore:    a.epStore,
		HeartbeatStore:   a.hbStore,
		CertificateStore: a.certStore,
		ResourceStore:    a.resStore,
		Config: store.RetentionConfig{
			Snapshots: a.cfg.Retention.Snapshots,
			Interval:  a.cfg.Retention.Interval,
			BatchSize: a.cfg.Retention.BatchSize,
		},
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
				deleted, err := a.alertStore.DeleteAlertsOlderThan(ctx, before)
				if err != nil {
					a.logger.Error("alert retention cleanup failed", "error", err)
				} else if deleted > 0 {
					a.logger.Info("alert retention cleanup", "deleted", deleted)
				}
			}
		}
	}()

	// Update retention cleanup (30 days)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				before := time.Now().Add(-30 * 24 * time.Hour)
				deleted, err := a.updateStore.CleanupExpired(ctx, before)
				if err != nil {
					a.logger.Error("update retention cleanup failed", "error", err)
				} else if deleted > 0 {
					a.logger.Info("update retention cleanup", "deleted", deleted)
				}
			}
		}
	}()

	// Escalation run retention (90 days, nightly at 03:00).
	if a.escalationSvc != nil {
		go func() {
			a.escalationSvc.RunRetentionLoop(ctx)
			a.logger.ErrorContext(ctx, "escalation: retention loop exited unexpectedly")
		}()
	}
}

// startSwarmRecheck periodically re-checks Swarm mode and broadcasts context changes.
func (a *App) startSwarmRecheck(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed, result, err := a.swarmDetector.Recheck(ctx)
			if err != nil {
				a.logger.Warn("swarm recheck failed", "error", err)
				continue
			}
			if !changed {
				continue
			}

			now := time.Now().UTC().Format(time.RFC3339)
			var previousCtx, newCtx, message string

			if result.Active && result.IsManager {
				// Swarm activated.
				previousCtx = "docker"
				newCtx = "swarm"
				message = "Swarm cluster detected — dashboard adapted."

				a.swarmCluster = &swarm.SwarmCluster{
					ID:        result.ClusterID,
					IsManager: true,
				}

				// Create swarm discovery and event processor if needed.
				if a.swarmDiscovery == nil {
					if dr, ok := a.rt.(*docker.Runtime); ok {
						a.swarmDiscovery = swarm.NewServiceDiscovery(dr.Client(), a.logger)
						a.swarmDiscovery.SetNetworkResolver(func(ctx context.Context, networkID string) (string, string, error) {
							net, err := dr.Client().NetworkInspect(ctx, networkID)
							if err != nil {
								return "", "", err
							}
							return net.Name, net.Scope, nil
						})
						a.swarmEvents = swarm.NewEventProcessor(a.swarmDiscovery, a.logger)

						// Run initial discovery.
						_, services, err := a.swarmDiscovery.DiscoverAll(ctx)
						if err != nil {
							a.logger.Error("initial Swarm discovery after activation failed", "error", err)
						} else {
							a.logger.Info("Swarm discovery after activation complete", "services", len(services))
						}
					}
				}
			} else {
				// Swarm deactivated.
				previousCtx = "swarm"
				newCtx = "docker"
				message = "Swarm cluster deactivated — switched to Docker mode."

				a.swarmCluster = nil
			}

			a.logger.Info("runtime context changed",
				"previous", previousCtx,
				"current", newCtx,
			)

			a.broker.Broadcast(v1.SSEEvent{
				Type: event.RuntimeContextChanged,
				Data: map[string]interface{}{
					"previous":    previousCtx,
					"current":     newCtx,
					"message":     message,
					"detected_at": now,
				},
			})
		}
	}
}

// wireContainerMonitoring câbles la surveillance conteneur pour un cycle de connexion :
// réconciliation initiale et flux d'événements.
// Retourne un canal fermé quand le flux d'événements se termine (perte daemon).
// Appelée exactement une fois par cycle de connexion (la garde est le superviseur).
func (a *App) wireContainerMonitoring(ctx context.Context) <-chan struct{} {
	a.reconcile(ctx)
	return a.startEventStream(ctx)
}

// broadcastRuntimeAvailability diffuse l'état runtime courant via SSE.
func (a *App) broadcastRuntimeAvailability() {
	a.broker.Broadcast(v1.SSEEvent{
		Type: event.RuntimeAvailabilityChanged,
		Data: map[string]interface{}{
			"name":      a.rt.Name(),
			"connected": a.rt.IsConnected(),
		},
	})
}

// startRuntimeSupervisor orchestre la surveillance du runtime en tâche de fond.
// Si connecté au boot : câble immédiatement, puis supervise la perte.
// Si dégradé au boot : goroutine de reconnexion de fond (ConnectWithRetry).
// Garantie : wireContainerMonitoring est appelée exactement une fois par cycle de connexion.
func (a *App) startRuntimeSupervisor(ctx context.Context) {
	a.broadcastRuntimeAvailability()

	if a.rt.IsConnected() {
		// Comportement nominal : câblage immédiat + supervision de fond.
		streamDone := a.wireContainerMonitoring(ctx)
		go a.supervisorLoop(ctx, streamDone)
	} else {
		// Dégradé au boot : reconnexion de fond.
		a.logger.Info("container runtime unavailable, monitoring suspended", "runtime", a.rt.Name())
		go a.supervisorLoop(ctx, nil)
	}
}

// supervisorLoop gère le cycle reconnexion → câblage → détection de perte.
// streamDone est non-nil si une connexion est déjà active (fermeture = perte).
func (a *App) supervisorLoop(ctx context.Context, streamDone <-chan struct{}) {
	lossNotify := streamDone

	for {
		if lossNotify != nil {
			// Attendre la perte du daemon ou l'annulation du contexte.
			select {
			case <-ctx.Done():
				return
			case <-lossNotify:
				// T025 : flux fermé = daemon perdu.
				a.rt.SetDisconnected()
				a.broadcastRuntimeAvailability()
				a.logger.Warn("container runtime lost, entering degraded mode", "runtime", a.rt.Name())
			}
		}

		// Tentative de reconnexion (avec retry de fond, respecte ctx.Done).
		if err := a.rt.Connect(ctx); err != nil {
			// ctx annulé — arrêt propre.
			return
		}

		// T026 : garde idempotente par cycle — le superviseur lui-même garantit
		// qu'on passe ici une seule fois par reconnexion.
		a.broadcastRuntimeAvailability()
		a.logger.Info("container runtime reconnected, resuming container monitoring", "runtime", a.rt.Name())
		lossNotify = a.wireContainerMonitoring(ctx)
	}
}
