// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package commercial

import (
	"github.com/kolapsis/maintenant/internal/commercial/channels"
	"github.com/kolapsis/maintenant/internal/commercial/escalation"
	"github.com/kolapsis/maintenant/internal/commercial/maintenance"
	"github.com/kolapsis/maintenant/internal/commercial/posture"
	"github.com/kolapsis/maintenant/internal/commercial/statuspage"
	"github.com/kolapsis/maintenant/internal/commercial/updates"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// Extensions returns the commercial implementation of every extension point.
func Extensions() extpoint.Set {
	return extpoint.Set{
		Enricher:      updates.NewEnricher,
		PostureScorer: posture.NewPostureScorer,
		Channels:      channels.NewChannels,
		StatusPage:    statuspage.NewStatusPage,
		Suppressor:    maintenance.NewMaintenanceSuppressor,
		Escalation:    escalation.NewEscalation,
	}
}
