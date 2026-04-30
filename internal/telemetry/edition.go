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

package telemetry

import "github.com/kolapsis/maintenant/internal/extension"

// editionCommunity / editionPro are the only two values ever emitted on the
// wire (per spec FR-001 and contracts/telemetry-payload.md). Trial, paid,
// and in-grace Pro states all collapse to "pro".
const (
	editionCommunity = "community"
	editionPro       = "pro"
)

// mapEdition translates the in-process extension.Edition value to the
// stable wire value. Anything that is not extension.Enterprise resolves
// to "community" — including a Pro-capable build whose license has
// expired or is missing.
func mapEdition(e extension.Edition) string {
	if e == extension.Enterprise {
		return editionPro
	}
	return editionCommunity
}
