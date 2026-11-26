// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/uid"
)

const containerStartGrace = 5 * time.Minute

// ContainerStatus derives the status page status for a container, a major outage when stale says nothing vouches for its last known state.
func ContainerStatus(c *container.Container, stale bool, now time.Time) string {
	if stale {
		return status.StatusMajorOutage
	}
	switch c.State {
	case container.StateRunning:
		if c.HealthStatus != nil && *c.HealthStatus == container.HealthUnhealthy {
			return status.StatusDegraded
		}
		return status.StatusOperational
	case container.StateCompleted:
		return status.StatusOperational
	case container.StateCreated:
		if now.Sub(c.LastStateChangeAt) < containerStartGrace {
			return status.StatusOperational
		}
		return status.StatusMajorOutage
	default:
		return status.StatusMajorOutage
	}
}

// containerStale reports whether the runtime or the agent behind agentID is gone: the local runtime does not answer, the agent's runtime is unreachable, or the agent is disconnected past the stale threshold with its agent-offline alert open.
func (a *App) containerStale(ctx context.Context, agentID string) bool {
	if uid.Agent(agentID) == uid.LocalAgent {
		return a.rt == nil || !a.rt.IsConnected()
	}
	if a.agentSessions == nil {
		return true
	}
	if a.agentSessions.IsConnected(agentID) {
		return a.agentSessions.RuntimeUnreachable(agentID)
	}
	ag, err := a.agentStore.Get(ctx, agentID)
	if err != nil {
		return true
	}
	stale := time.Duration(a.cfg.MultiHost.AgentStaleThresholdSeconds) * time.Second
	if stale == 0 {
		stale = 60 * time.Second
	}
	if v1.ConnectionState(a.agentSessions, stale, agentID, ag.LastSeenAt) != "disconnected" {
		return false
	}
	// The offline alert waits out the reconnection grace that follows a server start.
	open, err := a.alertStore.GetActiveAlert(ctx, alert.SourceAgent, "disconnected", "agent", agentID)
	return err != nil || open != nil
}

// EndpointStatus derives the status page status for an endpoint.
func EndpointStatus(ep *endpoint.Endpoint) string {
	switch ep.Status {
	case endpoint.StatusUp:
		return status.StatusOperational
	case endpoint.StatusDown:
		return status.StatusMajorOutage
	case endpoint.StatusDegraded:
		// Serving, but over a certificate we do not trust: not an outage.
		return status.StatusDegraded
	default:
		if ep.LastCheckAt != nil || ep.LastError != "" {
			return status.StatusMajorOutage
		}
		return status.StatusOperational
	}
}

// HeartbeatStatus derives the status page status for a heartbeat.
func HeartbeatStatus(hb *heartbeat.Heartbeat) string {
	if hb.Status == heartbeat.StatusDown {
		return status.StatusMajorOutage
	}
	return status.StatusOperational
}

// CertificateStatus derives the status page status for a certificate monitor.
func CertificateStatus(cert *certificate.CertMonitor) string {
	switch cert.Status {
	case certificate.StatusValid:
		return status.StatusOperational
	case certificate.StatusExpiring:
		return status.StatusDegraded
	case certificate.StatusUnknown:
		if cert.LastCheckAt == nil && cert.LastError == "" {
			return status.StatusOperational
		}
		return status.StatusMajorOutage
	default:
		return status.StatusMajorOutage
	}
}

