package status

import (
	"bytes"
	"net/http"
)

var assetMIMEAllowlist = map[AssetRole][]string{
	AssetRoleLogo:    {"image/png", "image/jpeg", "image/webp", "image/svg+xml"},
	AssetRoleFavicon: {"image/png", "image/x-icon", "image/vnd.microsoft.icon", "image/svg+xml"},
	AssetRoleHero:    {"image/png", "image/jpeg", "image/webp"},
}

// DetectAssetMIME sniffs the MIME type from the first 512 bytes and validates it for the given role.
func DetectAssetMIME(role AssetRole, head []byte) (string, error) {
	sniffed := http.DetectContentType(head)
	// Normalize: strip parameters (e.g., "text/xml; charset=utf-8")
	for i, c := range sniffed {
		if c == ';' || c == ' ' {
			sniffed = sniffed[:i]
			break
		}
	}

	// SVG fallback: http.DetectContentType returns "text/xml" for SVG
	if (sniffed == "text/xml" || sniffed == "application/xml") && isSVG(head) {
		sniffed = "image/svg+xml"
	}

	allowed, ok := assetMIMEAllowlist[role]
	if !ok {
		return "", ErrAssetUnsupportedMIME
	}

	for _, m := range allowed {
		if sniffed == m {
			return sniffed, nil
		}
	}
	return "", ErrAssetUnsupportedMIME
}

func isSVG(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	// skip XML declaration if present
	if bytes.HasPrefix(trimmed, []byte("<?xml")) {
		end := bytes.Index(trimmed, []byte("?>"))
		if end != -1 {
			trimmed = bytes.TrimSpace(trimmed[end+2:])
		}
	}
	lower := bytes.ToLower(trimmed)
	if !bytes.HasPrefix(lower, []byte("<svg")) {
		return false
	}
	// Reject if it contains a <script tag (basic XSS guard)
	return !bytes.Contains(lower, []byte("<script"))
}

var assetSizeCaps = map[AssetRole]int64{
	AssetRoleLogo:    200 * 1024,
	AssetRoleFavicon: 50 * 1024,
	AssetRoleHero:    500 * 1024,
}

// AssetSizeCap returns the max allowed bytes for the given role.
func AssetSizeCap(role AssetRole) int64 {
	return assetSizeCaps[role]
}
