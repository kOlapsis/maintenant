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

package alert

import (
	"context"
	"time"
)

// Alert sources.
const (
	SourceContainer   = "container"
	SourceEndpoint    = "endpoint"
	SourceHeartbeat   = "heartbeat"
	SourceCertificate = "certificate"
	SourceResource    = "resource"
	SourceSecurity    = "security"
)

// Security alert types.
const (
	AlertTypeDangerousConfig  = "dangerous_configuration"
	AlertTypePostureThreshold = "posture_threshold"
)

// Alert statuses.
const (
	StatusActive   = "active"
	StatusResolved = "resolved"
	StatusSilenced = "silenced"
)

// Severity levels.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// Delivery statuses.
const (
	DeliveryPending   = "pending"
	DeliveryDelivered = "delivered"
	DeliveryFailed    = "failed"
)

// Event represents a unified alert event sent via Go channel from any monitoring service.
type Event struct {
	Source     string         // "container", "endpoint", "heartbeat", "certificate", "resource"
	AlertType  string         // source-specific type
	Severity   string         // "critical", "warning", "info"
	IsRecover  bool           // true if this is a recovery event
	Message    string         // human-readable description
	EntityType string         // "container", "endpoint", "heartbeat", "certificate"
	EntityID   int64          // ID in source table
	EntityName string         // display name
	Details    map[string]any // source-specific metadata
	Timestamp  time.Time      // when condition was detected
}

