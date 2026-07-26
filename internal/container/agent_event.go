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

package container

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
)

// Docker label keys mirrored from internal/docker/discovery.go. Duplicated here
// (rather than imported) because internal/docker depends on this package.
const (
	labelComposeProject    = "com.docker.compose.project"
	labelComposeService    = "com.docker.compose.service"
	labelComposeWorkingDir = "com.docker.compose.project.working_dir"

	labelPBIgnore    = "maintenant.ignore"
	labelPBGroup     = "maintenant.group"
	labelPBSeverity  = "maintenant.alert.severity"
	labelPBThreshold = "maintenant.alert.restart_threshold"
	labelPBChannels  = "maintenant.alert.channels"
)

// HandleAgentEvent processes a ContainerEvent received from a remote agent.
//
// Unlike local discovery, agent containers never come through Reconcile, so this
// method owns their lifecycle: it INSERTs the container the first time the agent
// reports it (attributed via agent_id), then routes subsequent state changes
// through the normal pipeline for updates.
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.ContainerEvent) error {
	externalID := ev.GetContainerId()
	if externalID == "" {
		return nil
	}

	c, err := s.store.GetContainerByExternalID(ctx, externalID)
	if err != nil {
		return fmt.Errorf("agent event: lookup %s: %w", shortID(externalID), err)
	}

	// First time we see this container: insert it directly. The local reconcile
	// path only knows the server's own runtime, so agent containers must be
	// created here or they never reach the database.
	if c == nil {
		return s.insertAgentContainer(ctx, agentID, ev)
	}

	// Existing container: keep attribution and image fresh, then route the state
	// change through the normal pipeline (handleStateChange updates existing rows
	// correctly — the c == nil branch is never reached).
	dirty := false
	if c.AgentID != agentID {
		c.AgentID = agentID
		dirty = true
	}
	if img := ev.GetImage(); img != "" && img != c.Image {
		c.Image = img
		dirty = true
	}
	// The agent still sees it, so it is alive: resurrect a row archived while we
	// had lost sight of the container.
	if c.Archived {
		c.Archived = false
		c.ArchivedAt = nil
		dirty = true
	}
	if dirty {
		// Persist before ProcessEvent, which reloads the row from the store.
		if err := s.store.UpdateContainer(ctx, c); err != nil {
			return fmt.Errorf("agent event: update %s: %w", shortID(externalID), err)
		}
	}

	s.ProcessEvent(ctx, ContainerEvent{
		Action:     containerStateToAction(ev.GetState()),
		ExternalID: externalID,
		Name:       ev.GetName(),
		ExitCode:   ev.GetStatusMessage(),
		Timestamp:  agentEventTime(ev),
		Labels:     ev.GetLabels(),
	})
	return nil
}

// HandleAgentInventory reconciles a full container snapshot from a remote agent:
// every reported container is upserted (and un-archived), then any container we
// still hold for this agent but which the snapshot omits is archived.
//
// This is the only mechanism that retires a remote container — agents do not
// stream destroy events — and it is why an empty snapshot must be ignored: it
// would mean "archive this agent's whole fleet" on a transient discovery error.
func (s *Service) HandleAgentInventory(ctx context.Context, agentID string, ev *agentpb.ContainerInventory) error {
	reported := ev.GetContainers()
	if len(reported) == 0 {
		return nil
	}

	live := make(map[string]struct{}, len(reported))
	for _, c := range reported {
		externalID := c.GetContainerId()
		if externalID == "" {
			continue
		}
		live[externalID] = struct{}{}
		if err := s.HandleAgentEvent(ctx, agentID, c); err != nil {
			return err
		}
	}

	stored, err := s.store.ListContainers(ctx, ListContainersOpts{
		IncludeArchived: false, IncludeIgnored: true, AgentFilter: &agentID,
	})
	if err != nil {
		return fmt.Errorf("agent inventory: list stored: %w", err)
	}

	now := time.Now()
	for _, sc := range stored {
		if _, ok := live[sc.ExternalID]; ok {
			continue
		}
		if err := s.store.ArchiveContainer(ctx, sc.ExternalID, now); err != nil {
			s.logger.Error("agent inventory: archive", "external_id", shortID(sc.ExternalID), "error", err)
			continue
		}
		s.logger.Info("agent inventory: container gone, archived",
			"external_id", shortID(sc.ExternalID), "name", sc.Name, "agent_id", agentID)
		s.emitEvent(event.ContainerArchived, map[string]interface{}{
			"id": sc.ID, "archived_at": now, "agent_id": sc.AgentID,
		})
	}
	return nil
}

