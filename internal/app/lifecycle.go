// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

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
	"github.com/kolapsis/maintenant/internal/retry"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/uid"
)

// localTopologyReconcileInterval is how often the server re-snapshots its own
// runtime's topology into the per-agent store under the LocalAgent id.
const localTopologyReconcileInterval = 30 * time.Second

// Bounds of the delay between two attempts to reach a runtime whose Connect gives up.
const (
	runtimeRetryMin = time.Second
	runtimeRetryMax = time.Minute
)

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

// seedRestartAlertTracking hands the containers with an open restart_loop alert to the recovery loop.
func (a *App) seedRestartAlertTracking(ctx context.Context) {
	activeAlerts, err := a.alertStore.ListActiveAlerts(ctx)
	if err != nil {
		a.logger.Error("seed restart alert tracking", "error", err)
		return
	}
	var ids []string
	for _, al := range activeAlerts {
		if al.EntityType == "container" && al.AlertType == restartLoopAlertType {
			ids = append(ids, al.EntityID)
		}
	}
	a.containerSvc.TrackRestartAlerts(ids)
}

// seedKubernetesAlertTracking hands the active Kubernetes alerts to the checker,
// so the local cluster's reconcile loop resolves those whose condition is gone.
func (a *App) seedKubernetesAlertTracking(ctx context.Context) {
	activeAlerts, err := a.alertStore.ListActiveAlerts(ctx)
	if err != nil {
		a.logger.Error("seed kubernetes alert tracking", "error", err)
		return
	}
	a.k8sAlerts.Resume(activeAlerts)
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

	if m := a.swarmMgr.Load(); m != nil {
		a.logger.Info("running Swarm service discovery")
		services, err := m.discovery.DiscoverAll(ctx)
		if err != nil {
			a.logger.Error("Swarm service discovery failed", "error", err)
		} else {
			a.logger.Info("Swarm discovery complete", "services", len(services))
		}

		a.logger.Info("running Swarm node reconciliation")
		if err := m.nodeSvc.Reconcile(ctx); err != nil {
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
				a.endpointSvc.SyncEndpoints(ctx, r.Container.Name, r.Container.ExternalID, r.Labels)
				a.certSvc.SyncFromLabels(ctx, r.Container.ExternalID, r.Labels)

				dbC := dbByExtID[r.Container.ExternalID]
				if r.SecurityConfig != nil && dbC != nil && dbC.ID != "" {
					a.securitySvc.UpdateContainer(dbC.ID, dbC.Name, dockerInsights(dbC, r.SecurityConfig, now))
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
			a.dispatchRuntimeEvent(ctx, evt)
		}
	}()
	return done
}

// dispatchRuntimeEvent hands one runtime event to the services it concerns.
func (a *App) dispatchRuntimeEvent(ctx context.Context, evt runtime.RuntimeEvent) {
	m := a.swarmMgr.Load()

	if evt.ResourceType == runtime.ResourceService || evt.ResourceType == runtime.ResourceNode {
		if m != nil {
			m.events.ProcessEvent(ctx, evt)
			if evt.ResourceType == runtime.ResourceService && evt.Action == "update" {
				go m.updateTracker.CheckService(ctx, evt.ExternalID)
			}
		}
		return
	}

	// A `docker compose run` container carries the service's labels under
	// a generated name; monitoring it would duplicate the service it was
	// run from. Discovery skips them, and so does the event stream.
	if docker.IsOneOff(evt.Labels) {
		return
	}

	a.containerSvc.ProcessEvent(ctx, container.ContainerEvent{
		Action:       evt.Action,
		AgentID:      uid.LocalAgent,
		ExternalID:   evt.ExternalID,
		Name:         evt.Name,
		ExitCode:     evt.ExitCode,
		OOMKilled:    evt.OOMKilled,
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
		a.endpointSvc.HandleContainerStart(ctx, name, evt.ExternalID, evt.Labels)
		a.certSvc.SyncFromLabels(ctx, evt.ExternalID, evt.Labels)

		if dr, ok := a.rt.(*docker.Runtime); ok {
			go ScanContainerSecurity(ctx, dr, a.containerSvc, a.securitySvc, evt.ExternalID, a.logger)
		}
	case "stop", "die", "kill":
		a.endpointSvc.HandleContainerStop(ctx, evt.ExternalID)

		if svcID, svcName, ok := swarmTaskFailure(evt); ok && m != nil {
			m.crashLoop.RecordFailure(svcID, svcName, evt.ErrorDetail)
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
	case "destroy":
		a.endpointSvc.HandleContainerDestroy(ctx, evt.ExternalID)
		a.certSvc.HandleContainerDestroy(ctx, evt.ExternalID)
	}
}

// swarmTaskFailure returns the service of a Swarm task that died, unless the
// task is ignored.
func swarmTaskFailure(evt runtime.RuntimeEvent) (serviceID, serviceName string, ok bool) {
	serviceID = evt.Labels["com.docker.swarm.service.id"]
	if evt.Action != "die" || serviceID == "" || container.IgnoredByLabels(evt.Labels) {
		return "", "", false
	}
	return serviceID, evt.Labels["com.docker.swarm.service.name"], true
}

// startSwarmManager resumes the Swarm alerts left by a previous run, then starts
// the manager's periodic loops.
func (a *App) startSwarmManager(ctx context.Context, m *swarmManager) {
	a.seedSwarmAlertTracking(ctx, m)
	go a.startNodeRefresh(ctx, m)
	go a.startSwarmTopologyReconcile(ctx, m)
}

// seedSwarmAlertTracking hands the active Swarm alerts to the services that
// resolve them.
func (a *App) seedSwarmAlertTracking(ctx context.Context, m *swarmManager) {
	activeAlerts, err := a.alertStore.ListActiveAlerts(ctx)
	if err != nil {
		a.logger.Error("seed swarm alert tracking", "error", err)
		return
	}
	m.replicaChecker.Resume(activeAlerts)
	m.crashLoop.Resume(activeAlerts)
	m.updateTracker.Resume(activeAlerts)
	m.nodeSvc.Resume(activeAlerts)
}

// startNodeRefresh runs periodic Swarm node reconciliation and alert checks (60s).
func (a *App) startNodeRefresh(ctx context.Context, m *swarmManager) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.nodeSvc.Reconcile(ctx); err != nil {
				a.logger.Warn("periodic node reconciliation failed", "error", err)
			}
			m.crashLoop.CheckRecoveries()
			m.replicaChecker.Check(m.discovery.ListServices())
		}
	}
}

