// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package update

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/container"
)

// ContainerServiceAdapter adapts container.Service to the ContainerLister interface.
type ContainerServiceAdapter struct {
	svc *container.Service
}

// NewContainerServiceAdapter creates a new adapter.
func NewContainerServiceAdapter(svc *container.Service) *ContainerServiceAdapter {
	return &ContainerServiceAdapter{svc: svc}
}

// ListContainerInfos returns container info for all running containers.
func (a *ContainerServiceAdapter) ListContainerInfos(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := a.svc.ListContainers(ctx, container.ListContainersOpts{
		StateFilter: string(container.StateRunning),
	})
	if err != nil {
		return nil, err
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
			OrchestrationGroup: c.OrchestrationGroup,
			OrchestrationUnit:  c.OrchestrationUnit,
			RuntimeType:        c.RuntimeType,
			ControllerKind:     c.ControllerKind,
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
	for _, c := range containers {
		if c.ExternalID == externalID {
			return ContainerInfo{
				ExternalID:         c.ExternalID,
				Name:               c.Name,
				Image:              c.Image,
				OrchestrationGroup: c.OrchestrationGroup,
				OrchestrationUnit:  c.OrchestrationUnit,
				RuntimeType:        c.RuntimeType,
				ControllerKind:     c.ControllerKind,
			}, nil
		}
	}
	return ContainerInfo{}, fmt.Errorf("container not found: %s", externalID)
}
