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

package update

import (
	"errors"
	"math"
	"testing"
)

func TestParseCVSSv3BaseScore(t *testing.T) {
	tests := []struct {
		name   string
		vector string
		want   float64
	}{
		{"none", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N", 0.0},
		{"critical unchanged", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8},
		{"critical changed", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H", 10.0},
		{"high AC", "CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:H", 8.1},
		{"medium changed", "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:C/C:L/I:L/A:N", 6.4},
		{"local high", "CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H", 7.8},
		{"user interaction", "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:N/I:N/A:H", 6.5},
		{"physical low", "CVSS:3.1/AV:P/AC:H/PR:H/UI:R/S:U/C:L/I:N/A:N", 1.6},
		{"adjacent", "CVSS:3.1/AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H", 6.5},
		{"v3.0 critical", "CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8},
		{"with temporal metrics", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H/E:P/RL:O/RC:C", 9.8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseCVSSv3(tt.vector)
			if err != nil {
				t.Fatalf("ParseCVSSv3(%q) returned error: %v", tt.vector, err)
			}
			got := m.BaseScore()
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("BaseScore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCVSSv3ACNotConfusedWithC(t *testing.T) {
	m, err := ParseCVSSv3("CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:H")
	if err != nil {
		t.Fatalf("ParseCVSSv3 returned error: %v", err)
	}
	if m.AC != "H" {
		t.Errorf("AC = %q, want %q", m.AC, "H")
	}
	if m.C != "H" {
		t.Errorf("C = %q, want %q", m.C, "H")
	}
	if got := m.BaseScore(); math.Abs(got-8.1) > 1e-9 {
		t.Errorf("BaseScore() = %v, want %v", got, 8.1)
	}
}

func TestParseCVSSv3Invalid(t *testing.T) {
	tests := []struct {
		name   string
		vector string
	}{
		{"empty", ""},
		{"unsupported version", "CVSS:2.0/AV:N/AC:L/Au:N/C:N/I:N/A:N"},
		{"missing metric", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N"},
		{"duplicate metric", "CVSS:3.1/AV:N/AV:A/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N"},
		{"unknown value", "CVSS:3.1/AV:X/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N"},
		{"garbage", "not a cvss vector"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCVSSv3(tt.vector)
			if !errors.Is(err, ErrInvalidCVSS) {
				t.Errorf("ParseCVSSv3(%q) error = %v, want errors.Is(err, ErrInvalidCVSS)", tt.vector, err)
			}
		})
	}
}

func TestSeverityForScore(t *testing.T) {
	tests := []struct {
		score float64
		want  CVESeverity
	}{
		{8.9, CVESeverityHigh},
		{9.0, CVESeverityCritical},
		{6.9, CVESeverityMedium},
		{7.0, CVESeverityHigh},
		{3.9, CVESeverityLow},
		{4.0, CVESeverityMedium},
	}

	for _, tt := range tests {
		got := SeverityForScore(tt.score)
		if got != tt.want {
			t.Errorf("SeverityForScore(%v) = %v, want %v", tt.score, got, tt.want)
		}
	}
}
