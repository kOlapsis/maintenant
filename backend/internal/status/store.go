package status

import "context"

// ComponentStore defines the persistence interface for component groups and status components.
type ComponentStore interface {
	// Groups
	ListGroups(ctx context.Context) ([]ComponentGroup, error)
	GetGroup(ctx context.Context, id int64) (*ComponentGroup, error)
	CreateGroup(ctx context.Context, g *ComponentGroup) (int64, error)
	UpdateGroup(ctx context.Context, g *ComponentGroup) error
	DeleteGroup(ctx context.Context, id int64) error

	// Components
	ListComponents(ctx context.Context) ([]StatusComponent, error)
	ListVisibleComponents(ctx context.Context) ([]StatusComponent, error)
	GetComponent(ctx context.Context, id int64) (*StatusComponent, error)
	GetComponentByMonitor(ctx context.Context, monitorType string, monitorID int64) (*StatusComponent, error)
	CreateComponent(ctx context.Context, c *StatusComponent) (int64, error)
	UpdateComponent(ctx context.Context, c *StatusComponent) error
	DeleteComponent(ctx context.Context, id int64) error
}
