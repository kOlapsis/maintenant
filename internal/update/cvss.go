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
	"fmt"
	"math"
	"strings"
)

// ErrInvalidCVSS reports a malformed or incomplete CVSS v3.x base vector.
var ErrInvalidCVSS = errors.New("invalid CVSS v3 vector")

// CVSSMetrics is a parsed CVSS v3.0/v3.1 base vector.
type CVSSMetrics struct {
	Version string
	AV      string
	AC      string
	PR      string
	UI      string
	S       string
	C       string
	I       string
	A       string
}

var cvssBaseValues = map[string]map[string]struct{}{
	"AV": {"N": {}, "A": {}, "L": {}, "P": {}},
	"AC": {"L": {}, "H": {}},
	"PR": {"N": {}, "L": {}, "H": {}},
	"UI": {"N": {}, "R": {}},
	"S":  {"U": {}, "C": {}},
	"C":  {"N": {}, "L": {}, "H": {}},
	"I":  {"N": {}, "L": {}, "H": {}},
	"A":  {"N": {}, "L": {}, "H": {}},
}

var cvssTemporalKeys = map[string]struct{}{
	"E": {}, "RL": {}, "RC": {}, "CR": {}, "IR": {}, "AR": {},
	"MAV": {}, "MAC": {}, "MPR": {}, "MUI": {}, "MS": {}, "MC": {}, "MI": {}, "MA": {},
}

// ParseCVSSv3 parses a "CVSS:3.x/AV:../AC:../PR:../UI:../S:../C:../I:../A:.." base vector.
func ParseCVSSv3(vector string) (CVSSMetrics, error) {
	parts := strings.Split(vector, "/")
	if len(parts) < 1 {
		return CVSSMetrics{}, fmt.Errorf("%w: empty vector", ErrInvalidCVSS)
	}

	var version string
	switch parts[0] {
	case "CVSS:3.0":
		version = "3.0"
	case "CVSS:3.1":
		version = "3.1"
	default:
		return CVSSMetrics{}, fmt.Errorf("%w: unsupported prefix %q", ErrInvalidCVSS, parts[0])
	}

	m := CVSSMetrics{Version: version}
	seen := make(map[string]bool, 8)

	for _, part := range parts[1:] {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			return CVSSMetrics{}, fmt.Errorf("%w: malformed metric %q", ErrInvalidCVSS, part)
		}
		key, value := kv[0], kv[1]

		if _, ok := cvssTemporalKeys[key]; ok {
			continue
		}

		allowed, ok := cvssBaseValues[key]
		if !ok {
			return CVSSMetrics{}, fmt.Errorf("%w: unknown metric %q", ErrInvalidCVSS, key)
		}
		if _, ok := allowed[value]; !ok {
			return CVSSMetrics{}, fmt.Errorf("%w: invalid value %q for metric %q", ErrInvalidCVSS, value, key)
		}
		if seen[key] {
			return CVSSMetrics{}, fmt.Errorf("%w: duplicate metric %q", ErrInvalidCVSS, key)
		}
		seen[key] = true

		switch key {
		case "AV":
			m.AV = value
		case "AC":
			m.AC = value
		case "PR":
			m.PR = value
		case "UI":
			m.UI = value
		case "S":
			m.S = value
		case "C":
			m.C = value
		case "I":
			m.I = value
		case "A":
			m.A = value
		}
	}

	for key := range cvssBaseValues {
		if !seen[key] {
			return CVSSMetrics{}, fmt.Errorf("%w: missing metric %q", ErrInvalidCVSS, key)
		}
	}

	return m, nil
}

func cvssCIAValue(v string) float64 {
	switch v {
	case "H":
		return 0.56
	case "L":
		return 0.22
	default:
		return 0
	}
}

func cvssAVValue(v string) float64 {
	switch v {
	case "N":
		return 0.85
	case "A":
		return 0.62
	case "L":
		return 0.55
	default:
		return 0.2
	}
}

func cvssACValue(v string) float64 {
	if v == "L" {
		return 0.77
	}
	return 0.44
}

func cvssPRValue(v, scope string) float64 {
	if scope == "C" {
		switch v {
		case "N":
			return 0.85
		case "L":
			return 0.68
		default:
			return 0.5
		}
	}
	switch v {
	case "N":
		return 0.85
	case "L":
		return 0.62
	default:
		return 0.27
	}
}

func cvssUIValue(v string) float64 {
	if v == "N" {
		return 0.85
	}
	return 0.62
}

func cvssRoundup(input float64) float64 {
	intInput := int(math.Round(input * 100000))
	if intInput%10000 == 0 {
		return float64(intInput) / 100000.0
	}
	return float64(intInput/10000+1) / 10.0
}

// BaseScore computes the CVSS base score of the metrics using the v3.1 formulas
// (the v3.0 spec differs only in Roundup precision, not the score itself).
func (m CVSSMetrics) BaseScore() float64 {
	iss := 1 - (1-cvssCIAValue(m.C))*(1-cvssCIAValue(m.I))*(1-cvssCIAValue(m.A))

	var impact float64
	if m.S == "C" {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	} else {
		impact = 6.42 * iss
	}

	if impact <= 0 {
		return 0
	}

	exploitability := 8.22 * cvssAVValue(m.AV) * cvssACValue(m.AC) * cvssPRValue(m.PR, m.S) * cvssUIValue(m.UI)

	if m.S == "C" {
		return cvssRoundup(math.Min(1.08*(impact+exploitability), 10))
	}
	return cvssRoundup(math.Min(impact+exploitability, 10))
}

// SeverityForScore maps a CVSS base score to its qualitative severity rating.
func SeverityForScore(score float64) CVESeverity {
	if score >= 9.0 {
		return CVESeverityCritical
	}
	if score >= 7.0 {
		return CVESeverityHigh
	}
	if score >= 4.0 {
		return CVESeverityMedium
	}
	return CVESeverityLow
}
