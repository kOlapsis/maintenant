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

package runtime

import "time"

// Resource types for RuntimeEvent.
const (
	ResourceContainer = "container"
	ResourceService   = "service"
	ResourceNode      = "node"
)

// RuntimeEvent is a normalized state change from any runtime.
type RuntimeEvent struct {
	Action       string
	ExternalID   string
	Name         string
	ExitCode     string
	HealthStatus string
	ErrorDetail  string
	ResourceType string // "container", "service", or "node"
	Timestamp    time.Time
	Labels       map[string]string
}
