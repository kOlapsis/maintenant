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
	"log/slog"
	"net/http"

	"github.com/kolapsis/maintenant/internal/alert"

	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/update"
)

// Set holds one factory per extension point; a nil factory keeps the Community behaviour.
type Set struct {
	Enricher      func(EnricherDeps) update.Enricher
	PostureScorer func(PostureDeps) security.PostureScorer
	Channels      func(ChannelDeps) map[string]alert.ChannelSender
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
