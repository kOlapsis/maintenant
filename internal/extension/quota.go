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

package extension

// Resource names a capped resource type.
type Resource string

const (
	ResourceEndpoints        Resource = "endpoints"
	ResourceHeartbeats       Resource = "heartbeats"
	ResourceCertificates     Resource = "certificates"
	ResourceStatusComponents Resource = "status_components"
	ResourceAgentHosts       Resource = "agent_hosts"
)

// Unlimited is the limit value meaning "no cap". It is reported to the UI as-is.
const Unlimited = -1

// Limit returns the cap for r under the running edition. This is the only
// declaration of the caps: the value that refuses a creation and the value the
// interface displays both come from here, so they cannot drift apart.
//
// agent_hosts is 0 on Community by design — the multihost capability itself is
// Personal, so the REST routes refuse before any count happens. The 0 still
// matters: it is what the gRPC enrollment barrier reads, and it has no
// middleware in front of it.
func Limit(r Resource) int {
	edition := CurrentEdition()

	switch r {
	case ResourceEndpoints:
		if edition.AtLeast(Personal) {
			return Unlimited
		}
		return 10
	case ResourceHeartbeats:
		if edition.AtLeast(Personal) {
			return Unlimited
		}
		return 5
	case ResourceCertificates:
		if edition.AtLeast(Personal) {
			return Unlimited
		}
		return 5
	case ResourceStatusComponents:
		if edition.AtLeast(Personal) {
			return Unlimited
		}
		return 3
	case ResourceAgentHosts:
		switch {
		case edition.AtLeast(Pro):
			return Unlimited
		case edition.AtLeast(Personal):
			return 20
		default:
			return 0
		}
	default:
		// An undeclared resource is not one we hand out capacity for.
		return 0
	}
}
