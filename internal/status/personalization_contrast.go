package status

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func parseHexColor(hex string) ([3]float64, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 && len(hex) != 8 {
		return [3]float64{}, fmt.Errorf("invalid hex color")
	}
	r, err := strconv.ParseUint(hex[0:2], 16, 8)
	if err != nil {
		return [3]float64{}, err
	}
	g, err := strconv.ParseUint(hex[2:4], 16, 8)
	if err != nil {
		return [3]float64{}, err
	}
	b, err := strconv.ParseUint(hex[4:6], 16, 8)
	if err != nil {
		return [3]float64{}, err
	}
	return [3]float64{float64(r) / 255, float64(g) / 255, float64(b) / 255}, nil
}

func relativeLuminance(rgb [3]float64) float64 {
	lin := func(c float64) float64 {
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(rgb[0]) + 0.7152*lin(rgb[1]) + 0.0722*lin(rgb[2])
}

func ContrastRatio(fgHex, bgHex string) (float64, error) {
	fg, err := parseHexColor(fgHex)
	if err != nil {
		return 0, fmt.Errorf("fg color %q: %w", fgHex, err)
	}
	bg, err := parseHexColor(bgHex)
	if err != nil {
		return 0, fmt.Errorf("bg color %q: %w", bgHex, err)
	}
	lFG := relativeLuminance(fg)
	lBG := relativeLuminance(bg)
	light := math.Max(lFG, lBG)
	dark := math.Min(lFG, lBG)
	return (light + 0.05) / (dark + 0.05), nil
}

// EvaluatePalette returns warnings for pairs that fail WCAG AA contrast ratios.
func EvaluatePalette(p Palette) []ContrastWarning {
	type pair struct {
		name      string
		fg        string
		bg        string
		threshold float64 // 4.5 for normal text, 3.0 for UI components
	}

	pairs := []pair{
		{"text_on_bg", p.Text, p.Background, 4.5},
		{"accent_on_bg", p.Accent, p.Background, 4.5},
		{"accent_on_surface", p.Accent, p.Surface, 4.5},
		{"status_operational_on_bg", p.StatusOperational, p.Background, 3.0},
		{"status_operational_on_surface", p.StatusOperational, p.Surface, 3.0},
		{"status_degraded_on_bg", p.StatusDegraded, p.Background, 3.0},
		{"status_degraded_on_surface", p.StatusDegraded, p.Surface, 3.0},
		{"status_partial_on_bg", p.StatusPartialOutage, p.Background, 3.0},
		{"status_partial_on_surface", p.StatusPartialOutage, p.Surface, 3.0},
		{"status_major_on_bg", p.StatusMajorOutage, p.Background, 3.0},
		{"status_major_on_surface", p.StatusMajorOutage, p.Surface, 3.0},
	}

	var warnings []ContrastWarning
	for _, pair := range pairs {
		ratio, err := ContrastRatio(pair.fg, pair.bg)
		if err != nil {
			continue
		}
		ratio = math.Round(ratio*100) / 100
		if ratio < pair.threshold {
			warnings = append(warnings, ContrastWarning{
				Pair:            pair.name,
				Ratio:           ratio,
				WCAGAAThreshold: pair.threshold,
				Severity:        "fail",
			})
		}
	}
	return warnings
}
