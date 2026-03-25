// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package swarm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/docker/docker/api/types/swarm"

	cmodel "github.com/kolapsis/maintenant/internal/container"
)

// ServiceClient abstracts Docker SDK calls needed for Swarm service discovery.
type ServiceClient interface {
	ServiceList(ctx context.Context) ([]swarm.Service, error)
	ServiceInspect(ctx context.Context, serviceID string) (swarm.Service, error)
	TaskList(ctx context.Context) ([]swarm.Task, error)
	NodeList(ctx context.Context) ([]swarm.Node, error)
}

// NetworkResolver resolves network IDs to names and scopes.
type NetworkResolver func(ctx context.Context, networkID string) (name, scope string, err error)

// ServiceDiscovery handles Swarm service discovery and mapping to Container models.
type ServiceDiscovery struct {
	client          ServiceClient
	networkResolver NetworkResolver
	logger          *slog.Logger

	mu           sync.RWMutex
	services     map[string]*SwarmService // keyed by service ID
	networkCache map[string]NetworkAttachment
}

// NewServiceDiscovery creates a new Swarm service discovery instance.
func NewServiceDiscovery(client ServiceClient, logger *slog.Logger) *ServiceDiscovery {
	return &ServiceDiscovery{
		client:       client,
		logger:       logger,
		services:     make(map[string]*SwarmService),
		networkCache: make(map[string]NetworkAttachment),
	}
}

// SetNetworkResolver sets the function used to resolve network names from IDs.
func (sd *ServiceDiscovery) SetNetworkResolver(resolver NetworkResolver) {
	sd.networkResolver = resolver
}

// DiscoverAll discovers all Swarm services and maps them to Container models.
// Returns containers representing Swarm tasks with Swarm fields populated.
func (sd *ServiceDiscovery) DiscoverAll(ctx context.Context) ([]*cmodel.Container, []*SwarmService, error) {
	services, err := sd.client.ServiceList(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("discover services: %w", err)
	}

	tasks, err := sd.client.TaskList(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("discover tasks: %w", err)
	}

	// Build node hostname map for task placement.
	nodeHostnames := make(map[string]string)
	nodes, err := sd.client.NodeList(ctx)
	if err != nil {
		sd.logger.Warn("failed to list nodes for hostname resolution", "error", err)
	} else {
		for _, n := range nodes {
			nodeHostnames[n.ID] = n.Description.Hostname
		}
	}

	// Group tasks by service ID.
	tasksByService := make(map[string][]swarm.Task)
	for _, t := range tasks {
		tasksByService[t.ServiceID] = append(tasksByService[t.ServiceID], t)
	}

	now := time.Now()
	var containers []*cmodel.Container
	swarmServices := make([]*SwarmService, 0, len(services))

	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Clear old services.
	sd.services = make(map[string]*SwarmService, len(services))

	for _, svc := range services {
		ss := mapService(svc)

		serviceTasks := tasksByService[svc.ID]
		runningCount := 0
		for _, t := range serviceTasks {
			if t.Status.State == swarm.TaskStateRunning {
				runningCount++
			}
		}
		ss.RunningReplicas = runningCount

		sd.resolveNetworks(ctx, ss)
		sd.services[svc.ID] = ss
		swarmServices = append(swarmServices, ss)

		// Map tasks to containers.
		for _, t := range serviceTasks {
			// Only map tasks with an active desired state.
			if t.DesiredState != swarm.TaskStateRunning && t.DesiredState != swarm.TaskStateShutdown {
				continue
			}

			c := mapTaskToContainer(svc, t, ss, nodeHostnames, now)
			containers = append(containers, c)
		}
	}

	sd.logger.Info("discovered Swarm services",
		"services", len(services),
		"tasks", len(containers))

	return containers, swarmServices, nil
}

// GetService returns a cached Swarm service by ID.
func (sd *ServiceDiscovery) GetService(serviceID string) *SwarmService {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.services[serviceID]
}

