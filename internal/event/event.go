// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

// Package event defines SSE event type constants shared across all services.
package event

// Endpoint monitoring events.
const (
	EndpointDiscovered    = "endpoint.discovered"
	EndpointStatusChanged = "endpoint.status_changed"
	EndpointRemoved       = "endpoint.removed"
	EndpointAlert         = "endpoint.alert"
	EndpointRecovery      = "endpoint.recovery"
	EndpointConfigError   = "endpoint.config_error"
)

// Heartbeat monitoring events.
const (
	HeartbeatCreated       = "heartbeat.created"
	HeartbeatPingReceived  = "heartbeat.ping_received"
	HeartbeatStatusChanged = "heartbeat.status_changed"
	HeartbeatAlert         = "heartbeat.alert"
	HeartbeatRecovery      = "heartbeat.recovery"
	HeartbeatDeleted       = "heartbeat.deleted"
)

// Certificate monitoring events.
const (
	CertificateCreated        = "certificate.created"
	CertificateCheckCompleted = "certificate.check_completed"
	CertificateStatusChanged  = "certificate.status_changed"
	CertificateAlert          = "certificate.alert"
	CertificateRecovery       = "certificate.recovery"
	CertificateDeleted        = "certificate.deleted"
)

// Container monitoring events.
const (
	ContainerDiscovered     = "container.discovered"
	ContainerStateChanged   = "container.state_changed"
	ContainerHealthChanged  = "container.health_changed"
	ContainerArchived       = "container.archived"
	ContainerRestartAlert   = "container.restart_alert"
	ContainerRestartRecover = "container.restart_recovery"
)

// Resource monitoring events.
const (
	ResourceSnapshot = "resource.snapshot"
	ResourceAlert    = "resource.alert"
	ResourceRecovery = "resource.recovery"
)

// Alert engine events.
const (
	AlertFired    = "alert.fired"
	AlertResolved = "alert.resolved"
	AlertSilenced = "alert.silenced"
)

// Notification channel management events.
const (
	ChannelCreated = "channel.created"
	ChannelUpdated = "channel.updated"
	ChannelDeleted = "channel.deleted"
)

// Silence rule management events.
const (
	SilenceCreated   = "silence.created"
	SilenceCancelled = "silence.cancelled"
)

// Runtime status events.
const (
	RuntimeStatus = "runtime.status"
)

// Update intelligence events.
const (
	UpdateScanStarted   = "update.scan_started"
	UpdateScanCompleted = "update.scan_completed"
	UpdateDetected      = "update.detected"
	UpdatePinned        = "update.pinned"
	UpdateUnpinned      = "update.unpinned"
)

// Public status page events.
const (
	StatusComponentChanged = "status.component_changed"
	StatusIncidentCreated  = "status.incident_created"
	StatusIncidentUpdated  = "status.incident_updated"
	StatusIncidentResolved = "status.incident_resolved"
	StatusMaintenanceStart = "status.maintenance_started"
	StatusMaintenanceEnd   = "status.maintenance_ended"
	StatusGlobalChanged    = "status.global_changed"
)