// Alert represents a persisted alert record.
type Alert struct {
	ID             int64      `json:"id"`
	Source         string     `json:"source"`
	AlertType      string     `json:"alert_type"`
	Severity       string     `json:"severity"`
	Status         string     `json:"status"`
	Message        string     `json:"message"`
	EntityType     string     `json:"entity_type"`
	EntityID       int64      `json:"entity_id"`
	EntityName     string     `json:"entity_name"`
	Details        string     `json:"details"`
	ResolvedByID   *int64     `json:"resolved_by_id"`
	FiredAt        time.Time  `json:"fired_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
	EscalatedAt    *time.Time `json:"escalated_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// NotificationChannel represents a configured delivery target.
// Channels are silent by default — they only receive alerts when referenced
// by an active AlertTrigger or by an EscalationLevel.
type NotificationChannel struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	URL       string    `json:"url"`
	Headers   string    `json:"headers,omitempty"`
	Enabled   bool      `json:"enabled"`
	Health    string    `json:"health,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AlertTrigger is a routing rule that maps an alert filter to one or more channels.
// Filters are stored as CSV strings; an empty filter matches anything.
// Filters are combined in AND between fields, OR within a field.
// FilterScopes and FilterTags are Pro-only (gated at the handler level).
type AlertTrigger struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	FilterSeverities string    `json:"filter_severities"`
	FilterSources    string    `json:"filter_sources"`
	FilterScopes     string    `json:"filter_scopes"`
	FilterTags       string    `json:"filter_tags"`
	Enabled          bool      `json:"enabled"`
	ChannelIDs       []int64   `json:"channel_ids"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// NotificationDelivery represents a delivery attempt record.
type NotificationDelivery struct {
	ID        int64     `json:"id"`
	AlertID   int64     `json:"alert_id"`
	ChannelID int64     `json:"channel_id"`
	Status    string    `json:"status"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SilenceRule represents a time-bounded notification suppression.
type SilenceRule struct {
	ID              int64      `json:"id"`
	EntityType      string     `json:"entity_type,omitempty"`
	EntityID        *int64     `json:"entity_id,omitempty"`
	Source          string     `json:"source,omitempty"`
	Reason          string     `json:"reason,omitempty"`
	StartsAt        time.Time  `json:"starts_at"`
	DurationSeconds int        `json:"duration_seconds"`
	ExpiresAt       time.Time  `json:"expires_at"`
	IsActive        bool       `json:"is_active"`
	CancelledAt     *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// ListAlertsOpts contains filter parameters for listing alerts.
type ListAlertsOpts struct {
	Source   string
	Severity string
	Status   string
	Before   *time.Time
	Limit    int
}

// Escalator manages Pro escalation policy execution.
type Escalator interface {
	EvaluateCycle(ctx context.Context) error
	OnAlertAcknowledged(ctx context.Context, alertID int64, ack Acknowledgment) error
	OnAlertResolved(ctx context.Context, alertID int64, resolvedAt time.Time) error
	OnEditionDowngraded(ctx context.Context) error
}

// Acknowledgment carries info about an alert ack event.
type Acknowledgment struct {
	By string
	At time.Time
}

// EntityRouter provides per-entity alert routing.
type EntityRouter interface {
	Route(ctx context.Context, entityType string, entityID string, severity string) ([]string, error)
}

// MaintenanceSuppressor checks if an alert should be suppressed during a maintenance window.
type MaintenanceSuppressor interface {
	IsSuppressed(ctx context.Context, source string, entityType string, entityID string) (bool, error)
}

// AlertStore defines the persistence interface for alerts.
type AlertStore interface {
	InsertAlert(ctx context.Context, a *Alert) (int64, error)
	GetAlert(ctx context.Context, id int64) (*Alert, error)
	ListAlerts(ctx context.Context, opts ListAlertsOpts) ([]*Alert, error)
	UpdateAlertStatus(ctx context.Context, id int64, status string, resolvedAt *time.Time, resolvedByID *int64) error
	UpdateAlertSeverity(ctx context.Context, id int64, severity, message string) error
	GetActiveAlert(ctx context.Context, source, alertType, entityType string, entityID int64) (*Alert, error)
	ListActiveAlerts(ctx context.Context) ([]*Alert, error)
	DeleteAlertsOlderThan(ctx context.Context, before time.Time) (int64, error)
	AcknowledgeAlert(ctx context.Context, id int64, by string, at time.Time) error
	SetEscalatedAt(ctx context.Context, id int64, at time.Time) error
	ListUnacknowledgedActiveAlerts(ctx context.Context) ([]*Alert, error)
}

// ChannelStore defines the persistence interface for notification channels.
type ChannelStore interface {
	InsertChannel(ctx context.Context, ch *NotificationChannel) (int64, error)
	GetChannel(ctx context.Context, id int64) (*NotificationChannel, error)
	ListChannels(ctx context.Context) ([]*NotificationChannel, error)
	UpdateChannel(ctx context.Context, ch *NotificationChannel) error
	DeleteChannel(ctx context.Context, id int64) error
	GetChannelHealth(ctx context.Context, channelID int64) (string, error)

	InsertDelivery(ctx context.Context, d *NotificationDelivery) (int64, error)
	UpdateDelivery(ctx context.Context, d *NotificationDelivery) error
	ListDeliveriesByAlert(ctx context.Context, alertID int64) ([]*NotificationDelivery, error)
}

// TriggerStore defines the persistence interface for AlertTrigger objects
// and their M:N relationship with channels.
type TriggerStore interface {
	InsertTrigger(ctx context.Context, t *AlertTrigger) (int64, error)
	GetTrigger(ctx context.Context, id int64) (*AlertTrigger, error)
	ListTriggers(ctx context.Context) ([]*AlertTrigger, error)
	ListEnabledTriggers(ctx context.Context) ([]*AlertTrigger, error)
	UpdateTrigger(ctx context.Context, t *AlertTrigger) error
	DeleteTrigger(ctx context.Context, id int64) error
	SetChannels(ctx context.Context, triggerID int64, channelIDs []int64) error
	ListChannelsForTrigger(ctx context.Context, triggerID int64) ([]int64, error)
	ListTriggersForChannel(ctx context.Context, channelID int64) ([]*AlertTrigger, error)
}

// SilenceStore defines the persistence interface for silence rules.
type SilenceStore interface {
	InsertSilenceRule(ctx context.Context, rule *SilenceRule) (int64, error)
	ListSilenceRules(ctx context.Context, activeOnly bool) ([]*SilenceRule, error)
	CancelSilenceRule(ctx context.Context, id int64) error
	GetActiveSilenceRules(ctx context.Context) ([]*SilenceRule, error)
}