// insertAgentContainer creates a new container row reported by a remote agent,
// mirroring the field defaults of local discovery (docker.mapFromList).
func (s *Service) insertAgentContainer(ctx context.Context, agentID string, ev *agentpb.ContainerEvent) error {
	externalID := ev.GetContainerId()
	labels := ev.GetLabels()
	state := containerStateToState(ev.GetState())
	now := agentEventTime(ev)

	readyCount := 0
	if state == StateRunning {
		readyCount = 1
	}

	c := &Container{
		ExternalID:         externalID,
		AgentID:            agentID,
		Name:               ev.GetName(),
		Image:              ev.GetImage(),
		State:              state,
		OrchestrationGroup: labels[labelComposeProject],
		OrchestrationUnit:  labels[labelComposeService],
		ComposeWorkingDir:  labels[labelComposeWorkingDir],
		RuntimeType:        s.resolveAgentRuntime(ctx, agentID),
		PodCount:           1,
		ReadyCount:         readyCount,
		AlertSeverity:      SeverityWarning,
		RestartThreshold:   3,
		FirstSeenAt:        now,
		LastStateChangeAt:  now,
	}
	applyAgentLabels(c, labels)

	id, err := s.store.InsertContainer(ctx, c)
	if err != nil {
		// Likely a race with another event for the same external_id (UNIQUE
		// constraint on containers.external_id). If the row now exists, treat
		// the insert as a no-op; the follow-up event will carry the state.
		if existing, gerr := s.store.GetContainerByExternalID(ctx, externalID); gerr == nil && existing != nil {
			s.logger.Debug("agent event: insert raced, container already exists", "external_id", shortID(externalID))
			return nil
		}
		return fmt.Errorf("agent event: insert %s: %w", shortID(externalID), err)
	}
	c.ID = id

	// Record an initial transition so uptime tracking has a starting point,
	// skipping the no-op created→created case (mirrors Reconcile).
	if state != StateCreated {
		if _, err := s.store.InsertTransition(ctx, &StateTransition{
			ContainerID:   id,
			PreviousState: StateCreated,
			NewState:      state,
			Timestamp:     now,
		}); err != nil {
			s.logger.Error("agent event: initial transition", "container_id", id, "error", err)
		}
	}

	s.logger.Info("agent event: container discovered",
		"external_id", shortID(externalID), "name", c.Name, "agent_id", agentID, "state", string(state))
	s.emitEvent(event.ContainerDiscovered, c)
	return nil
}

// applyAgentLabels applies the maintenant.* labels to an agent container,
// mirroring docker.applyLabels for local/remote parity.
func applyAgentLabels(c *Container, labels map[string]string) {
	if v, ok := labels[labelPBIgnore]; ok && (v == "true" || v == "1") {
		c.IsIgnored = true
	}
	if v, ok := labels[labelPBGroup]; ok && v != "" {
		c.CustomGroup = v
	}
	if v, ok := labels[labelPBSeverity]; ok {
		switch AlertSeverity(v) {
		case SeverityCritical, SeverityWarning, SeverityInfo:
			c.AlertSeverity = AlertSeverity(v)
		}
	}
	if v, ok := labels[labelPBThreshold]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.RestartThreshold = n
		}
	}
	if v, ok := labels[labelPBChannels]; ok && v != "" {
		c.AlertChannels = v
	}
}

// agentEventTime returns the event's started_at when present, else now.
func agentEventTime(ev *agentpb.ContainerEvent) time.Time {
	if ts := ev.GetStartedAt(); ts != nil {
		return ts.AsTime()
	}
	return time.Now()
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// containerStateToState maps a proto ContainerState to the domain ContainerState
// (used when creating a container from an agent inventory/event).
func containerStateToState(state agentpb.ContainerState) ContainerState {
	switch state {
	case agentpb.ContainerState_CONTAINER_STATE_RUNNING:
		return StateRunning
	case agentpb.ContainerState_CONTAINER_STATE_EXITED:
		return StateExited
	case agentpb.ContainerState_CONTAINER_STATE_PAUSED:
		return StatePaused
	case agentpb.ContainerState_CONTAINER_STATE_RESTARTING:
		return StateRestarting
	case agentpb.ContainerState_CONTAINER_STATE_DEAD:
		return StateDead
	case agentpb.ContainerState_CONTAINER_STATE_CREATED:
		return StateCreated
	default:
		return StateRunning
	}
}

// containerStateToAction maps a proto ContainerState to the action string
// expected by ProcessEvent (mirrors the Docker event action vocabulary).
func containerStateToAction(state agentpb.ContainerState) string {
	switch state {
	case agentpb.ContainerState_CONTAINER_STATE_RUNNING:
		return "start"
	case agentpb.ContainerState_CONTAINER_STATE_EXITED:
		return "die"
	case agentpb.ContainerState_CONTAINER_STATE_PAUSED:
		return "pause"
	case agentpb.ContainerState_CONTAINER_STATE_RESTARTING:
		return "restart"
	case agentpb.ContainerState_CONTAINER_STATE_DEAD:
		return "die"
	case agentpb.ContainerState_CONTAINER_STATE_CREATED:
		return "create"
	default:
		return "start"
	}
}