// wireStatusProvider sets up the monitor status provider for the status page.
func (a *App) wireStatusProvider() {
	a.statusSvc.SetMonitorStatusProvider(func(ctx context.Context, monitorType string, monitorID string) string {
		switch monitorType {
		case "container":
			c, err := a.containerSvc.GetContainer(ctx, monitorID)
			if err != nil || c == nil {
				return status.StatusOperational
			}
			return ContainerStatus(c, a.containerStale(ctx, c.AgentID), time.Now())
		case "endpoint":
			ep, err := a.endpointSvc.GetEndpoint(ctx, monitorID)
			if err != nil || ep == nil || ep.Limited || !ep.Active {
				return status.StatusOperational
			}
			return EndpointStatus(ep)
		case "heartbeat":
			hb, err := a.heartbeatSvc.GetHeartbeat(ctx, monitorID)
			if err != nil || hb == nil {
				return status.StatusOperational
			}
			return HeartbeatStatus(hb)
		case "certificate":
			cert, err := a.certSvc.GetMonitor(ctx, monitorID)
			if err != nil || cert == nil || cert.Limited || cert.InactiveSince != nil {
				return status.StatusOperational
			}
			return CertificateStatus(cert)
		}
		return status.StatusOperational
	})

	a.wireMonitorPopulationProvider()
	a.wireMonitorNameProvider()
}

// wireMonitorPopulationProvider sets up the monitor population provider for match-all components.
func (a *App) wireMonitorPopulationProvider() {
	a.statusSvc.SetMonitorPopulationProvider(func(ctx context.Context, monitorType string) []status.MonitorRef {
		switch monitorType {
		case "container":
			containers, err := a.containerSvc.ListContainers(ctx, container.ListContainersOpts{})
			if err != nil {
				return nil
			}
			replaced := container.ReplacedSwarmTasks(containers)
			refs := make([]status.MonitorRef, 0, len(containers))
			for _, c := range containers {
				if replaced[c.ID] {
					continue
				}
				refs = append(refs, status.MonitorRef{Type: "container", ID: c.ID, Name: c.Name})
			}
			return refs
		case "endpoint":
			endpoints, err := a.endpointSvc.ListEndpoints(ctx, endpoint.ListEndpointsOpts{})
			if err != nil {
				return nil
			}
			refs := make([]status.MonitorRef, 0, len(endpoints))
			for _, ep := range endpoints {
				if ep.Limited {
					continue
				}
				refs = append(refs, status.MonitorRef{Type: "endpoint", ID: ep.ID, Name: ep.Target})
			}
			return refs
		case "heartbeat":
			heartbeats, err := a.heartbeatSvc.ListHeartbeats(ctx, heartbeat.ListHeartbeatsOpts{})
			if err != nil {
				return nil
			}
			refs := make([]status.MonitorRef, 0, len(heartbeats))
			for _, h := range heartbeats {
				refs = append(refs, status.MonitorRef{Type: "heartbeat", ID: h.ID, Name: h.Name})
			}
			return refs
		case "certificate":
			certs, err := a.certSvc.ListMonitors(ctx, certificate.ListCertificatesOpts{})
			if err != nil {
				return nil
			}
			refs := make([]status.MonitorRef, 0, len(certs))
			for _, c := range certs {
				if c.Limited {
					continue
				}
				refs = append(refs, status.MonitorRef{Type: "certificate", ID: c.ID, Name: fmt.Sprintf("%s:%d", c.Hostname, c.Port)})
			}
			return refs
		}
		return nil
	})
}

// wireMonitorNameProvider sets up the monitor name provider for enriching monitor refs.
func (a *App) wireMonitorNameProvider() {
	a.statusSvc.SetMonitorNameProvider(func(ctx context.Context, monitorType string, monitorID string) string {
		switch monitorType {
		case "container":
			c, err := a.containerSvc.GetContainer(ctx, monitorID)
			if err != nil || c == nil {
				return ""
			}
			return c.Name
		case "endpoint":
			ep, err := a.endpointSvc.GetEndpoint(ctx, monitorID)
			if err != nil || ep == nil {
				return ""
			}
			return ep.Target
		case "heartbeat":
			hb, err := a.heartbeatSvc.GetHeartbeat(ctx, monitorID)
			if err != nil || hb == nil {
				return ""
			}
			return hb.Name
		case "certificate":
			cert, err := a.certSvc.GetMonitor(ctx, monitorID)
			if err != nil || cert == nil {
				return ""
			}
			return fmt.Sprintf("%s:%d", cert.Hostname, cert.Port)
		}
		return ""
	})
}
