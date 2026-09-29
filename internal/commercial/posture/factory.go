// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

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
