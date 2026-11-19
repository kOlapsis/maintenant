package updates

import (
	"context"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/update"
)

type editionGated struct {
	inner update.Enricher
}

func (g editionGated) Enrich(ctx context.Context, results []update.UpdateResult) error {
	if !extension.Allows(extension.CapCVEEnrichment) {
		return nil
	}
	return g.inner.Enrich(ctx, results)
}

// NewEnricher builds the enrichment pipeline in every edition; it enriches only while the running edition opens CVE enrichment.
func NewEnricher(d extpoint.EnricherDeps) update.Enricher {
	return editionGated{inner: NewProEnricher(
		d.Store,
		NewCVEClient(d.Store, d.Logger.With("component", "cve")),
		NewChangelogResolver(d.Registry, d.Logger.With("component", "changelog")),
		NewRiskEngine(d.Insights, d.Restarts),
		NewEcosystemResolver(d.Registry, d.Logger.With("component", "ecosystem")),
		d.Logger.With("component", "enricher"),
	)}
}
