// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

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
	AlertFired        = "alert.fired"
	AlertResolved     = "alert.resolved"
	AlertSilenced     = "alert.silenced"
	AlertAcknowledged = "alert.acknowledged"
)

// Notification channel management events.
const (
	ChannelCreated = "channel.created"
	ChannelUpdated = "channel.updated"
	ChannelDeleted = "channel.deleted"
)

// Alert trigger management events.
const (
	TriggerCreated = "trigger.created"
	TriggerUpdated = "trigger.updated"
	TriggerDeleted = "trigger.deleted"
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

// Runtime context change events.
const (
	RuntimeContextChanged      = "runtime.context_changed"
	RuntimeAvailabilityChanged = "runtime.availability_changed"
)

// Storage availability events. Emitted once per transition, so the interface
// can say the database is unreachable instead of showing empty screens.
const (
	StorageAvailabilityChanged = "storage.availability_changed"
)

// Update intelligence events.
const (
	UpdateScanStarted   = "update.scan_started"
	UpdateScanCompleted = "update.scan_completed"
	UpdateDetected      = "update.detected"
	UpdateResolved      = "update.resolved"
	UpdatePinned        = "update.pinned"
	UpdateUnpinned      = "update.unpinned"
)

// Security insight events.
const (
	SecurityInsightsChanged  = "security.insights_changed"
	SecurityInsightsResolved = "security.insights_resolved"
	SecurityPostureChanged   = "security.posture_changed"
)

// Swarm service events.
const (
	SwarmServiceDiscovered = "swarm.service_discovered"
	SwarmServiceUpdated    = "swarm.service_updated"
	SwarmServiceRemoved    = "swarm.service_removed"
	SwarmStatus            = "swarm.status"
)

// Swarm node, task and rolling update events.
const (
	SwarmNodeStatusChanged  = "swarm.node_status_changed"
	SwarmNodeUpdated        = "swarm.node_updated"
	SwarmTaskFailed         = "swarm.task_failed"
	SwarmCrashLoopDetected  = "swarm.crash_loop_detected"
	SwarmCrashLoopRecovered = "swarm.crash_loop_recovered"
	SwarmUpdateProgress     = "swarm.update_progress"
	SwarmUpdateCompleted    = "swarm.update_completed"
)

// Kubernetes monitoring events.
const (
	KubernetesWorkloadChanged = "kubernetes.workload_changed"
	KubernetesPodChanged      = "kubernetes.pod_changed"
	KubernetesNodeChanged     = "kubernetes.node_changed"
)

// Multi-host agent events.
const (
	AgentCreated      = "agent.created"
	AgentUpdated      = "agent.updated"
	AgentRevoked      = "agent.revoked"
	AgentDeleted      = "agent.deleted"
	AgentConnected    = "agent.connected"
	AgentDisconnected = "agent.disconnected"
)

// Public status page events.
const (
	StatusComponentCreated = "status.component_created"
	StatusComponentUpdated = "status.component_updated"
	StatusComponentDeleted = "status.component_deleted"
	StatusComponentChanged = "status.component_changed"
	StatusIncidentCreated  = "status.incident_created"
	StatusIncidentUpdated  = "status.incident_updated"
	StatusIncidentResolved = "status.incident_resolved"
	StatusMaintenanceStart = "status.maintenance_started"
	StatusMaintenanceEnd   = "status.maintenance_ended"
	StatusGlobalChanged    = "status.global_changed"
)
