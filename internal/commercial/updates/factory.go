// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/update"
)

// NewEnricher builds the CVE, changelog and risk enricher when the running edition opens CVE enrichment, nil otherwise.
func NewEnricher(d extpoint.EnricherDeps) update.Enricher {
	if !extension.Allows(extension.CapCVEEnrichment) {
		return nil
	}
	e := NewProEnricher(
		d.Store,
		NewCVEClient(d.Store, d.Logger.With("component", "cve")),
		NewChangelogResolver(d.Registry, d.Logger.With("component", "changelog")),
		NewRiskEngine(),
		NewEcosystemResolver(d.Registry, d.Logger.With("component", "ecosystem")),
		d.Logger.With("component", "enricher"),
	)
	d.Logger.Info("update enrichment pipeline enabled (Pro)")
	return e
}
