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
type SessionChecker interface {
	IsConnected(agentID string) bool
}

// AgentLogFetcher reads logs of a container living on a remote agent's host,
// which the server's own runtime cannot see. Satisfied by *agentserver.Sessions.
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
	Containers    *container.Service
	Endpoints     *endpoint.Service
	Heartbeats    *heartbeat.Service
	Certificates  *certificate.Service
	Resources     *resource.Service
	Alerts        alert.AlertStore
	Channels      alert.ChannelStore
	Triggers      alert.TriggerStore
	Escalator     alert.Escalator
	Updates       *update.Service
	Incidents     status.IncidentStore
	Maintenance   status.MaintenanceStore
	Runtime       runtime.Runtime
	LogFetcher    LogFetcher
	EscalationSvc *escalation.Service
	Agents        AgentLister
	Sessions      SessionChecker
	AgentLogs     AgentLogFetcher

	// Security & supply-chain (read-only MCP surface).
	SecuritySvc *security.Service
	Scorer      *security.Scorer
	UpdateStore update.UpdateStore

	// Orchestrators (read-only MCP surface).
	Kubernetes     K8sReader
	SwarmCluster   func() *swarm.SwarmCluster
	SwarmDiscovery func() *swarm.ServiceDiscovery
	SwarmTopology  SwarmTopologyReader
	SwarmNodes     SwarmNodeReader

	Version string
	Logger  *slog.Logger
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
	registerSecurityTools(server, svc)
	registerKubernetesTools(server, svc)
	registerSwarmTools(server, svc)

	return server
}
