// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/update"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// LogFetcher fetches container logs from the runtime.
type LogFetcher interface {
	FetchLogs(ctx context.Context, externalID string, lines int, timestamps bool) ([]string, error)
}

// AgentLister lists registered agents.
type AgentLister interface {
	List(ctx context.Context, statusFilter string) ([]*agent.Agent, error)
}

// SessionChecker reports whether an agent currently has an active gRPC stream.
type ChannelTester interface {
	SendTestWebhook(ctx context.Context, ch *alert.NotificationChannel) (int, error)
}

// ChannelValidators returns the validator of a channel type, if it has one.
type ChannelValidators interface {
	Validator(chType string) (alert.ChannelValidator, bool)
}

type SessionChecker interface {
	IsConnected(agentID string) bool
}

// IncidentAnnouncer pushes incident changes to the public status page and emails its subscribers.
type IncidentAnnouncer interface {
	AnnounceIncident(ctx context.Context, inc *status.Incident, message string)
	AnnounceIncidentUpdate(ctx context.Context, inc *status.Incident, upd *status.IncidentUpdate)
}

// AgentLogFetcher reads logs of a container living on a remote agent's host,
// which the server's own runtime cannot see. Satisfied by the multi-host session registry.
type AgentLogFetcher interface {
	FetchLogs(ctx context.Context, agentID, externalID string, lines int, timestamps bool) ([]string, error)
}

// K8sReader exposes the read methods of the Kubernetes store needed by MCP.
type K8sReader interface {
	ListNamespaces(ctx context.Context, agentID string) ([]string, error)
	ListWorkloads(ctx context.Context, agentID string, namespaces []string) ([]kubernetes.K8sWorkloadGroup, error)
	ListPods(ctx context.Context, agentID string, namespaces []string, filters kubernetes.PodFilters) ([]kubernetes.K8sPod, error)
	ListNodes(ctx context.Context, agentID string) ([]kubernetes.K8sNode, error)
}

// SwarmTopologyReader exposes the read methods of the Swarm topology store.
type SwarmTopologyReader interface {
	ListServices(ctx context.Context, agentID string) ([]*swarm.SwarmService, error)
	ListTasks(ctx context.Context, agentID, serviceID string) ([]*swarm.SwarmTask, error)
}

// SwarmNodeReader exposes the read methods of the Swarm node store.
type SwarmNodeReader interface {
	ListNodes(ctx context.Context, agentID string) ([]*swarm.SwarmNode, error)
}

// Services holds all dependencies required by MCP tool handlers.
type Services struct {
	Containers        *container.Service
	Endpoints         *endpoint.Service
	Heartbeats        *heartbeat.Service
	Certificates      *certificate.Service
	Resources         *resource.Service
	Alerts            alert.AlertStore
	Channels          alert.ChannelStore
	Triggers          alert.TriggerStore
	Escalator         alert.Escalator
	ChannelTester     ChannelTester
	ChannelValidators ChannelValidators
	Updates           *update.Service
	Incidents         status.IncidentStore
	IncidentAnnouncer IncidentAnnouncer
	Maintenance       status.MaintenanceStore
	StatusComponents  status.ComponentStore
	Runtime           runtime.Runtime
	LogFetcher        LogFetcher
	EscalationSvc     escalation.Service
	Agents            AgentLister
	Sessions          SessionChecker
	AgentLogs         AgentLogFetcher
	EOL               *eol.Service

	// Security & supply-chain (read-only MCP surface).
	SecuritySvc *security.Service
	Scorer      security.PostureScorer
	UpdateStore update.UpdateStore

	// Orchestrators (read-only MCP surface).
	Kubernetes     K8sReader
	SwarmCluster   func() *swarm.SwarmCluster
	SwarmDiscovery func() *swarm.ServiceDiscovery
	SwarmTopology  SwarmTopologyReader
	SwarmNodes     SwarmNodeReader

	// AllowPrivateWebhooks mirrors the REST flag: it relaxes the SSRF guard on
	// channel destinations in development.
	AllowPrivateWebhooks bool
	// Broadcast forwards a store change to the SSE brokers so an interface open
	// on the page sees an MCP write without reloading.
	Broadcast func(eventType string, data any)

	Version  string
	Logger   *slog.Logger
	DemoMode bool
}

func (s *Services) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func addTool[In, Out any](server *gomcp.Server, svc *Services, t *gomcp.Tool, h gomcp.ToolHandlerFor[In, Out]) {
	if svc.DemoMode && (t.Annotations == nil || !t.Annotations.ReadOnlyHint) {
		return
	}
	gomcp.AddTool(server, t, h)
}

// NewServer creates and configures an MCP server with all maintenant tools registered.
func NewServer(svc *Services) *gomcp.Server {
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "maintenant",
		Version: svc.Version,
	}, &gomcp.ServerOptions{
		Instructions: "maintenant infrastructure monitoring server. Provides real-time access to container states, endpoint health, heartbeat monitors, TLS certificates, resource metrics, alerts, and update intelligence.",
		Logger:       svc.Logger,
	})

	registerReadTools(server, svc)
	registerWriteTools(server, svc)
	registerEscalationTools(server, svc)
	registerTriggerTools(server, svc)
	registerChannelTools(server, svc)
	registerEditionTools(server, svc)
	registerSecurityTools(server, svc)
	registerKubernetesTools(server, svc)
	registerSwarmTools(server, svc)

	return server
}
