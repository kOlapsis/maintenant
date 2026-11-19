// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package maintenance

import (
	"context"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

type editionGated struct {
	inner alert.MaintenanceSuppressor
}

func (g editionGated) IsSuppressed(ctx context.Context, source, entityType, entityID string) (bool, error) {
	if !extension.Allows(extension.CapMaintenanceWindows) {
		return false, nil
	}
	return g.inner.IsSuppressed(ctx, source, entityType, entityID)
}

// NewMaintenanceSuppressor builds the suppressor in every edition; it suppresses only while the running edition opens maintenance windows.
func NewMaintenanceSuppressor(d extpoint.SuppressorDeps) alert.MaintenanceSuppressor {
	return editionGated{inner: NewSuppressor(d.Windows, d.Logger.With("component", "maintenance-suppressor"))}
}
