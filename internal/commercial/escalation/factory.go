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
package escalation

import (
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// NewEscalation builds the escalation service in every edition, and its escalator only when the running edition opens escalation.
func NewEscalation(d extpoint.EscalationDeps) extpoint.Escalation {
	svc := NewService(d.Store, d.Channels, extension.CurrentEdition, d.Suppressor, d.Logger.With("component", "escalation"))
	out := extpoint.Escalation{Service: svc}
	if extension.Allows(extension.CapAlertEscalation) {
		out.Escalator = NewRunner(RunnerDeps{
			Store:        d.Store,
			AlertStore:   d.Alerts,
			ChannelStore: d.Channels,
			Notifier:     d.Notifier,
			Suppressor:   d.Suppressor,
			Service:      svc,
			Logger:       d.Logger.With("component", "escalation-runner"),
		})
		d.Logger.Info("escalation runner enabled (Pro)")
	}
	return out
}
