// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

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
