// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package update

import "strings"

// tagOSVariants maps tag suffixes to OSV ecosystem identifiers.
var tagOSVariants = []struct {
	suffix    string
	ecosystem string
}{
	// Alpine variants (check versioned first)
	{"-alpine3.21", "Alpine:3.21"},
	{"-alpine3.20", "Alpine:3.20"},
	{"-alpine3.19", "Alpine:3.19"},
	{"-alpine3.18", "Alpine:3.18"},
	{"-alpine", "Alpine:3.20"},
	// Debian variants
	{"-slim-bookworm", "Debian:12"},
	{"-bookworm", "Debian:12"},
	{"-slim-bullseye", "Debian:11"},
	{"-bullseye", "Debian:11"},
	{"-slim-buster", "Debian:10"},
	{"-buster", "Debian:10"},
	// Ubuntu variants
	{"-noble", "Ubuntu:24.04"},
	{"-jammy", "Ubuntu:22.04"},
	{"-focal", "Ubuntu:20.04"},
}

// ParseTagOSVariant detects an OS variant from an image tag suffix.
// Returns the OSV ecosystem identifier and true if a variant was detected.
func ParseTagOSVariant(tag string) (string, bool) {
	if tag == "" {
		return "", false
	}

	lower := strings.ToLower(tag)

	// Check if the entire tag is an OS variant (e.g., "alpine", "bookworm-slim")
	switch {
	case lower == "alpine" || strings.HasPrefix(lower, "alpine3."):
		if strings.HasPrefix(lower, "alpine3.") {
			parts := strings.SplitN(lower[len("alpine"):], ".", 3)
			if len(parts) >= 2 {
				return "Alpine:" + parts[0] + "." + parts[1], true
			}
		}
		return "Alpine:3.20", true
	case lower == "bookworm" || lower == "bookworm-slim":
		return "Debian:12", true
	case lower == "bullseye" || lower == "bullseye-slim":
		return "Debian:11", true
	case lower == "noble":
		return "Ubuntu:24.04", true
	case lower == "jammy":
		return "Ubuntu:22.04", true
	case lower == "focal":
		return "Ubuntu:20.04", true
	}

	// Check for OS variant as suffix (e.g., "1.25-alpine", "16-bookworm")
	for _, v := range tagOSVariants {
		if strings.HasSuffix(lower, v.suffix) {
			return v.ecosystem, true
		}
	}

	return "", false
}
