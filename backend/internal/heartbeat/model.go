package heartbeat

import "time"

// HeartbeatStatus represents the current state of a heartbeat monitor.
type HeartbeatStatus string

const (
	StatusNew     HeartbeatStatus = "new"
	StatusUp      HeartbeatStatus = "up"
	StatusDown    HeartbeatStatus = "down"
	StatusStarted HeartbeatStatus = "started"
	StatusPaused  HeartbeatStatus = "paused"
)

// AlertState represents the alert state of a heartbeat monitor.
type AlertState string

const (
	AlertNormal   AlertState = "normal"
	AlertAlerting AlertState = "alerting"
)

// PingType represents the type of a ping event.
type PingType string

const (
	PingSuccess  PingType = "success"
	PingStart    PingType = "start"
	PingExitCode PingType = "exit_code"
)

// ExecutionOutcome represents the result of a job execution.
type ExecutionOutcome string

const (
	OutcomeSuccess    ExecutionOutcome = "success"
	OutcomeFailure    ExecutionOutcome = "failure"
	OutcomeTimeout    ExecutionOutcome = "timeout"
	OutcomeInProgress ExecutionOutcome = "in_progress"
)

// Validation constants.
const (
	MinIntervalSeconds = 60      // 1 minute
	MaxIntervalSeconds = 604800  // 7 days
	MaxPayloadBytes    = 10240   // 10 KB
	MaxNameLength      = 255
)

// Heartbeat represents a passive heartbeat monitor.
type Heartbeat struct {
	ID                   int64           `json:"id"`
	UUID                 string          `json:"uuid"`
	Name                 string          `json:"name"`
	Status               HeartbeatStatus `json:"status"`
	AlertState           AlertState      `json:"alert_state"`
	IntervalSeconds      int             `json:"interval_seconds"`
	GraceSeconds         int             `json:"grace_seconds"`
	LastPingAt           *time.Time      `json:"last_ping_at,omitempty"`
	NextDeadlineAt       *time.Time      `json:"next_deadline_at,omitempty"`
	CurrentRunStartedAt  *time.Time      `json:"current_run_started_at,omitempty"`
	LastExitCode         *int            `json:"last_exit_code,omitempty"`
	LastDurationMs       *int64          `json:"last_duration_ms,omitempty"`
	ConsecutiveFailures  int             `json:"consecutive_failures"`
	ConsecutiveSuccesses int             `json:"consecutive_successes"`
	Active               bool            `json:"active"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// PingURL returns the relative ping URL for this heartbeat.
func (h *Heartbeat) PingURL() string {
	return "/ping/" + h.UUID
}

// HeartbeatPing represents a raw ping event.
type HeartbeatPing struct {
	ID          int64    `json:"id"`
	HeartbeatID int64    `json:"heartbeat_id"`
	PingType    PingType `json:"ping_type"`
	ExitCode    *int     `json:"exit_code,omitempty"`
	SourceIP    string   `json:"source_ip"`
	HTTPMethod  string   `json:"http_method"`
	Payload     *string  `json:"payload,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// HeartbeatExecution represents a logical job run.
type HeartbeatExecution struct {
	ID          int64            `json:"id"`
	HeartbeatID int64            `json:"heartbeat_id"`
	StartedAt   *time.Time       `json:"started_at,omitempty"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
	DurationMs  *int64           `json:"duration_ms,omitempty"`
	ExitCode    *int             `json:"exit_code,omitempty"`
	Outcome     ExecutionOutcome `json:"outcome"`
	Payload     *string          `json:"payload,omitempty"`
}

// ListHeartbeatsOpts configures heartbeat listing queries.
type ListHeartbeatsOpts struct {
	Status          string
	IncludeInactive bool
}

// ListPingsOpts configures ping listing queries.
type ListPingsOpts struct {
	Limit  int
	Offset int
}

// ListExecutionsOpts configures execution listing queries.
type ListExecutionsOpts struct {
	Limit  int
	Offset int
}

// CreateHeartbeatInput contains the fields needed to create a heartbeat.
type CreateHeartbeatInput struct {
	Name            string `json:"name"`
	IntervalSeconds int    `json:"interval_seconds"`
	GraceSeconds    int    `json:"grace_seconds"`
}

// UpdateHeartbeatInput contains the fields that can be updated.
type UpdateHeartbeatInput struct {
	Name            *string `json:"name,omitempty"`
	IntervalSeconds *int    `json:"interval_seconds,omitempty"`
	GraceSeconds    *int    `json:"grace_seconds,omitempty"`
}
