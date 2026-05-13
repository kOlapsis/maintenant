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

package escalation

import "time"

// Policy represents a stored escalation policy.
type Policy struct {
	ID                    int64     `json:"id"`
	Name                  string    `json:"name"`
	Active                bool      `json:"active"`
	ActiveBeforeDowngrade bool      `json:"active_before_downgrade,omitempty"`
	Filters               Filters   `json:"filters"`
	Levels                []Level   `json:"levels"`
	CreatedAt             time.Time `json:"created_at"`
	CreatedBy             string    `json:"created_by,omitempty"`
	UpdatedAt             time.Time `json:"updated_at"`
	UpdatedBy             string    `json:"updated_by,omitempty"`
}

// Filters defines alert matching criteria for a policy.
type Filters struct {
	Severities []string `json:"severities"`
	Scopes     []Scope  `json:"scopes"`
	Tags       []string `json:"tags"`
}

// Scope identifies a specific monitored entity.
type Scope struct {
	Kind  string `json:"kind"`
	RefID int64  `json:"ref_id"`
}

// Level defines one escalation step.
type Level struct {
	Order        int     `json:"order"`
	DelaySeconds int     `json:"delay_seconds"`
	ChannelIDs   []int64 `json:"channel_ids"`
}

// Run represents an active or completed escalation run for an alert.
type Run struct {
	ID                     int64       `json:"id"`
	PolicyID               *int64      `json:"policy_id"`
	PolicySnapshotJSON     string      `json:"-"`
	AlertID                int64       `json:"alert_id"`
	Status                 string      `json:"status"`
	LastExecutedLevelIndex int         `json:"last_executed_level_index"`
	StartedAt              time.Time   `json:"started_at"`
	EndedAt                *time.Time  `json:"ended_at"`
	NextActionAt           *time.Time  `json:"next_action_at"`
	Policy                 *RunPolicy  `json:"policy,omitempty"`
	Deliveries             []*Delivery `json:"deliveries,omitempty"`
}

// RunPolicy is the minimal policy info embedded in run responses.
type RunPolicy struct {
	ID   *int64 `json:"id"`
	Name string `json:"name"`
}

// DeliveriesSummary aggregates delivery counts for a run.
type DeliveriesSummary struct {
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Pending int `json:"pending"`
}

// Delivery represents a single notification attempt within a run.
type Delivery struct {
	ID               int64      `json:"id"`
	RunID            int64      `json:"run_id"`
	LevelIndex       int        `json:"level_index"`
	ChannelID        *int64     `json:"channel_id"`
	ChannelName      string     `json:"channel_name,omitempty"`
	Status           string     `json:"status"`
	Error            string     `json:"error,omitempty"`
	AttemptStartedAt time.Time  `json:"attempt_started_at"`
	SentAt           *time.Time `json:"sent_at"`
}

// Limits describes plan tier constraints for the current user.
type Limits struct {
	MaxActive     int `json:"max_active"`
	MaxLevels     int `json:"max_levels"`
	CurrentActive int `json:"current_active"`
}

// OverlapWarning describes a conflict between two policies.
type OverlapWarning struct {
	PolicyID           int64   `json:"policy_id"`
	PolicyName         string  `json:"policy_name"`
	SharedChannels     []int64 `json:"shared_channels"`
	FilterIntersection string  `json:"filter_intersection"`
}
