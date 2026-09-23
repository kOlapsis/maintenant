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
package posture

import (
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/security"
)

// NewPostureScorer builds the posture scorer in every edition; the routes that read it are gated by capability.
func NewPostureScorer(d extpoint.PostureDeps) security.PostureScorer {
	return NewScorer(ScorerDeps{
		Certs:          d.Certs,
		CVEs:           d.CVEs,
		CVEEvaluations: d.CVEEvaluations,
		Updates:        d.Updates,
		Insights:       d.Insights,
		Acks:           d.Acks,
		Threshold:      d.Threshold,
	})
}
