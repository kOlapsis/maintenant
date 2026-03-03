// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package container

import (
	"context"
	"time"
)

// ContainerStore defines the persistence interface for container data.
type ContainerStore interface {
	// Container CRUD
	InsertContainer(ctx context.Context, c *Container) (int64, error)
	UpdateContainer(ctx context.Context, c *Container) error
	GetContainerByExternalID(ctx context.Context, externalID string) (*Container, error)
	GetContainerByID(ctx context.Context, id int64) (*Container, error)
	ListContainers(ctx context.Context, opts ListContainersOpts) ([]*Container, error)
	ArchiveContainer(ctx context.Context, externalID string, archivedAt time.Time) error
	DeleteContainerByID(ctx context.Context, id int64) error

	// State transitions
	InsertTransition(ctx context.Context, t *StateTransition) (int64, error)
	ListTransitionsByContainer(ctx context.Context, containerID int64, opts ListTransitionsOpts) ([]*StateTransition, int, error)
	CountRestartsSince(ctx context.Context, containerID int64, since time.Time) (int, error)

	// Uptime
	GetTransitionsInWindow(ctx context.Context, containerID int64, from time.Time, to time.Time) ([]*StateTransition, error)

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
}

// ListTransitionsOpts configures transition listing queries.
type ListTransitionsOpts struct {
	Since  *time.Time
	Until  *time.Time
	Limit  int
	Offset int
}
