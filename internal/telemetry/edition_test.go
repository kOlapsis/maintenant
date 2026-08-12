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
		{"personal is reported distinctly, never folded into community", extension.Personal, "personal"},
		{"pro resolves to pro", extension.Pro, "pro"},
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

// TestMapEdition_CoversEveryKnownEdition: a new edition must not silently be
// reported as community. This fails the day one is added without a wire value.
func TestMapEdition_CoversEveryKnownEdition(t *testing.T) {
	for _, e := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		if got := mapEdition(e); got != string(e) {
			t.Errorf("mapEdition(%q) = %q; every known edition must map to its own value", e, got)
		}
	}
}
