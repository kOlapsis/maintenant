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

package agentserver

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// ContainerHandler processes a container event from a remote agent.
type ContainerHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.ContainerEvent) error
}

// EndpointHandler processes an endpoint probe result from a remote agent.
type EndpointHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.EndpointEvent) error
}

// HeartbeatHandler processes a heartbeat ping from a remote agent.
type HeartbeatHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.HeartbeatEvent) error
}

// ResourceHandler processes a resource sample from a remote agent.
type ResourceHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.ResourceSample) error
}

// CertificateHandler processes a certificate scan result from a remote agent.
type CertificateHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.CertificateInfo) error
}

// SwarmTopologyHandler processes a full swarm topology snapshot from an agent.
type SwarmTopologyHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.SwarmTopology) error
}

// KubernetesTopologyHandler processes a full Kubernetes topology snapshot from an agent.
type KubernetesTopologyHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.KubernetesTopology) error
}

// LabelSyncFunc provisions label-discovered endpoint/cert monitors for a remote
// agent's container. Invoked for every container event so monitors track label
// changes: created on first sight, deprovisioned when a label is removed. The
// agent itself probes them and pushes the results.
type LabelSyncFunc func(ctx context.Context, agentID, containerName, externalID string, labels map[string]string)

// DispatchDeps groups the optional per-domain handlers the dispatcher calls.
// A nil handler means that event type is silently ignored.
type DispatchDeps struct {
	Container   ContainerHandler
	Endpoint    EndpointHandler
	Heartbeat   HeartbeatHandler
	Resource    ResourceHandler
	Certificate CertificateHandler
	Swarm       SwarmTopologyHandler
	Kubernetes  KubernetesTopologyHandler
	// LabelSync, if set, provisions endpoint/cert monitors from a container's
	// labels after each container event. Optional (nil = no label discovery).
	LabelSync LabelSyncFunc
}

// Dispatcher routes AgentEvents to the appropriate domain handler.
type Dispatcher struct {
	deps DispatchDeps
}

// NewDispatcher creates a Dispatcher with the given handler set.
func NewDispatcher(deps DispatchDeps) *Dispatcher {
	return &Dispatcher{deps: deps}
}

// Dispatch routes evt to the handler matching its body type.
// Returns an error if a handler is wired and returns an error.
// Silently ignores events whose handler is nil.
func (d *Dispatcher) Dispatch(ctx context.Context, evt *agentpb.AgentEvent) error {
	agentID := evt.GetAgentId()

	switch body := evt.GetBody().(type) {
	case *agentpb.AgentEvent_Container:
		if d.deps.Container != nil {
			if err := d.deps.Container.HandleAgentEvent(ctx, agentID, body.Container); err != nil {
				return fmt.Errorf("dispatch container event: %w", err)
			}
		}
		if d.deps.LabelSync != nil {
			d.deps.LabelSync(ctx, agentID, body.Container.GetName(), body.Container.GetContainerId(), body.Container.GetLabels())
		}
	case *agentpb.AgentEvent_Endpoint:
		if d.deps.Endpoint != nil {
			if err := d.deps.Endpoint.HandleAgentEvent(ctx, agentID, body.Endpoint); err != nil {
				return fmt.Errorf("dispatch endpoint event: %w", err)
			}
		}
	case *agentpb.AgentEvent_Heartbeat:
		if d.deps.Heartbeat != nil {
			if err := d.deps.Heartbeat.HandleAgentEvent(ctx, agentID, body.Heartbeat); err != nil {
				return fmt.Errorf("dispatch heartbeat event: %w", err)
			}
		}
	case *agentpb.AgentEvent_Resource:
		if d.deps.Resource != nil {
			if err := d.deps.Resource.HandleAgentEvent(ctx, agentID, body.Resource); err != nil {
				return fmt.Errorf("dispatch resource event: %w", err)
			}
		}
	case *agentpb.AgentEvent_Certificate:
		if d.deps.Certificate != nil {
			if err := d.deps.Certificate.HandleAgentEvent(ctx, agentID, body.Certificate); err != nil {
				return fmt.Errorf("dispatch certificate event: %w", err)
			}
		}
	case *agentpb.AgentEvent_Swarm:
		if d.deps.Swarm != nil {
			if err := d.deps.Swarm.HandleAgentEvent(ctx, agentID, body.Swarm); err != nil {
				return fmt.Errorf("dispatch swarm topology: %w", err)
			}
		}
	case *agentpb.AgentEvent_Kubernetes:
		if d.deps.Kubernetes != nil {
			if err := d.deps.Kubernetes.HandleAgentEvent(ctx, agentID, body.Kubernetes); err != nil {
				return fmt.Errorf("dispatch kubernetes topology: %w", err)
			}
		}
	}
	return nil
}
