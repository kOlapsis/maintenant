// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/status"
)

const svgOpen = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 10 10">`

func TestDetectAssetMIME_AcceptsInertSVG(t *testing.T) {
	for name, doc := range map[string]string{
		"with declaration":    `<?xml version="1.0" encoding="UTF-8"?>` + svgOpen + `<rect width="10" height="10"/></svg>`,
		"without declaration": svgOpen + `<circle cx="5" cy="5" r="4" fill="#22C55E"/></svg>`,
		"comment first":       `<!-- logo -->` + svgOpen + `<a href="https://example.com"><text>on time</text></a></svg>`,
		"doctype":             `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">` + svgOpen + `</svg>`,
	} {
		t.Run(name, func(t *testing.T) {
			mime, err := DetectAssetMIME(status.AssetRoleLogo, []byte(doc))
			require.NoError(t, err)
			assert.Equal(t, "image/svg+xml", mime)
		})
	}
}

func TestDetectAssetMIME_RefusesActiveSVG(t *testing.T) {
	padding := strings.Repeat(`<rect width="1" height="1"/>`, 40)
	for name, body := range map[string]string{
		"script far down":         padding + `<script>alert(1)</script>`,
		"script in capitals":      `<SCRIPT>alert(1)</SCRIPT>`,
		"namespaced script":       `<svg:script xmlns:svg="http://www.w3.org/2000/svg">alert(1)</svg:script>`,
		"onload":                  `<rect width="1" height="1" onload="alert(1)"/>`,
		"event in capitals":       `<rect width="1" height="1" ONCLICK = "alert(1)"/>`,
		"javascript href":         `<a href="javascript:alert(1)"><text>x</text></a>`,
		"javascript xlink:href":   `<a xlink:href="  JaVaScRiPt:alert(1)"><text>x</text></a>`,
		"javascript split":        "<a href=\"java\tscript:alert(1)\"><text>x</text></a>",
		"javascript entity split": `<a href="java&#x09;script:alert(1)"><text>x</text></a>`,
		"animated href":           `<a><set attributeName="href" to="javascript:alert(1)"/><text>x</text></a>`,
		"foreignObject":           `<foreignObject width="10" height="10"><div>x</div></foreignObject>`,
		"xhtml element":           `<html:iframe xmlns:html="http://www.w3.org/1999/xhtml" src="https://example.com"/>`,
		"undeclared entity":       `<text>&payload;</text>`,
		"stylesheet instruction":  `<?xml-stylesheet href="evil.xsl" type="text/xsl"?>`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DetectAssetMIME(status.AssetRoleLogo, []byte(svgOpen+body+`</svg>`))
			assert.ErrorIs(t, err, status.ErrAssetActiveSVG)
		})
	}
}

func TestDetectAssetMIME_SVGStaysOutOfTheHero(t *testing.T) {
	_, err := DetectAssetMIME(status.AssetRoleHero, []byte(svgOpen+`</svg>`))
	assert.ErrorIs(t, err, status.ErrAssetUnsupportedMIME)
}

func TestDetectAssetMIME_PlainTextIsNotAnImage(t *testing.T) {
	_, err := DetectAssetMIME(status.AssetRoleLogo, []byte("just some text"))
	assert.ErrorIs(t, err, status.ErrAssetUnsupportedMIME)
}
