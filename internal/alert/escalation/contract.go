// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package escalation

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by Service methods.
var (
	ErrValidationFailed = errors.New("validation_failed")
	ErrPolicyNotFound   = errors.New("policy_not_found")
	ErrRunNotFound      = errors.New("run_not_found")

	// ErrDeliveryDuplicate is returned by Store.InsertDelivery when a row
	// for (run_id, level_index, channel_id) already exists. This signals the
	// runner that another worker has already reserved this delivery slot
	// (idempotence guarantee R4 — reserve-then-deliver).
	ErrDeliveryDuplicate = errors.New("delivery_duplicate")
)

// PolicyRequest is the input for creating or updating a policy.
type PolicyRequest struct {
	Name    string     `json:"name"`
	Active  bool       `json:"active"`
	Filters Filters    `json:"filters"`
	Levels  []LevelReq `json:"levels"`
}

// LevelReq is one escalation step in a request (order is assigned by service).
type LevelReq struct {
	DelaySeconds int      `json:"delay_seconds"`
	ChannelIDs   []string `json:"channel_ids"`
}

// Store defines the persistence interface for escalation data.
type Store interface {
	InsertPolicy(ctx context.Context, p *Policy) (string, error)
	UpdatePolicy(ctx context.Context, p *Policy) error
	SelectPolicy(ctx context.Context, id string) (*Policy, error)
	SelectPolicies(ctx context.Context, activeOnly bool) ([]*Policy, error)
	DeletePolicy(ctx context.Context, id string) error
	CountActivePolicies(ctx context.Context) (int, error)
	SelectRun(ctx context.Context, id string) (*Run, error)
	SelectRunsByAlert(ctx context.Context, alertID string) ([]*Run, error)
	SelectRunsByPolicy(ctx context.Context, policyID string, limit int, cursor string) ([]*Run, error)
	SelectRunDeliveries(ctx context.Context, runID string) ([]*Delivery, error)
	BulkDeactivateAllPolicies(ctx context.Context) error
	BulkRestorePoliciesFromDowngrade(ctx context.Context) error
	BulkStopActiveRuns(ctx context.Context, stopStatus string, endedAt time.Time) error
	StopPolicyRuns(ctx context.Context, policyID string, stopStatus string, endedAt time.Time) error
	PurgeRunsAndDeliveriesOlderThan(ctx context.Context, before time.Time) error

	// Run lifecycle (used by the concrete Pro Runner).
	InsertRun(ctx context.Context, r *Run) (string, error)
	UpdateRunProgress(ctx context.Context, runID string, lastExecutedLevelIndex int, nextActionAt *time.Time, status string) error
	TerminateRun(ctx context.Context, runID string, status string, endedAt time.Time) error
	SelectActiveRunsByAlert(ctx context.Context, alertID string) ([]*Run, error)
	// SelectDueRuns returns runs in status 'active' OR 'paused_by_maintenance'
	// whose next_action_at <= now. The runner re-evaluates the suppressor on
	// every tick, so paused runs need to surface alongside active ones.
	SelectDueRuns(ctx context.Context, now time.Time) ([]*Run, error)
	PauseRunForMaintenance(ctx context.Context, runID string, recheckAt time.Time) error
	ResumeRunFromMaintenance(ctx context.Context, runID string, nextActionAt time.Time) error

	// Delivery lifecycle (used by the concrete Pro Runner).
	// InsertDelivery returns ErrDeliveryDuplicate when the UNIQUE
	// (run_id, level_index, channel_id) constraint is violated.
	InsertDelivery(ctx context.Context, d *Delivery) (string, error)
	UpdateDelivery(ctx context.Context, d *Delivery) error
	SelectOrphanPendingDeliveries(ctx context.Context, before time.Time) ([]*Delivery, error)
}

// Run statuses (mirror the SQL CHECK constraint on escalation_runs).
const (
	RunStatusActive                  = "active"
	RunStatusPausedByMaintenance     = "paused_by_maintenance"
	RunStatusStoppedByAck            = "stopped_by_ack"
	RunStatusStoppedByResolution     = "stopped_by_resolution"
	RunStatusStoppedByPolicyDelete   = "stopped_by_policy_deletion"
	RunStatusStoppedByPolicyDisabled = "stopped_by_policy_disabled"
	RunStatusStoppedByDowngrade      = "stopped_by_edition_downgrade"
	RunStatusExhausted               = "exhausted"
)

// Delivery statuses (mirror the SQL CHECK constraint on escalation_deliveries).
const (
	DeliveryStatusPending   = "pending"
	DeliveryStatusSent      = "sent"
	DeliveryStatusFailed    = "failed"
	DeliveryStatusAbandoned = "abandoned"
)

// MaxLevels is the most levels a policy may hold.
const MaxLevels = 5

// Service manages escalation policies and reads their runs.
type Service interface {
	CreatePolicy(ctx context.Context, req PolicyRequest) (*Policy, error)
	GetPolicy(ctx context.Context, id string) (*Policy, error)
	ListPolicies(ctx context.Context, activeOnly bool) ([]*Policy, error)
	UpdatePolicy(ctx context.Context, id string, req PolicyRequest) (*Policy, error)
	DeletePolicy(ctx context.Context, id string) error
	SetPolicyActive(ctx context.Context, id string, active bool) (*Policy, error)
	GetPlanLimits(ctx context.Context) (Limits, error)
	ListRunsForAlert(ctx context.Context, alertID string) ([]*Run, error)
	GetRun(ctx context.Context, id string) (*Run, error)
	ListPolicyRuns(ctx context.Context, policyID string, limit int, cursor string) ([]*Run, error)
	DetectOverlap(candidate *Policy, existing []*Policy) []OverlapWarning
	OnEditionDowngraded(ctx context.Context) error
	OnEditionUpgraded(ctx context.Context) error
	RunRetentionLoop(ctx context.Context)
}
