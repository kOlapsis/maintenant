// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import "strings"

// parseDisableTelemetry returns true when the raw value of the
// MAINTENANT_DISABLE_TELEMETRY environment variable should disable telemetry.
// Truthy values: 1, t, true, y, yes, on (case-insensitive, whitespace-trimmed).
// Anything else — including 0, false, no, off, the empty string — returns
// false (telemetry stays enabled), per spec FR-005 / FR-007.
func parseDisableTelemetry(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}
