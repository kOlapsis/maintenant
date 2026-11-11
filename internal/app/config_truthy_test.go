// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import "testing"

func TestParseTruthy_Truthy(t *testing.T) {
	truthy := []string{
		"1", "t", "T", "true", "True", "TRUE",
		"y", "Y", "yes", "Yes", "YES",
		"on", "On", "ON",
		" yes ", "  true\t", "\non\n", "TRUE  ",
	}
	for _, in := range truthy {
		t.Run(in, func(t *testing.T) {
			first := parseTruthy(in)
			second := parseTruthy(in)
			if !first {
				t.Fatalf("parseTruthy(%q) = false, want true", in)
			}
			if first != second {
				t.Fatalf("parseTruthy(%q) is non-idempotent (%v vs %v)", in, first, second)
			}
		})
	}
}

func TestParseTruthy_Falsy(t *testing.T) {
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
			if parseTruthy(in) {
				t.Fatalf("parseTruthy(%q) = true, want false", in)
			}
		})
	}
}
