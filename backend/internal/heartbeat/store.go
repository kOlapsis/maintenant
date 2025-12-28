package heartbeat

import (
	"context"
	"time"
)

// HeartbeatStore defines the persistence interface for heartbeat monitoring data.
type HeartbeatStore interface {
	// Heartbeat CRUD
	CreateHeartbeat(ctx context.Context, h *Heartbeat) (int64, error)
	GetHeartbeatByID(ctx context.Context, id int64) (*Heartbeat, error)
	GetHeartbeatByUUID(ctx context.Context, uuid string) (*Heartbeat, error)
	ListHeartbeats(ctx context.Context, opts ListHeartbeatsOpts) ([]*Heartbeat, error)
	UpdateHeartbeat(ctx context.Context, id int64, input UpdateHeartbeatInput) error
	DeleteHeartbeat(ctx context.Context, id int64) error

	// State updates
	UpdateHeartbeatState(ctx context.Context, id int64, status HeartbeatStatus, alertState AlertState,
		lastPingAt *time.Time, nextDeadlineAt *time.Time, currentRunStartedAt *time.Time,
		lastExitCode *int, lastDurationMs *int64,
		consecutiveFailures, consecutiveSuccesses int) error
	PauseHeartbeat(ctx context.Context, id int64) error
	ResumeHeartbeat(ctx context.Context, id int64, nextDeadlineAt time.Time) error

	// Deadline scanning
	ListOverdueHeartbeats(ctx context.Context, now time.Time) ([]*Heartbeat, error)

	// License gating
	CountActiveHeartbeats(ctx context.Context) (int, error)

	// Pings
	InsertPing(ctx context.Context, p *HeartbeatPing) (int64, error)
	ListPings(ctx context.Context, heartbeatID int64, opts ListPingsOpts) ([]*HeartbeatPing, int, error)

	// Executions
	InsertExecution(ctx context.Context, e *HeartbeatExecution) (int64, error)
	UpdateExecution(ctx context.Context, id int64, completedAt *time.Time, durationMs *int64, exitCode *int, outcome ExecutionOutcome, payload *string) error
	GetCurrentExecution(ctx context.Context, heartbeatID int64) (*HeartbeatExecution, error)
	ListExecutions(ctx context.Context, heartbeatID int64, opts ListExecutionsOpts) ([]*HeartbeatExecution, int, error)

	// Retention
	DeletePingsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExecutionsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error)
}
