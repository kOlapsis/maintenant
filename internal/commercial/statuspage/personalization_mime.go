// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/kolapsis/maintenant/internal/status"
)

var assetMIMEAllowlist = map[status.AssetRole][]string{
	status.AssetRoleLogo:    {"image/png", "image/jpeg", "image/webp", "image/svg+xml"},
	status.AssetRoleFavicon: {"image/png", "image/x-icon", "image/vnd.microsoft.icon", "image/svg+xml"},
	status.AssetRoleHero:    {"image/png", "image/jpeg", "image/webp"},
}

// DetectAssetMIME sniffs the MIME type of the whole upload and validates it for the given role, refusing an SVG that carries active content.
func DetectAssetMIME(role status.AssetRole, data []byte) (string, error) {
	sniffed := http.DetectContentType(data)
	// Normalize: strip parameters (e.g., "text/xml; charset=utf-8")
	for i, c := range sniffed {
		if c == ';' || c == ' ' {
			sniffed = sniffed[:i]
			break
		}
	}

	// http.DetectContentType has no SVG signature: it reports XML, HTML or plain text.
	if strings.HasPrefix(sniffed, "text/") || sniffed == "application/xml" {
		switch inspectSVG(data) {
		case svgInert:
			sniffed = "image/svg+xml"
		case svgActive:
			return "", status.ErrAssetActiveSVG
		}
	}

	allowed, ok := assetMIMEAllowlist[role]
	if !ok {
		return "", status.ErrAssetUnsupportedMIME
	}

	for _, m := range allowed {
		if sniffed == m {
			return sniffed, nil
		}
	}
	return "", status.ErrAssetUnsupportedMIME
}

type svgVerdict int

const (
	notSVG svgVerdict = iota
	svgInert
	svgActive
)

const xhtmlNamespace = "http://www.w3.org/1999/xhtml"

var activeSVGElements = map[string]bool{
	"script": true, "foreignobject": true, "iframe": true, "embed": true, "object": true, "handler": true,
}

// inspectSVG reads the whole document and flags as active any script, event handler, script URL, embedded HTML or unparsable content.
func inspectSVG(data []byte) svgVerdict {
	dec := xml.NewDecoder(bytes.NewReader(data))
	rootSeen := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			if rootSeen {
				return svgInert
			}
			return notSVG
		}
		if err != nil {
			if rootSeen {
				return svgActive
			}
			return notSVG
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !rootSeen {
				if !strings.EqualFold(t.Name.Local, "svg") {
					return notSVG
				}
				rootSeen = true
			}
			if t.Name.Space == xhtmlNamespace || activeSVGElements[strings.ToLower(t.Name.Local)] {
				return svgActive
			}
			for _, a := range t.Attr {
				if strings.HasPrefix(strings.ToLower(a.Name.Local), "on") || carriesScriptURL(a.Value) {
					return svgActive
				}
			}
		case xml.ProcInst:
			if rootSeen || !strings.EqualFold(t.Target, "xml") {
				return svgActive
			}
		}
	}
}

// carriesScriptURL reports whether an attribute value holds a script URL, however it is cased or split by blanks.
func carriesScriptURL(v string) bool {
	folded := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, v)
	return strings.Contains(folded, "javascript:") || strings.Contains(folded, "vbscript:")
}

var assetSizeCaps = map[status.AssetRole]int64{
	status.AssetRoleLogo:    200 * 1024,
	status.AssetRoleFavicon: 50 * 1024,
	status.AssetRoleHero:    500 * 1024,
}

// AssetSizeCap returns the max allowed bytes for the given role.
func AssetSizeCap(role status.AssetRole) int64 {
	return assetSizeCaps[role]
}
