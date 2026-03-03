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

package runtime

import (
	"context"
	"io"

	"github.com/kolapsis/maintenant/internal/container"
)

// Runtime abstracts the container orchestration platform.
// Implementations: docker.Runtime, kubernetes.Runtime.
//
//goland:noinspection GoCommentStart
type Runtime interface {
	// Lifecycle
	Connect(ctx context.Context) error
	IsConnected() bool
	SetDisconnected()
	Close() error

	// Identity
	Name() string // "docker" or "kubernetes"

	// Discovery
	DiscoverAll(ctx context.Context) ([]*container.Container, error)

	// Events
	StreamEvents(ctx context.Context) <-chan RuntimeEvent

	// Stats
	StatsSnapshot(ctx context.Context, externalID string) (*RawStats, error)

	// Logs
	FetchLogs(ctx context.Context, externalID string, lines int, timestamps bool) ([]string, error)
	StreamLogs(ctx context.Context, externalID string, lines int, timestamps bool) (io.ReadCloser, error)

	// Health
	GetHealthInfo(ctx context.Context, externalID string) (*HealthInfo, error)
}
