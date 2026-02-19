package update

import (
	"context"

	"github.com/kolapsis/pulseboard/internal/container"
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
