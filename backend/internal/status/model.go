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

// Monitor types.
const (
	MonitorContainer   = "container"
	MonitorEndpoint    = "endpoint"
	MonitorHeartbeat   = "heartbeat"
	MonitorCertificate = "certificate"
)

// Global status messages.
const (
	GlobalAllOperational = "All Systems Operational"
	GlobalDegraded       = "Degraded Performance"
	GlobalPartialOutage  = "Partial System Outage"
	GlobalMajorOutage    = "Major System Outage"
	GlobalMaintenance    = "Scheduled Maintenance"
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
type StatusComponent struct {
	ID              int64   `json:"id"`
	MonitorType     string  `json:"monitor_type"`
	MonitorID       int64   `json:"monitor_id"`
	MonitorName     string  `json:"monitor_name,omitempty"`
	DisplayName     string  `json:"display_name"`
	GroupID         *int64  `json:"group_id"`
	GroupName       string  `json:"group_name,omitempty"`
	DisplayOrder    int     `json:"display_order"`
	Visible         bool    `json:"visible"`
	DerivedStatus   string  `json:"derived_status,omitempty"`
	StatusOverride  *string `json:"status_override"`
	EffectiveStatus string  `json:"effective_status,omitempty"`
	AutoIncident    bool    `json:"auto_incident"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
