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

package status

import "time"

// Component status values.
const (
	StatusOperational   = "operational"
	StatusDegraded      = "degraded"
	StatusPartialOutage = "partial_outage"
	StatusMajorOutage   = "major_outage"
	StatusUnderMaint    = "under_maintenance"
)

// Incident severity levels.
const (
	SeverityMinor    = "minor"
	SeverityMajor    = "major"
	SeverityCritical = "critical"
)

// Incident status values.
const (
	IncidentInvestigating = "investigating"
	IncidentResolved      = "resolved"
)

// Global status messages.
const (
	GlobalAllOperational = "All Systems Operational"
	GlobalDegraded       = "Degraded Performance"
	GlobalPartialOutage  = "Partial System Outage"
	GlobalMajorOutage    = "Major System Outage"
	GlobalMaintenance    = "Scheduled Maintenance"
)

// TLS policy values for SMTP.
const (
	TLSMandatory = "mandatory"
	TLSNone      = "none"
)

// ComponentGroup groups status components into named categories.
type ComponentGroup struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	DisplayOrder   int       `json:"display_order"`
	ComponentCount int       `json:"component_count,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// StatusComponent is a public-facing representation of a monitored service.
type Component struct {
	ID              int64     `json:"id"`
	MonitorType     string    `json:"monitor_type"`
	MonitorID       int64     `json:"monitor_id"`
	MonitorName     string    `json:"monitor_name,omitempty"`
	DisplayName     string    `json:"display_name"`
	GroupID         *int64    `json:"group_id"`
	GroupName       string    `json:"group_name,omitempty"`
	DisplayOrder    int       `json:"display_order"`
	Visible         bool      `json:"visible"`
	DerivedStatus   string    `json:"derived_status,omitempty"`
	StatusOverride  *string   `json:"status_override"`
	EffectiveStatus string    `json:"effective_status,omitempty"`
	AutoIncident    bool      `json:"auto_incident"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Incident represents a public-facing incident.
type Incident struct {
	ID                  int64             `json:"id"`
	Title               string            `json:"title"`
	Severity            string            `json:"severity"`
	Status              string            `json:"status"`
	IsMaintenance       bool              `json:"is_maintenance"`
	MaintenanceWindowID *int64            `json:"maintenance_window_id,omitempty"`
	Components          []IncidentCompRef `json:"components,omitempty"`
	Updates             []IncidentUpdate  `json:"updates,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	ResolvedAt          *time.Time        `json:"resolved_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

// IncidentCompRef is a lightweight component reference for incident responses.
type IncidentCompRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// IncidentUpdate is a timestamped entry in an incident timeline.
type IncidentUpdate struct {
	ID         int64     `json:"id"`
	IncidentID int64     `json:"incident_id"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	IsAuto     bool      `json:"is_auto"`
	AlertID    *int64    `json:"alert_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// StatusSubscriber is an email subscriber for incident notifications.
type StatusSubscriber struct {
	ID             int64      `json:"id"`
	Email          string     `json:"email"`
	Confirmed      bool       `json:"confirmed"`
	ConfirmToken   *string    `json:"-"`
	ConfirmExpires *time.Time `json:"-"`
	UnsubToken     string     `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
}

// MaintenanceWindow represents a scheduled maintenance period.
type MaintenanceWindow struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      time.Time         `json:"ends_at"`
	Active      bool              `json:"active"`
	IncidentID  *int64            `json:"incident_id"`
	Components  []IncidentCompRef `json:"components,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// SmtpConfig holds SMTP server configuration for sending emails.
type SmtpConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	TLSPolicy   string `json:"tls_policy"`
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
	Configured  bool   `json:"configured"`
}

// ListIncidentsOpts contains filter parameters for listing incidents.
type ListIncidentsOpts struct {
	Status   string
	Severity string
	Limit    int
	Offset   int
}

// SubscriberStats holds aggregated subscriber information.
type SubscriberStats struct {
	Total     int `json:"total"`
	Confirmed int `json:"confirmed"`
}