// ListServices returns all cached Swarm services.
func (sd *ServiceDiscovery) ListServices() []*SwarmService {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	result := make([]*SwarmService, 0, len(sd.services))
	for _, s := range sd.services {
		result = append(result, s)
	}
	return result
}

// RefreshService re-inspects a single service and updates the cache.
func (sd *ServiceDiscovery) RefreshService(ctx context.Context, serviceID string) (*SwarmService, error) {
	svc, err := sd.client.ServiceInspect(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("refresh service %s: %w", serviceID, err)
	}

	ss := mapService(svc)

	// Count running tasks.
	tasks, err := sd.client.TaskList(ctx)
	if err == nil {
		running := 0
		for _, t := range tasks {
			if t.ServiceID == serviceID && t.Status.State == swarm.TaskStateRunning {
				running++
			}
		}
		ss.RunningReplicas = running
	}

	sd.resolveNetworks(ctx, ss)

	sd.mu.Lock()
	sd.services[serviceID] = ss
	sd.mu.Unlock()

	return ss, nil
}

// resolveNetworks resolves network IDs to names using the network resolver.
func (sd *ServiceDiscovery) resolveNetworks(ctx context.Context, svc *SwarmService) {
	if sd.networkResolver == nil {
		return
	}
	for i, n := range svc.Networks {
		if n.NetworkName != "" {
			continue
		}
		// Check cache first.
		if cached, ok := sd.networkCache[n.NetworkID]; ok {
			svc.Networks[i] = cached
			continue
		}
		name, scope, err := sd.networkResolver(ctx, n.NetworkID)
		if err != nil {
			sd.logger.Debug("failed to resolve network", "network_id", n.NetworkID, "error", err)
			continue
		}
		resolved := NetworkAttachment{
			NetworkID:   n.NetworkID,
			NetworkName: name,
			Scope:       scope,
		}
		sd.networkCache[n.NetworkID] = resolved
		svc.Networks[i] = resolved
	}
}

// RemoveService removes a service from the cache.
// GetTasksForService returns tasks for a given service by querying the Docker API.
func (sd *ServiceDiscovery) GetTasksForService(serviceID string) []*SwarmTask {
	ctx := context.Background()
	allTasks, err := sd.client.TaskList(ctx)
	if err != nil {
		sd.logger.Warn("failed to list tasks for service", "service_id", serviceID, "error", err)
		return nil
	}

	var result []*SwarmTask
	for _, t := range allTasks {
		if t.ServiceID != serviceID {
			continue
		}
		if t.DesiredState != swarm.TaskStateRunning && t.DesiredState != swarm.TaskStateShutdown {
			continue
		}
		st := &SwarmTask{
			TaskID:       t.ID,
			ServiceID:    t.ServiceID,
			NodeID:       t.NodeID,
			Slot:         t.Slot,
			State:        string(t.Status.State),
			DesiredState: string(t.DesiredState),
			Timestamp:    t.Status.Timestamp,
		}
		if t.Status.ContainerStatus != nil {
			st.ContainerID = t.Status.ContainerStatus.ContainerID
			if t.Status.ContainerStatus.ExitCode != 0 {
				code := t.Status.ContainerStatus.ExitCode
				st.ExitCode = &code
			}
		}
		if t.Status.Err != "" {
			st.Error = t.Status.Err
		}
		result = append(result, st)
	}
	return result
}

func (sd *ServiceDiscovery) RemoveService(serviceID string) {
	sd.mu.Lock()
	delete(sd.services, serviceID)
	sd.mu.Unlock()
}

