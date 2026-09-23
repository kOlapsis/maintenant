package statuspage

import (
	"bytes"
	"net/http"

	"github.com/kolapsis/maintenant/internal/status"
)

var assetMIMEAllowlist = map[status.AssetRole][]string{
	status.AssetRoleLogo:    {"image/png", "image/jpeg", "image/webp", "image/svg+xml"},
	status.AssetRoleFavicon: {"image/png", "image/x-icon", "image/vnd.microsoft.icon", "image/svg+xml"},
	status.AssetRoleHero:    {"image/png", "image/jpeg", "image/webp"},
}

// DetectAssetMIME sniffs the MIME type from the first 512 bytes and validates it for the given role.
func DetectAssetMIME(role status.AssetRole, head []byte) (string, error) {
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
		return "", status.ErrAssetUnsupportedMIME
	}

	for _, m := range allowed {
		if sniffed == m {
			return sniffed, nil
		}
	}
	return "", status.ErrAssetUnsupportedMIME
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

var assetSizeCaps = map[status.AssetRole]int64{
	status.AssetRoleLogo:    200 * 1024,
	status.AssetRoleFavicon: 50 * 1024,
	status.AssetRoleHero:    500 * 1024,
}

// AssetSizeCap returns the max allowed bytes for the given role.
func AssetSizeCap(role status.AssetRole) int64 {
	return assetSizeCaps[role]
}
