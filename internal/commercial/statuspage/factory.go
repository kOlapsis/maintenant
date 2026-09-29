// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/status"
)

// NewStatusPage builds the incident, subscriber, maintenance and personalization features in every edition; their admin routes are gated by capability.
func NewStatusPage(d extpoint.StatusPageDeps) extpoint.StatusPage {
	return extpoint.StatusPage{
		Incidents:       NewIncidentHandler(d.Components, d.Incidents, d.Service, d.Logger),
		Notifier:        NewSubscriberNotifier(d.Subscribers, nil, d.BaseURL, d.Logger),
		Maintenance:     NewMaintenanceScheduler(d.Maintenance, d.Components, d.Incidents, d.Service, d.Logger),
		Personalization: NewPersonalizationService(d.Personalization, d.Logger.With("component", "personalization")),
		Mailer: func(cfg status.SmtpConfig) status.Mailer {
			return NewSmtpClient(cfg)
		},
	}
}