func mapService(svc swarm.Service) *SwarmService {
	ss := &SwarmService{
		ServiceID: svc.ID,
		Name:      svc.Spec.Name,
		Labels:    svc.Spec.Labels,
		StackName: svc.Spec.Labels[labelStackNamespace],
		CreatedAt: svc.CreatedAt,
	}

	// Image.
	if svc.Spec.TaskTemplate.ContainerSpec != nil {
		ss.Image = svc.Spec.TaskTemplate.ContainerSpec.Image
	}

	// Mode.
	if svc.Spec.Mode.Replicated != nil {
		ss.Mode = "replicated"
		if svc.Spec.Mode.Replicated.Replicas != nil {
			ss.DesiredReplicas = int(*svc.Spec.Mode.Replicated.Replicas)
		}
	} else if svc.Spec.Mode.Global != nil {
		ss.Mode = "global"
	}

	// Ports.
	if svc.Endpoint.Ports != nil {
		for _, p := range svc.Endpoint.Ports {
			ss.Ports = append(ss.Ports, PortConfig{
				Protocol:      string(p.Protocol),
				TargetPort:    p.TargetPort,
				PublishedPort: p.PublishedPort,
				PublishMode:   string(p.PublishMode),
			})
		}
	}

	// Networks.
	for _, n := range svc.Spec.TaskTemplate.Networks {
		ss.Networks = append(ss.Networks, NetworkAttachment{
			NetworkID: n.Target,
		})
	}

	// Update status (Enterprise feature, but captured always).
	if svc.UpdateStatus != nil && svc.UpdateStatus.State != "" {
		us := &UpdateStatus{
			State:   string(svc.UpdateStatus.State),
			Message: svc.UpdateStatus.Message,
		}
		if svc.UpdateStatus.StartedAt != nil && !svc.UpdateStatus.StartedAt.IsZero() {
			us.StartedAt = svc.UpdateStatus.StartedAt
		}
		if svc.UpdateStatus.CompletedAt != nil && !svc.UpdateStatus.CompletedAt.IsZero() {
			us.CompletedAt = svc.UpdateStatus.CompletedAt
		}
		ss.UpdateStatus = us
	}

	return ss
}

func mapTaskToContainer(svc swarm.Service, task swarm.Task, ss *SwarmService, nodeHostnames map[string]string, now time.Time) *cmodel.Container {
	containerID := ""
	if task.Status.ContainerStatus != nil {
		containerID = task.Status.ContainerStatus.ContainerID
	}

	name := svc.Spec.Name
	if task.Slot > 0 {
		name = fmt.Sprintf("%s.%d", svc.Spec.Name, task.Slot)
	}

	state := mapTaskState(task.Status.State)
	readyCount := 0
	if state == cmodel.StateRunning {
		readyCount = 1
	}

	c := &cmodel.Container{
		ExternalID:           containerID,
		Name:                 name,
		Image:                ss.Image,
		State:                state,
		RuntimeType:          "docker",
		ControllerKind:       "swarm-service",
		OrchestrationUnit:    svc.Spec.Name,
		PodCount:             1,
		ReadyCount:           readyCount,
		AlertSeverity:        cmodel.SeverityWarning,
		RestartThreshold:     3,
		FirstSeenAt:          now,
		LastStateChangeAt:    task.Status.Timestamp,
		SwarmServiceID:       svc.ID,
		SwarmServiceName:     svc.Spec.Name,
		SwarmServiceMode:     ss.Mode,
		SwarmNodeID:          task.NodeID,
		SwarmTaskSlot:        task.Slot,
		SwarmDesiredReplicas: ss.DesiredReplicas,
	}

	// Set error detail from task errors.
	if task.Status.Err != "" {
		c.ErrorDetail = task.Status.Err
	}

	// Apply service-level labels.
	ApplyServiceLabels(c, svc.Spec.Labels)

	return c
}

func mapTaskState(state swarm.TaskState) cmodel.ContainerState {
	switch state {
	case swarm.TaskStateRunning:
		return cmodel.StateRunning
	case swarm.TaskStateComplete:
		return cmodel.StateCompleted
	case swarm.TaskStateFailed, swarm.TaskStateRejected:
		return cmodel.StateExited
	case swarm.TaskStateShutdown:
		return cmodel.StateExited
	case swarm.TaskStateNew, swarm.TaskStatePending, swarm.TaskStateAssigned,
		swarm.TaskStateAccepted, swarm.TaskStatePreparing, swarm.TaskStateStarting,
		swarm.TaskStateReady:
		return cmodel.StateCreated
	default:
		return cmodel.StateCreated
	}
}
