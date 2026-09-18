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

// OSJSON renders the operating system block the agents and updates APIs both serve.
func OSJSON(identity Identity, reportedAt *time.Time, support Support) map[string]any {
	return map[string]any{
		"id":                 identity.ID,
		"version_id":         identity.VersionID,
		"pretty_name":        identity.PrettyName,
		"source":             identity.Source,
		"unavailable_reason": identity.UnavailableReason,
		"reported_at":        reportedAt,
		"support":            support,
	}
}

// HostJSON renders one host of HostSupports, without its connection state.
func HostJSON(h HostSupport) map[string]any {
	return map[string]any{
		"agent_id": h.AgentID,
		"hostname": h.Hostname,
		"label":    h.Label,
		"is_local": h.IsLocal,
		"runtime":  h.Runtime,
		"os":       OSJSON(h.Identity, h.ReportedAt, h.Support),
	}
}
