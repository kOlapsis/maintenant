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
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
)

// EventMeta is the observation time and the replay flag of an agent event.
type EventMeta = agentevent.Meta

const (
	maxClockSkew      = 60 * time.Second
	maxObservationAge = 24 * time.Hour
)

// ContainerHandler processes a container event from a remote agent.
type ContainerHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.ContainerEvent, meta EventMeta) error
}

// ContainerInventoryHandler reconciles a full container snapshot from an agent.
type ContainerInventoryHandler interface {
	HandleAgentInventory(ctx context.Context, agentID string, ev *agentpb.ContainerInventory, meta EventMeta) error
}

// EndpointHandler processes an endpoint probe result from a remote agent.
type EndpointHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.EndpointEvent, meta EventMeta) error
}

// HeartbeatHandler processes a heartbeat ping from a remote agent.
type HeartbeatHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.HeartbeatEvent) error
}

// ResourceHandler processes a resource sample from a remote agent.
type ResourceHandler interface {
	HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.ResourceSample, meta EventMeta) error
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
	Inventory   ContainerInventoryHandler
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

	mu       sync.Mutex
	rejected map[string]uint64
}

// NewDispatcher creates a Dispatcher with the given handler set.
func NewDispatcher(deps DispatchDeps) *Dispatcher {
	return &Dispatcher{deps: deps, rejected: make(map[string]uint64)}
}

// RejectedEvents returns how many events the dispatcher refused for an out of
// range observation time since startup, for agentID.
func (d *Dispatcher) RejectedEvents(agentID string) uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rejected[agentID]
}

func (d *Dispatcher) countRejection(agentID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rejected[agentID]++
}

// eventMeta reads the observation time carried by evt. An event without one
// comes from an agent predating the field: it falls back to the receive time.
// An out of range time is refused outright, never clamped onto a bound.
func eventMeta(evt *agentpb.AgentEvent, now time.Time) (EventMeta, error) {
	meta := EventMeta{ObservedAt: now, Replayed: evt.GetReplayed()}

	ts := evt.GetObservedAt()
	if ts == nil || (ts.GetSeconds() == 0 && ts.GetNanos() == 0) {
		return meta, nil
	}

	observed := ts.AsTime()
	if observed.After(now.Add(maxClockSkew)) {
		return meta, fmt.Errorf("observed_at %s is more than %s ahead of the server clock",
			observed.Format(time.RFC3339Nano), maxClockSkew)
	}
	if observed.Before(now.Add(-maxObservationAge)) {
		return meta, fmt.Errorf("observed_at %s is older than the %s retention window",
			observed.Format(time.RFC3339Nano), maxObservationAge)
	}

	meta.ObservedAt = observed
	return meta, nil
}

// Dispatch routes evt to the handler matching its body type, attributing every
// event to agentID — the identity proven by the auth handshake, NOT the
// client-controlled agent_id carried on the wire. Returns an error if a handler
// is wired and returns an error. Silently ignores events whose handler is nil.
func (d *Dispatcher) Dispatch(ctx context.Context, agentID string, evt *agentpb.AgentEvent) error {
	// The wire agent_id is attacker-controlled. Trusting it would let a
	// compromised agent forge or wipe data for any other host. Reject a
	// mismatch outright; the authenticated agentID is the only source of truth.
	if claimed := evt.GetAgentId(); claimed != "" && claimed != agentID {
		return fmt.Errorf("dispatch: event agent_id %q does not match authenticated agent %q", claimed, agentID)
	}

	meta, err := eventMeta(evt, time.Now())
	if err != nil {
		d.countRejection(agentID)
		return fmt.Errorf("dispatch: reject event from agent %q: %w", agentID, err)
	}

	switch body := evt.GetBody().(type) {
	case *agentpb.AgentEvent_Container:
		if d.deps.Container != nil {
			if err := d.deps.Container.HandleAgentEvent(ctx, agentID, body.Container, meta); err != nil {
				return fmt.Errorf("dispatch container event: %w", err)
			}
		}
		if d.deps.LabelSync != nil {
			labels := body.Container.GetLabels()
			if body.Container.GetDestroyed() {
				labels = nil
			}
			d.deps.LabelSync(ctx, agentID, body.Container.GetName(), body.Container.GetContainerId(), labels)
		}
	case *agentpb.AgentEvent_Inventory:
		if d.deps.Inventory != nil {
			if err := d.deps.Inventory.HandleAgentInventory(ctx, agentID, body.Inventory, meta); err != nil {
				return fmt.Errorf("dispatch container inventory: %w", err)
			}
		}
		if d.deps.LabelSync != nil {
			for _, c := range body.Inventory.GetContainers() {
				d.deps.LabelSync(ctx, agentID, c.GetName(), c.GetContainerId(), c.GetLabels())
			}
		}
	case *agentpb.AgentEvent_Endpoint:
		if d.deps.Endpoint != nil {
			if err := d.deps.Endpoint.HandleAgentEvent(ctx, agentID, body.Endpoint, meta); err != nil {
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
			if err := d.deps.Resource.HandleAgentEvent(ctx, agentID, body.Resource, meta); err != nil {
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
