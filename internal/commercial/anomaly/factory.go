// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// NewAnomaly builds the anomaly detection API in every edition, and its jobs unless MAINTENANT_ANOMALY_ENABLED turns them off.
func NewAnomaly(d extpoint.AnomalyDeps) extpoint.Anomaly {
	logger := d.Logger.With("component", "anomaly")
	cfg := ConfigFromEnv(logger)
	out := extpoint.Anomaly{API: NewHandler(d.Store, cfg, logger)}
	if cfg.Enabled {
		out.Jobs = newJobs(d, cfg, logger)
	}
	return out
}
