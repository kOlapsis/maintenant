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

// The three values ever emitted on the wire. Trial, paid and in-grace states
// all collapse to the edition they currently grant.
const (
	editionCommunity = "community"
	editionPersonal  = "personal"
	editionPro       = "pro"
)

// mapEdition translates the in-process extension.Edition to the stable wire
// value. It reports the edition actually in force, not the one that was bought:
// a Pro instance whose license has expired emits "community". An edition this
// build does not recognise also emits "community", rather than inventing a
// value consumers would have to guess at.
func mapEdition(e extension.Edition) string {
	switch e {
	case extension.Pro:
		return editionPro
	case extension.Personal:
		return editionPersonal
	default:
		return editionCommunity
	}
}
