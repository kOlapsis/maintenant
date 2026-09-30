// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/container"
)

// LabelFetcher retrieves raw container labels from the runtime.
// Returns a map of externalID -> labels. Implemented by docker.Runtime via a thin adapter.
// Returns nil (not an error) when the runtime doesn't support label fetching (e.g. Kubernetes).
type LabelFetcher interface {
	FetchLabels(ctx context.Context) (map[string]map[string]string, error)
}

// RepoDigestFetcher maps each container external ID to the repo digests of its image, an empty list marking an image that never came from a registry.
type RepoDigestFetcher interface {
	FetchRepoDigests(ctx context.Context) (map[string][]string, error)
}

// ContainerServiceAdapter adapts container.Service to the ContainerLister interface.
type ContainerServiceAdapter struct {
	svc           *container.Service
	labelFetcher  LabelFetcher      // optional, nil when runtime doesn't support label fetching
	digestFetcher RepoDigestFetcher // optional, nil when the runtime does not report image digests
}

// NewContainerServiceAdapter creates a new adapter.
func NewContainerServiceAdapter(svc *container.Service) *ContainerServiceAdapter {
	return &ContainerServiceAdapter{svc: svc}
}

// WithLabelFetcher attaches a runtime label fetcher to the adapter.
// When set, ContainerInfo.Labels is populated with live runtime labels at scan time.
func (a *ContainerServiceAdapter) WithLabelFetcher(lf LabelFetcher) *ContainerServiceAdapter {
	a.labelFetcher = lf
	return a
}

// WithRepoDigestFetcher attaches the runtime source of ContainerInfo.RepoDigests and ContainerInfo.LocallyBuilt.
func (a *ContainerServiceAdapter) WithRepoDigestFetcher(df RepoDigestFetcher) *ContainerServiceAdapter {
	a.digestFetcher = df
	return a
}

// ListContainerInfos returns container info for all running containers.
func (a *ContainerServiceAdapter) ListContainerInfos(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := a.svc.ListContainers(ctx, container.ListContainersOpts{
		StateFilter: string(container.StateRunning),
	})
	if err != nil {
		return nil, err
	}

	// Fetch live labels from the runtime if a fetcher is wired.
	var labelsByExtID map[string]map[string]string
	if a.labelFetcher != nil {
		labelsByExtID, _ = a.labelFetcher.FetchLabels(ctx)
	}
	var digestsByExtID map[string][]string
	if a.digestFetcher != nil {
		digestsByExtID, _ = a.digestFetcher.FetchRepoDigests(ctx)
	}

	infos := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		if c.IsIgnored || c.Archived {
			continue
		}
		info := newContainerInfo(c, labelsByExtID[c.ExternalID])
		digests, known := digestsByExtID[c.ExternalID]
		info.RepoDigests = digests
		info.LocallyBuilt = known && len(digests) == 0
		infos = append(infos, info)
	}
	return infos, nil
}

// GetContainerInfo returns container metadata for a single container by external ID.
func (a *ContainerServiceAdapter) GetContainerInfo(ctx context.Context, externalID string) (ContainerInfo, error) {
	containers, err := a.svc.ListContainers(ctx, container.ListContainersOpts{})
	if err != nil {
		return ContainerInfo{}, fmt.Errorf("get container info: %w", err)
	}

	// Fetch live labels if a fetcher is wired.
	var labelsByExtID map[string]map[string]string
	if a.labelFetcher != nil {
		labelsByExtID, _ = a.labelFetcher.FetchLabels(ctx)
	}

	for _, c := range containers {
		if c.ExternalID == externalID {
			return newContainerInfo(c, labelsByExtID[c.ExternalID]), nil
		}
	}
	return ContainerInfo{}, fmt.Errorf("container not found: %s", externalID)
}

func newContainerInfo(c *container.Container, labels map[string]string) ContainerInfo {
	return ContainerInfo{
		UID:                c.ID,
		ExternalID:         c.ExternalID,
		Name:               c.Name,
		Image:              c.Image,
		Labels:             labels,
		OrchestrationGroup: c.OrchestrationGroup,
		OrchestrationUnit:  c.OrchestrationUnit,
		RuntimeType:        c.RuntimeType,
		ControllerKind:     c.ControllerKind,
		ComposeWorkingDir:  c.ComposeWorkingDir,
	}
}
