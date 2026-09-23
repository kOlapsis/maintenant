// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"time"
)

// ContainerStore defines the persistence interface for container data.
type ContainerStore interface {
	// Container CRUD
	InsertContainer(ctx context.Context, c *Container) (string, error)
	UpdateContainer(ctx context.Context, c *Container) error
	GetContainerByExternalID(ctx context.Context, agentID, externalID string) (*Container, error)
	GetContainerByID(ctx context.Context, id string) (*Container, error)
	ListContainers(ctx context.Context, opts ListContainersOpts) ([]*Container, error)
	ArchiveContainer(ctx context.Context, id string, archivedAt time.Time) error
	DeleteContainerByID(ctx context.Context, id string) error

	// State transitions
	InsertTransition(ctx context.Context, t *StateTransition) (string, error)
	ListTransitionsByContainer(ctx context.Context, containerID string, opts ListTransitionsOpts) ([]*StateTransition, int, error)
	CountRestartsSince(ctx context.Context, containerID string, since time.Time) (int, error)

	// Uptime
	GetTransitionsInWindow(ctx context.Context, containerID string, from time.Time, to time.Time) ([]*StateTransition, error)

	// Retention
	DeleteTransitionsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteArchivedContainersBefore(ctx context.Context, before time.Time) (int64, error)
}

// ListContainersOpts configures container listing queries.
type ListContainersOpts struct {
	IncludeArchived bool
	IncludeIgnored  bool
	GroupFilter     string
	StateFilter     string
	// AgentFilter filters by agent_id. Nil = no filter; "local" = agent_id IS NULL; UUID = specific agent.
	AgentFilter *string
}

// ListTransitionsOpts configures transition listing queries.
type ListTransitionsOpts struct {
	Since  *time.Time
	Until  *time.Time
	Limit  int
	Offset int
}
