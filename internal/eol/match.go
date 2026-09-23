// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package eol

import "strings"

// Match maps an os-release ID and VERSION_ID onto a product slug and a support cycle.
func Match(id, versionID string) (product, cycle string, ok bool) {
	if versionID == "" {
		return "", "", false
	}
	switch id {
	case "debian", "ubuntu", "sles":
		return id, versionID, true
	case "rhel", "almalinux":
		return id, versionPrefix(versionID, 1), true
	case "rocky":
		return "rocky-linux", versionPrefix(versionID, 1), true
	case "alpine":
		return "alpine-linux", versionPrefix(versionID, 2), true
	}
	return "", "", false
}

func versionPrefix(versionID string, parts int) string {
	fields := strings.Split(versionID, ".")
	if len(fields) <= parts {
		return versionID
	}
	return strings.Join(fields[:parts], ".")
}
