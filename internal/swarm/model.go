// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import "time"

// SwarmCluster represents the Swarm cluster state (in-memory only, reconstructed from docker info).
type SwarmCluster struct {
	ID           string    `json:"cluster_id"`
	CreatedAt    time.Time `json:"created_at"`
	ManagerCount int       `json:"manager_count"`
	WorkerCount  int       `json:"worker_count"`
	IsManager    bool      `json:"is_manager"`
}

// SwarmService represents a Swarm service with aggregated state (volatile/in-memory).
type SwarmService struct {
	ServiceID       string              `json:"service_id"`
	AgentID         string              `json:"agent_id,omitempty"`
	Name            string              `json:"name"`
	Image           string              `json:"image"`
	Mode            string              `json:"mode"` // "replicated" or "global"
	DesiredReplicas int                 `json:"desired_replicas"`
	RunningReplicas int                 `json:"running_replicas"`
	Labels          map[string]string   `json:"labels,omitempty"`
	StackName       string              `json:"stack_name,omitempty"`
	Networks        []NetworkAttachment `json:"networks,omitempty"`
	Ports           []PortConfig        `json:"ports,omitempty"`
	UpdateStatus    *UpdateStatus       `json:"update_status,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
}

// SwarmTask represents a single instance (replica) of a service (volatile/in-memory).
type SwarmTask struct {
	TaskID       string    `json:"task_id"`
	AgentID      string    `json:"agent_id,omitempty"`
	ServiceID    string    `json:"service_id"`
	NodeID       string    `json:"node_id"`
	Slot         int       `json:"slot"`
	State        string    `json:"state"`
	DesiredState string    `json:"desired_state"`
	ContainerID  string    `json:"container_id"`
	Error        string    `json:"error,omitempty"`
	ExitCode     *int      `json:"exit_code,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	NodeHostname string    `json:"node_hostname,omitempty"`
}

// NetworkAttachment represents a network attached to a Swarm service.
type NetworkAttachment struct {
	NetworkID   string `json:"network_id"`
	NetworkName string `json:"network_name"`
	Scope       string `json:"scope"` // "swarm", "local"
}

// PortConfig represents a published port on a Swarm service.
type PortConfig struct {
	Protocol      string `json:"protocol"`       // "tcp", "udp", "sctp"
	TargetPort    uint32 `json:"target_port"`    // container port
	PublishedPort uint32 `json:"published_port"` // host/ingress port
	PublishMode   string `json:"publish_mode"`   // "ingress" or "host"
}

// UpdateStatus represents the rolling update status of a Swarm service.
type UpdateStatus struct {
	State       string     `json:"state"` // "updating", "paused", "completed", "rollback_started", "rollback_completed", "rollback_paused"
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Message     string     `json:"message,omitempty"`
}

// TopologySnapshot is a full point-in-time view of an agent's swarm: every
// service, task and node it currently sees. The server reconciles it against
// the rows already held for that agent, hard-deleting anything absent from the
// snapshot. Produced by both the server's local runtime (under the LocalAgent
// id) and remote agents (under their own id).
type TopologySnapshot struct {
	Services []SwarmService
	Tasks    []SwarmTask
	Nodes    []SwarmNode
}

// SwarmNode represents a machine in the Swarm cluster.
type SwarmNode struct {
	ID                 string    `json:"id"`
	AgentID            string    `json:"agent_id"`
	NodeID             string    `json:"node_id"`
	Hostname           string    `json:"hostname"`
	Role               string    `json:"role"`         // "manager" or "worker"
	Status             string    `json:"status"`       // "ready", "down", "disconnected", "unknown"
	Availability       string    `json:"availability"` // "active", "pause", "drain"
	EngineVersion      string    `json:"engine_version,omitempty"`
	Address            string    `json:"address,omitempty"`
	TaskCount          int       `json:"task_count"`
	FirstSeenAt        time.Time `json:"first_seen_at"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	LastStatusChangeAt time.Time `json:"last_status_change_at"`
}
