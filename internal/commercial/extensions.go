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
