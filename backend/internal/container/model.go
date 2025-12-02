package container

import "time"

// ContainerState represents the possible states of a Docker container.
type ContainerState string

const (
	StateRunning    ContainerState = "running"
	StateExited     ContainerState = "exited"
	StateCompleted  ContainerState = "completed" // exited with code 0 (normal/expected termination)
	StateRestarting ContainerState = "restarting"
	StatePaused     ContainerState = "paused"
	StateCreated    ContainerState = "created"
	StateDead       ContainerState = "dead"
)

// HealthStatus represents the health check status of a container.
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthStarting  HealthStatus = "starting"
)

// AlertSeverity represents the alert severity level for a container.
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

// Container represents a discovered container/workload tracked by PulseBoard.
type Container struct {
	ID                  int64          `json:"id"`
	ExternalID          string         `json:"external_id"`
	Name                string         `json:"name"`
	Image               string         `json:"image"`
	State               ContainerState `json:"state"`
	HealthStatus        *HealthStatus  `json:"health_status"`
	HasHealthCheck      bool           `json:"has_health_check"`
	OrchestrationGroup  string         `json:"orchestration_group,omitempty"`
	OrchestrationUnit   string         `json:"orchestration_unit,omitempty"`
	CustomGroup         string         `json:"custom_group,omitempty"`
	IsIgnored           bool           `json:"is_ignored"`
	AlertSeverity       AlertSeverity  `json:"alert_severity"`
	RestartThreshold    int            `json:"restart_threshold"`
	AlertChannels       string         `json:"alert_channels,omitempty"`
	Archived            bool           `json:"archived"`
	FirstSeenAt         time.Time      `json:"first_seen_at"`
	LastStateChangeAt   time.Time      `json:"last_state_change_at"`
	ArchivedAt          *time.Time     `json:"archived_at,omitempty"`
	RuntimeType         string         `json:"runtime_type"`
	ErrorDetail         string         `json:"error_detail,omitempty"`
	ControllerKind      string         `json:"controller_kind,omitempty"`
	Namespace           string         `json:"namespace,omitempty"`
	PodCount            int            `json:"pod_count"`
	ReadyCount          int            `json:"ready_count"`
}

// StateTransition records a container state change event.
type StateTransition struct {
	ID             int64          `json:"id"`
	ContainerID    int64          `json:"container_id"`
	PreviousState  ContainerState `json:"previous_state"`
	NewState       ContainerState `json:"new_state"`
	PreviousHealth *HealthStatus  `json:"previous_health,omitempty"`
	NewHealth      *HealthStatus  `json:"new_health,omitempty"`
	ExitCode       *int           `json:"exit_code,omitempty"`
	LogSnippet     string         `json:"log_snippet,omitempty"`
	Timestamp      time.Time      `json:"timestamp"`
}

// ContainerGroup represents a logical grouping of containers.
type ContainerGroup struct {
	Name       string       `json:"name"`
	Source     string       `json:"source"` // "compose", "label", "default"
	Containers []*Container `json:"containers"`
}

// UptimeResult holds calculated uptime for a container.
type UptimeResult struct {
	Hours24  *float64 `json:"24h"`
	Days7    *float64 `json:"7d,omitempty"`
	Days30   *float64 `json:"30d,omitempty"`
	Days90   *float64 `json:"90d,omitempty"`
}

// GroupName returns the effective group name for this container.
func (c *Container) GroupName() string {
	if c.CustomGroup != "" {
		return c.CustomGroup
	}
	if c.OrchestrationGroup != "" {
		return c.OrchestrationGroup
	}
	return "Ungrouped"
}

// GroupSource returns the source of the group assignment.
func (c *Container) GroupSource() string {
	if c.CustomGroup != "" {
		return "label"
	}
	if c.ControllerKind != "" {
		return "namespace"
	}
	if c.OrchestrationGroup != "" && c.RuntimeType == "docker" {
		return "compose"
	}
	if c.OrchestrationGroup != "" {
		return "orchestration"
	}
	return "default"
}