// containerDownCheckInterval is how often the container-down sweep runs. It is
// the alert's resolution, not its threshold: the threshold comes from the
// operator's MAINTENANT_CONTAINER_DOWN_AFTER.
const containerDownCheckInterval = 30 * time.Second

// startContainerDownCheck sweeps for containers stopped past the configured
// threshold. The first pass waits one tick so the initial runtime reconcile has
// refreshed states the store may have been holding since the last shutdown.
func (a *App) startContainerDownCheck(ctx context.Context) {
	ticker := time.NewTicker(containerDownCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.downDetector.Check(ctx); err != nil {
				a.logger.Warn("container down check failed", "error", err)
			}
		}
	}
}

// startKubernetesReconcile periodically snapshots the server's own Kubernetes
// runtime into the per-agent store under the LocalAgent id, so the store-backed
// Workloads/Pods/Nodes views reflect the local cluster the same way they reflect
// remote agents. It runs for one connection cycle and stops when until closes.
func (a *App) startKubernetesReconcile(ctx context.Context, src kubernetes.SnapshotSource, until <-chan struct{}) {
	reconcile := func() {
		snap, err := kubernetes.SnapshotFromRuntime(ctx, src)
		if err != nil {
			a.logger.Warn("local kubernetes reconcile: snapshot failed", "error", err)
			return
		}
		if err := a.k8sIngest.Reconcile(ctx, uid.LocalAgent, snap); err != nil {
			a.logger.Warn("local kubernetes reconcile: store failed", "error", err)
		}
		a.k8sAlerts.Check(snap)
		if exposures, ok := a.rt.(serviceExposureSource); ok {
			ScanKubernetesSecurity(ctx, exposures, a.containerSvc, a.securitySvc, a.logger)
		}
	}
	reconcile()
	ticker := time.NewTicker(localTopologyReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-until:
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

// startSwarmTopologyReconcile periodically snapshots the server's own Swarm
// services and tasks into the per-agent store under the LocalAgent id. Nodes are
// left to the NodeService to avoid two writers fighting over the same
// rows.
func (a *App) startSwarmTopologyReconcile(ctx context.Context, m *swarmManager) {
	dr, ok := a.rt.(*docker.Runtime)
	if !ok {
		return
	}
	reconcile := func() {
		snap, err := swarm.SnapshotFromClient(ctx, m.discovery, dr.Client())
		if err != nil {
			a.logger.Warn("local swarm reconcile: snapshot failed", "error", err)
			return
		}
		if err := a.swarmIngest.ReconcileServicesTasks(ctx, uid.LocalAgent, snap); err != nil {
			a.logger.Warn("local swarm reconcile: store failed", "error", err)
		}
		a.pruneSwarmAlerts(ctx, snap)
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

// pruneSwarmAlerts resolves the active alerts of Swarm services and nodes the
// manager no longer lists.
func (a *App) pruneSwarmAlerts(ctx context.Context, snap swarm.TopologySnapshot) {
	services := make(map[string]bool, len(snap.Services))
	for _, s := range snap.Services {
		services[s.ServiceID] = true
	}
	nodes := make(map[string]bool, len(snap.Nodes))
	for _, n := range snap.Nodes {
		nodes[n.NodeID] = true
	}
	activeAlerts, err := a.alertStore.ListActiveAlerts(ctx)
	if err != nil {
		a.logger.Warn("prune swarm alerts", "error", err)
		return
	}
	type entity struct{ kind, id string }
	gone := make(map[entity]bool)
	for _, al := range activeAlerts {
		var isGone bool
		switch al.EntityType {
		case "swarm_service":
			isGone = !services[al.EntityID]
		case "swarm_node":
			// A manager always lists itself: no node at all means the list failed.
			isGone = len(snap.Nodes) > 0 && !nodes[al.EntityID]
		}
		if isGone {
			gone[entity{al.EntityType, al.EntityID}] = true
		}
	}
	for e := range gone {
		a.alertEngine.ResolveByEntity(ctx, e.kind, e.id)
	}
}

// startRetentionCleanup starts background retention cleanup goroutines.
func (a *App) startRetentionCleanup(ctx context.Context) {
	// Core store retention cleanup
	a.retentionStopped = store.StartRetentionCleanupWithOpts(ctx, a.containerStore, a.db, a.logger, store.RetentionOpts{
		EndpointStore:    a.epStore,
		HeartbeatStore:   a.hbStore,
		CertificateStore: a.certStore,
		ResourceStore:    a.resStore,
		UptimeStore:      a.uptimeStore,
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
		go a.escalationSvc.RunRetentionLoop(ctx)
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
			if changed {
				a.applySwarmContext(ctx, result)
			}
		}
	}
}

// applySwarmContext follows a Swarm activation or deactivation detected while
// the server runs, and announces it.
func (a *App) applySwarmContext(ctx context.Context, result swarm.DetectionResult) {
	var previousCtx, newCtx, message string

	if result.Active && result.IsManager {
		previousCtx = "docker"
		newCtx = "swarm"
		message = "Swarm cluster detected — dashboard adapted."

		a.swarmCluster.Store(&swarm.SwarmCluster{
			ID:        result.ClusterID,
			IsManager: true,
		})
		if a.swarmMgr.Load() == nil {
			if dr, ok := a.rt.(*docker.Runtime); ok {
				a.activateSwarm(ctx, dr)
			}
		}
	} else {
		previousCtx = "swarm"
		newCtx = "docker"
		message = "Swarm cluster deactivated — switched to Docker mode."

		a.swarmCluster.Store(nil)
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
			"detected_at": time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// activateSwarm builds, wires and starts the Swarm manager services when Swarm
// is enabled while the server runs, as New and Start do at boot.
func (a *App) activateSwarm(ctx context.Context, dr *docker.Runtime) {
	m := newSwarmManager(dr, a.swarmNodeStore, a.logger)
	a.wireSwarmCallbacks(m)
	a.swarmMgr.Store(m)

	services, err := m.discovery.DiscoverAll(ctx)
	if err != nil {
		a.logger.Error("initial Swarm discovery after activation failed", "error", err)
	} else {
		a.logger.Info("Swarm discovery after activation complete", "services", len(services))
	}

	a.startSwarmManager(ctx, m)
}

// wireContainerMonitoring câbles la surveillance conteneur pour un cycle de connexion :
// réconciliation initiale et flux d'événements.
// Retourne un canal fermé quand le flux d'événements se termine (perte daemon).
// Appelée exactement une fois par cycle de connexion (la garde est le superviseur).
func (a *App) wireContainerMonitoring(ctx context.Context) <-chan struct{} {
	a.reconcile(ctx)
	streamDone := a.startEventStream(ctx)
	if src, ok := a.rt.(kubernetes.SnapshotSource); ok {
		go a.startKubernetesReconcile(ctx, src, streamDone)
	}
	return streamDone
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
		go a.supervisorLoop(ctx, streamDone, retry.New(runtimeRetryMin, runtimeRetryMax, 0))
	} else {
		// Dégradé au boot : reconnexion de fond.
		a.logger.Info("container runtime unavailable, monitoring suspended", "runtime", a.rt.Name())
		go a.supervisorLoop(ctx, nil, retry.New(runtimeRetryMin, runtimeRetryMax, 0))
	}
}

// supervisorLoop gère le cycle reconnexion → câblage → détection de perte.
// streamDone est non-nil si une connexion est déjà active (fermeture = perte).
func (a *App) supervisorLoop(ctx context.Context, streamDone <-chan struct{}, backoff *retry.Backoff) {
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
			if ctx.Err() != nil {
				return
			}
			delay := backoff.Next()
			a.logger.Warn("container runtime connection failed, retrying",
				"runtime", a.rt.Name(), "error", err, "retry_in", delay)
			lossNotify = nil
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}
		backoff.Reset()

		// T026 : garde idempotente par cycle — le superviseur lui-même garantit
		// qu'on passe ici une seule fois par reconnexion.
		a.broadcastRuntimeAvailability()
		a.logger.Info("container runtime reconnected, resuming container monitoring", "runtime", a.rt.Name())
		lossNotify = a.wireContainerMonitoring(ctx)
	}
}
