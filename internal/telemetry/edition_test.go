// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import (
	"testing"

	"github.com/kolapsis/maintenant/internal/extension"
)

func TestMapEdition(t *testing.T) {
	cases := []struct {
		name string
		in   extension.Edition
		want string
	}{
		{"community resolves to community", extension.Community, "community"},
		{"enterprise resolves to pro", extension.Enterprise, "pro"},
		{"zero-value resolves to community", extension.Edition(""), "community"},
		{"unknown resolves to community", extension.Edition("garbage"), "community"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapEdition(tc.in)
			if got != tc.want {
				t.Fatalf("mapEdition(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
