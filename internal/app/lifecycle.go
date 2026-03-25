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
	"time"

	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/event"
	pbruntime "github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/store/sqlite"
)

// reconcile performs startup reconciliation and endpoint/security discovery.
func (a *App) reconcile(ctx context.Context) {
	a.logger.Info("running startup container reconciliation")
	if err := a.containerSvc.Reconcile(ctx, a.rt); err != nil {
		a.logger.Error("startup reconciliation failed", "error", err)
	}

	// Prune orphan alerts
	if activeAlerts, err := a.alertStore.ListActiveAlerts(ctx); err == nil {
		for _, al := range activeAlerts {
			if al.EntityType != "container" {
				continue
			}
			c, err := a.containerSvc.GetContainer(ctx, al.EntityID)
			if err != nil || c == nil {
				a.alertEngine.ResolveByEntity(ctx, "container", al.EntityID)
				a.logger.Info("pruned orphan container alert", "alert_id", al.ID, "entity_id", al.EntityID)
			}
		}
	}

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

	// Swarm node reconciliation on startup (Enterprise).
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
			for _, r := range results {
				a.endpointSvc.SyncEndpoints(ctx, r.Container.Name, r.Container.ExternalID, r.Labels,
					r.Container.OrchestrationGroup, r.Container.OrchestrationUnit)
				a.certSvc.SyncFromLabels(ctx, r.Container.ExternalID, r.Labels)

				dbC := dbByExtID[r.Container.ExternalID]
				if r.SecurityConfig != nil && dbC != nil && dbC.ID > 0 {
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
			a.logger.Info("endpoint discovery complete", "active_checks", a.checkEngine.ActiveCount())
		} else {
			a.logger.Error("endpoint label discovery failed", "error", err)
		}
	}
}

// startEventStream consumes runtime events and dispatches to services.
func (a *App) startEventStream(ctx context.Context) {
	eventCh := a.rt.StreamEvents(ctx)
	go func() {
		for evt := range eventCh {
			// Route Swarm service/node events to the Swarm event processor.
			if evt.ResourceType == pbruntime.ResourceService || evt.ResourceType == pbruntime.ResourceNode {
				if a.swarmEvents != nil {
					a.swarmEvents.ProcessEvent(ctx, evt)

					// On service update, check rolling update status (Enterprise).
					if evt.ResourceType == pbruntime.ResourceService && evt.Action == "update" && a.swarmUpdateTracker != nil {
						go a.swarmUpdateTracker.CheckService(ctx, evt.ExternalID)
					}
				}
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

				// Feed Swarm task failures to crash-loop detector (Enterprise).
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
}

// startNodeRefresh runs periodic Swarm node reconciliation (Enterprise, 60s).
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
		}
	}
}

// startRetentionCleanup starts background retention cleanup goroutines.
func (a *App) startRetentionCleanup(ctx context.Context) {
	// Core store retention cleanup
	sqlite.StartRetentionCleanupWithOpts(ctx, a.containerStore, a.db, a.logger, sqlite.RetentionOpts{
		EndpointStore:    a.epStore,
		HeartbeatStore:   a.hbStore,
		CertificateStore: a.certStore,
		ResourceStore:    a.resStore,
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
}
