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

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		id        string
		versionID string
		product   string
		cycle     string
		ok        bool
	}{
		{id: "debian", versionID: "12", product: "debian", cycle: "12", ok: true},
		{id: "ubuntu", versionID: "22.04", product: "ubuntu", cycle: "22.04", ok: true},
		{id: "rhel", versionID: "9.4", product: "rhel", cycle: "9", ok: true},
		{id: "rocky", versionID: "9.4", product: "rocky-linux", cycle: "9", ok: true},
		{id: "almalinux", versionID: "9.4", product: "almalinux", cycle: "9", ok: true},
		{id: "alpine", versionID: "3.20.1", product: "alpine-linux", cycle: "3.20", ok: true},
		{id: "alpine", versionID: "3.20.10", product: "alpine-linux", cycle: "3.20", ok: true},
		{id: "sles", versionID: "15.6", product: "sles", cycle: "15.6", ok: true},
		{id: "raspbian", versionID: "12"},
		{id: "linuxmint", versionID: "22"},
		{id: "pop", versionID: "22.04"},
		{id: "fedora", versionID: "42"},
		{id: "debian", versionID: ""},
		{id: "", versionID: "12"},
	}

	for _, tc := range cases {
		product, cycle, ok := Match(tc.id, tc.versionID)
		if ok != tc.ok || product != tc.product || cycle != tc.cycle {
			t.Errorf("Match(%q, %q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.id, tc.versionID, product, cycle, ok, tc.product, tc.cycle, tc.ok)
		}
	}
}
