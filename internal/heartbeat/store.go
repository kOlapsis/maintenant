// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package heartbeat

import (
	"context"
	"time"
)

// HeartbeatStore defines the persistence interface for heartbeat monitoring data.
// A heartbeat's id is its public ping token; methods take that string.
type HeartbeatStore interface {
	// Heartbeat CRUD
	CreateHeartbeat(ctx context.Context, h *Heartbeat) (string, error)
	GetHeartbeatByID(ctx context.Context, id string) (*Heartbeat, error)
	// GetHeartbeatByUUID looks up by the ping token (which is the id).
	GetHeartbeatByUUID(ctx context.Context, token string) (*Heartbeat, error)
	ListHeartbeats(ctx context.Context, opts ListHeartbeatsOpts) ([]*Heartbeat, error)
	UpdateHeartbeat(ctx context.Context, id string, input UpdateHeartbeatInput) error
	DeleteHeartbeat(ctx context.Context, id string) error

	// State updates
	UpdateHeartbeatState(ctx context.Context, id string, status HeartbeatStatus, alertState AlertState,
		lastPingAt *time.Time, nextDeadlineAt *time.Time, currentRunStartedAt *time.Time,
		lastExitCode *int, lastDurationMs *int64,
		consecutiveFailures, consecutiveSuccesses int) error
	PauseHeartbeat(ctx context.Context, id string) error
	ResumeHeartbeat(ctx context.Context, id string, nextDeadlineAt time.Time) error

	// Deadline scanning
	ListOverdueHeartbeats(ctx context.Context, now time.Time) ([]*Heartbeat, error)

	// License gating
	CountActiveHeartbeats(ctx context.Context) (int, error)

	// Pings
	InsertPing(ctx context.Context, p *HeartbeatPing) (string, error)
	ListPings(ctx context.Context, heartbeatID string, opts ListPingsOpts) ([]*HeartbeatPing, int, error)

	// Executions
	InsertExecution(ctx context.Context, e *HeartbeatExecution) (string, error)
	UpdateExecution(ctx context.Context, id string, completedAt *time.Time, durationMs *int64, exitCode *int, outcome ExecutionOutcome, payload *string) error
	GetCurrentExecution(ctx context.Context, heartbeatID string) (*HeartbeatExecution, error)
	ListExecutions(ctx context.Context, heartbeatID string, opts ListExecutionsOpts) ([]*HeartbeatExecution, int, error)

	// Retention
	DeletePingsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExecutionsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
}
