// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package maintenance

import (
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
)

// NewMaintenanceSuppressor builds the suppressor when the running edition opens maintenance windows, nil otherwise.
func NewMaintenanceSuppressor(d extpoint.SuppressorDeps) alert.MaintenanceSuppressor {
	if !extension.Allows(extension.CapMaintenanceWindows) {
		return nil
	}
	return NewSuppressor(d.Windows, d.Logger.With("component", "maintenance-suppressor"))
}
