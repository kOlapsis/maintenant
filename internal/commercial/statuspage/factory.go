// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"github.com/kolapsis/maintenant/internal/commercial/channels"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// NewStatusPage builds the incident, subscriber, maintenance and personalization features in every edition, emailing through the SMTP server of the environment when one is set.
func NewStatusPage(d extpoint.StatusPageDeps) extpoint.StatusPage {
	sp := extpoint.StatusPage{
		Incidents:       NewIncidentHandler(d.Components, d.Incidents, d.Service, d.Logger),
		Maintenance:     NewMaintenanceScheduler(d.Maintenance, d.Components, d.Incidents, d.Service, d.Logger),
		Personalization: NewPersonalizationService(d.Personalization, d.Logger.With("component", "personalization")),
	}
	if d.SMTP.Host != "" {
		mailer := channels.NewSMTPSender(channels.SMTPConfig(d.SMTP))
		sp.Mailer = mailer
		sp.Notifier = NewSubscriberNotifier(d.Subscribers, mailer, d.BaseURL, d.Logger)
	}
	return sp
}
