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
