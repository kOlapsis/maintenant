// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import "testing"

func TestParseDisableTelemetry_Truthy(t *testing.T) {
	truthy := []string{
		"1", "t", "T", "true", "True", "TRUE",
		"y", "Y", "yes", "Yes", "YES",
		"on", "On", "ON",
		" yes ", "  true\t", "\non\n", "TRUE  ",
	}
	for _, in := range truthy {
		t.Run(in, func(t *testing.T) {
			first := parseDisableTelemetry(in)
			second := parseDisableTelemetry(in)
			if !first {
				t.Fatalf("parseDisableTelemetry(%q) = false, want true", in)
			}
			if first != second {
				t.Fatalf("parseDisableTelemetry(%q) is non-idempotent (%v vs %v)", in, first, second)
			}
		})
	}
}

func TestParseDisableTelemetry_Falsy(t *testing.T) {
	falsy := []string{
		"", " ", "\t", "\n",
		"0", "f", "false", "False", "FALSE",
		"n", "no", "No", "NO",
		"off", "Off", "OFF",
		"disabled", "enabled", "garbage", "yes!",
		"2", "0.5", "true ish",
	}
	for _, in := range falsy {
		t.Run(in, func(t *testing.T) {
			if parseDisableTelemetry(in) {
				t.Fatalf("parseDisableTelemetry(%q) = true, want false", in)
			}
		})
	}
}
