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

// ContainerServiceAdapter adapts container.Service to the ContainerLister interface.
type ContainerServiceAdapter struct {
	svc          *container.Service
	labelFetcher LabelFetcher // optional — nil when runtime doesn't support label fetching
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

	infos := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		if c.IsIgnored || c.Archived {
			continue
		}
		infos = append(infos, ContainerInfo{
			ExternalID:         c.ExternalID,
			Name:               c.Name,
			Image:              c.Image,
			Labels:             labelsByExtID[c.ExternalID],
			OrchestrationGroup: c.OrchestrationGroup,
			OrchestrationUnit:  c.OrchestrationUnit,
			RuntimeType:        c.RuntimeType,
			ControllerKind:     c.ControllerKind,
			ComposeWorkingDir:  c.ComposeWorkingDir,
		})
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
			return ContainerInfo{
				ExternalID:         c.ExternalID,
				Name:               c.Name,
				Image:              c.Image,
				Labels:             labelsByExtID[c.ExternalID],
				OrchestrationGroup: c.OrchestrationGroup,
				OrchestrationUnit:  c.OrchestrationUnit,
				RuntimeType:        c.RuntimeType,
				ControllerKind:     c.ControllerKind,
				ComposeWorkingDir:  c.ComposeWorkingDir,
			}, nil
		}
	}
	return ContainerInfo{}, fmt.Errorf("container not found: %s", externalID)
}
