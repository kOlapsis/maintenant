// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
)

// RuntimeDetails is what a runtime reports about a container at scan time and the store does not keep.
type RuntimeDetails struct {
	Labels       map[string]string
	RepoDigests  []string // "repo@sha256:..." of the image the container runs
	DigestsKnown bool     // the runtime reported RepoDigests; known and empty marks an image never pulled from nor pushed to a registry
	PodContainer string   // Kubernetes: the pod container that runs the image
}

// DetailsFetcher maps each container of the server's own runtime, by external ID, to its RuntimeDetails.
type DetailsFetcher interface {
	FetchDetails(ctx context.Context) (map[string]RuntimeDetails, error)
}

// ContainerServiceAdapter adapts container.Service to the ContainerLister interface.
type ContainerServiceAdapter struct {
	svc     *container.Service
	details DetailsFetcher // optional, nil when the local runtime reports nothing beyond the stored fields
	logger  *slog.Logger
}

// NewContainerServiceAdapter creates a new adapter.
func NewContainerServiceAdapter(svc *container.Service, logger *slog.Logger) *ContainerServiceAdapter {
	return &ContainerServiceAdapter{svc: svc, logger: logger}
}

// WithDetailsFetcher attaches the local runtime source of labels, image digests and pod container names.
func (a *ContainerServiceAdapter) WithDetailsFetcher(df DetailsFetcher) *ContainerServiceAdapter {
	a.details = df
	return a
}

// ListContainerInfos returns container info for the running containers, leaving out those whose runtime has not reported their labels.
func (a *ContainerServiceAdapter) ListContainerInfos(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := a.svc.ListContainers(ctx, container.ListContainersOpts{
		StateFilter: string(container.StateRunning),
	})
	if err != nil {
		return nil, err
	}

	local := a.localDetails(ctx)
	infos := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		if c.IsIgnored || c.Archived {
			continue
		}
		d, ok := a.detailsFor(c, local)
		if !ok {
			continue
		}
		infos = append(infos, newContainerInfo(c, d))
	}
	return infos, nil
}

// GetContainerInfo returns container metadata for a single container by external ID.
func (a *ContainerServiceAdapter) GetContainerInfo(ctx context.Context, externalID string) (ContainerInfo, error) {
	containers, err := a.svc.ListContainers(ctx, container.ListContainersOpts{})
	if err != nil {
		return ContainerInfo{}, fmt.Errorf("get container info: %w", err)
	}

	for _, c := range containers {
		if c.ExternalID != externalID {
			continue
		}
		var local map[string]RuntimeDetails
		if uid.Agent(c.AgentID) == uid.LocalAgent {
			local = a.localDetails(ctx)
		}
		d, _ := a.detailsFor(c, local)
		return newContainerInfo(c, d), nil
	}
	return ContainerInfo{}, fmt.Errorf("container not found: %s", externalID)
}

// localDetails fetches the details of the local runtime's containers, nil when no fetcher is wired or the runtime did not answer.
func (a *ContainerServiceAdapter) localDetails(ctx context.Context) map[string]RuntimeDetails {
	if a.details == nil {
		return nil
	}
	details, err := a.details.FetchDetails(ctx)
	if err != nil {
		a.logger.Warn("update: local container details unavailable, their containers are left out", "error", err)
		return nil
	}
	return details
}

func (a *ContainerServiceAdapter) detailsFor(c *container.Container, local map[string]RuntimeDetails) (RuntimeDetails, bool) {
	if uid.Agent(c.AgentID) != uid.LocalAgent {
		d, ok := a.svc.AgentDetails(c.AgentID, c.ExternalID)
		return RuntimeDetails{Labels: d.UpdateLabels, RepoDigests: d.RepoDigests, DigestsKnown: d.DigestsKnown}, ok
	}
	if a.details == nil {
		return RuntimeDetails{}, true
	}
	d, ok := local[c.ExternalID]
	return d, ok
}

func newContainerInfo(c *container.Container, d RuntimeDetails) ContainerInfo {
	return ContainerInfo{
		UID:                c.ID,
		AgentID:            uid.Agent(c.AgentID),
		ExternalID:         c.ExternalID,
		Name:               c.Name,
		Image:              c.Image,
		Labels:             d.Labels,
		OrchestrationGroup: c.OrchestrationGroup,
		OrchestrationUnit:  c.OrchestrationUnit,
		RuntimeType:        c.RuntimeType,
		ControllerKind:     c.ControllerKind,
		ComposeWorkingDir:  c.ComposeWorkingDir,
		RepoDigests:        d.RepoDigests,
		LocallyBuilt:       d.DigestsKnown && len(d.RepoDigests) == 0,
		PodContainer:       d.PodContainer,
		SwarmService:       c.SwarmServiceName,
	}
}
