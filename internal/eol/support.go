// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package eol

import "time"

// WarningDays is how many days before the end of security support the warning starts.
const WarningDays = 30

// SupportState is where a host stands in its operating system support cycle.
type SupportState string

const (
	StateUnknown      SupportState = "unknown"
	StateUntracked    SupportState = "untracked"
	StateSupported    SupportState = "supported"
	StateSecurityOnly SupportState = "security_only"
	StateEndingSoon   SupportState = "ending_soon"
	StateEnded        SupportState = "ended"
)

// Identity is the operating system an agent reports, as reported.
type Identity struct {
	ID                string `json:"id"`
	VersionID         string `json:"version_id"`
	PrettyName        string `json:"pretty_name"`
	Source            string `json:"source"`
	UnavailableReason string `json:"unavailable_reason"`
}

// Support is the evaluated support state of one host.
type Support struct {
	State         SupportState `json:"state"`
	Product       string       `json:"product"`
	Cycle         string       `json:"cycle"`
	ActiveUntil   *Date        `json:"active_until"`
	SecurityUntil *Date        `json:"security_until"`
	ExtendedUntil *Date        `json:"extended_until"`
	DaysRemaining *int         `json:"days_remaining"`
	TableSource   string       `json:"table_source"`
}

// Evaluate resolves the support state of an identity against a table on a given day.
func Evaluate(identity Identity, table Table, today time.Time) Support {
	support := Support{TableSource: table.Source}

	if identity.ID == "" && (identity.UnavailableReason != "" || identity == Identity{}) {
		support.State = StateUnknown
		return support
	}

	product, cycleName, ok := Match(identity.ID, identity.VersionID)
	if !ok {
		support.State = StateUntracked
		return support
	}
	cycle, ok := table.Cycle(product, cycleName)
	if !ok {
		support.State = StateUntracked
		return support
	}

	securityUntil := cycle.SecurityUntil
	days := securityUntil.DaysUntil(today)
	support.Product = product
	support.Cycle = cycle.Name
	support.ActiveUntil = cycle.ActiveUntil
	support.SecurityUntil = &securityUntil
	support.ExtendedUntil = cycle.ExtendedUntil
	support.DaysRemaining = &days

	switch {
	case days < 0:
		support.State = StateEnded
	case days <= WarningDays:
		support.State = StateEndingSoon
	case cycle.ActiveUntil != nil && cycle.ActiveUntil.DaysUntil(today) < 0:
		support.State = StateSecurityOnly
	default:
		support.State = StateSupported
	}
	return support
}
