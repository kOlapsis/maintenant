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
// Package extpoint declares the implementations a licensed build can plug into the core.
package extpoint

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"

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
