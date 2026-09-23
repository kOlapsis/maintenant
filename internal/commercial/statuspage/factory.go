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
