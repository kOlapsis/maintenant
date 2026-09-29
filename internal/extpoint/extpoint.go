// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0
// Package extpoint declares the implementations a licensed build can plug into the core.
package extpoint

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/agentproto"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/swarm"

	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/update"
)

// Set holds one factory per extension point; a nil factory keeps the Community behaviour.
type Set struct {
	Enricher      func(EnricherDeps) update.Enricher
	PostureScorer func(PostureDeps) security.PostureScorer
	Channels      func(ChannelDeps) map[string]alert.ChannelSender
	StatusPage    func(StatusPageDeps) StatusPage
	Suppressor    func(SuppressorDeps) alert.MaintenanceSuppressor
	Escalation    func(EscalationDeps) Escalation
	MultiHost     func(MultiHostDeps) MultiHost
}

// EnricherDeps is what an update enricher is built from.
type EnricherDeps struct {
	Store    update.UpdateStore
	Registry *update.RegistryClient
	Logger   *slog.Logger
}

// PostureDeps is what a security posture scorer is built from.
type PostureDeps struct {
	Certs          security.CertificateReader
	CVEs           security.CVEReader
	CVEEvaluations security.CVEEvaluationReader
	Updates        security.UpdateReader
	Insights       security.InsightsReader
	Acks           security.AcknowledgmentStore
	Threshold      int
}

// ChannelDeps is what the notification channel senders are built from.
type ChannelDeps struct {
	HTTPClient *http.Client
	SMTP       SMTPConfig
	Logger     *slog.Logger
}

// SMTPConfig holds the SMTP connection parameters.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// StatusPageDeps is what the status page extensions are built from.
type StatusPageDeps struct {
	Service         *status.Service
	Components      status.ComponentStore
	Incidents       status.IncidentStore
	Maintenance     status.MaintenanceStore
	Subscribers     status.SubscriberStore
	Personalization status.PersonalizationStore
	BaseURL         string
	Logger          *slog.Logger
}

// StatusPage holds the status page features a licensed build adds.
type StatusPage struct {
	Incidents       status.AlertIncidentHandler
	Notifier        status.SubscriberNotifier
	Maintenance     status.MaintenanceRunner
	Personalization status.PersonalizationManager
	Mailer          func(status.SmtpConfig) status.Mailer
}

// MaintenanceWindows tells whether a monitor sits inside an active maintenance window.
type MaintenanceWindows interface {
	IsEntitySuppressed(ctx context.Context, monitorType string, monitorID string, now time.Time) (matched bool, windowID string, endsAt time.Time, err error)
}

// SuppressorDeps is what the maintenance suppressor is built from.
type SuppressorDeps struct {
	Windows MaintenanceWindows
	Logger  *slog.Logger
}

// EscalationDeps is what escalation is built from.
type EscalationDeps struct {
	Store      escalation.Store
	Alerts     alert.AlertStore
	Channels   alert.ChannelStore
	Notifier   *alert.Notifier
	Suppressor alert.MaintenanceSuppressor
	Logger     *slog.Logger
}

// Escalation holds the escalation service and, when the running edition opens it, the escalator.
type Escalation struct {
	Service   escalation.Service
	Escalator alert.Escalator
}

// EventBroadcaster pushes a server-sent event to connected browsers.
type EventBroadcaster interface {
	BroadcastEvent(eventType string, data any)
}

// MultiHostDeps is what the agent gRPC server is built from.
type MultiHostDeps struct {
	AgentStore         *store.AgentStore
	Broadcaster        EventBroadcaster
	RateLimitPerSecond int
	DemoMode           bool
	Container          *container.Service
	Resource           *resource.Service
	Endpoint           *endpoint.Service
	Certificate        *certificate.Service
	Heartbeat          *heartbeat.Service
	Swarm              *swarm.IngestService
	Kubernetes         *kubernetes.IngestService
	HostOS             *eol.Service
	LabelSync          func(ctx context.Context, agentID, containerName, externalID string, labels map[string]string)
	Logger             *slog.Logger
}

// AgentSessions is the live registry of agents streaming to this server.
type AgentSessions interface {
	IsConnected(agentID string) bool
	Close(agentID, reason string)
	HasCapability(agentID, capability string) bool
	FetchLogs(ctx context.Context, agentID, externalID string, lines int, timestamps bool) ([]string, error)
	SendCommand(ctx context.Context, agentID, capability string, cmd *agentpb.AgentCommand) (<-chan *agentpb.CommandResult, func(), error)
	SpoolStatus(agentID string) *agentproto.SpoolState
	SetLifecycleAlertHook(fn func(agentID, reason string, connected bool))
	StartWatchers(ctx context.Context, staleThreshold time.Duration, staleAgents func(ctx context.Context, threshold time.Duration) ([]string, error))
}

// GRPCConfig is where and how the agent gRPC server listens.
type GRPCConfig struct {
	Listen      string
	PublicURL   string
	TLSCertFile string
	TLSKeyFile  string
	Insecure    bool
}

// MultiHost holds the agent session registry and the function that serves agents over gRPC.
type MultiHost struct {
	Sessions AgentSessions
	Serve    func(ctx context.Context, cfg GRPCConfig) error
}
