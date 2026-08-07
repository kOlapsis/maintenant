// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The digest a browser computes for the body of `<script>alert(1)</script>`,
// obtained independently (openssl dgst -sha256 -binary | openssl base64). If
// the hashing ever drifts — trimming the body, including the tags — this is
// what catches it, because a wrong digest blanks the SPA in production.
const alert1Digest = "sha256-bhHHL3z2vDgxUt0W3dWQOrprscmda2Y5pLsLg4GF+pI="

func TestInlineScriptHashes_MatchesBrowserDigest(t *testing.T) {
	hashes := inlineScriptHashes([]byte(`<html><body><script>alert(1)</script></body></html>`))
	require.Len(t, hashes, 1)
	assert.Equal(t, alert1Digest, hashes[0])
}

func TestInlineScriptHashes_SkipsExternalAndEmpty(t *testing.T) {
	html := []byte(`
		<script type="module" src="/assets/index-abc123.js"></script>
		<script SRC="/registerSW.js"></script>
		<script></script>
		<script>alert(1)</script>
	`)
	hashes := inlineScriptHashes(html)
	assert.Equal(t, []string{alert1Digest}, hashes,
		"only the inline, non-empty script needs a hash")
}

func TestInlineScriptHashes_HashesEveryInlineScript(t *testing.T) {
	hashes := inlineScriptHashes([]byte(`<script>a()</script><script>b()</script>`))
	assert.Len(t, hashes, 2)
	assert.NotEqual(t, hashes[0], hashes[1])
}

// The real index.html bootstraps the theme inline, so the policy must carry a
// hash. Without one the script is blocked and the app renders unthemed.
func TestContentSecurityPolicy_CarriesInlineScriptHash(t *testing.T) {
	index := []byte(`<!DOCTYPE html><html><body>
	<script>
	  (function() { document.documentElement.setAttribute('data-theme', 'dark'); })();
	</script>
	<div id="app"></div>
	<script type="module" src="/assets/index.js"></script>
	</body></html>`)

	csp := contentSecurityPolicy(index)

	assert.Contains(t, csp, "script-src 'self' 'sha256-")
	assert.NotContains(t, csp, "script-src 'self';",
		"the inline bootstrap script must be hashed, not blocked")
	assert.NotContains(t, csp, "'unsafe-inline'; script-src")
}

// script-src must never fall back to 'unsafe-inline' — that would defeat the
// only part of the policy that actually mitigates XSS.
func TestContentSecurityPolicy_NeverAllowsInlineScript(t *testing.T) {
	for name, index := range map[string][]byte{
		"no assets embedded": nil,
		"with inline script": []byte(`<script>alert(1)</script>`),
	} {
		t.Run(name, func(t *testing.T) {
			csp := contentSecurityPolicy(index)
			assert.Contains(t, csp, "object-src 'none'")
			assert.Contains(t, csp, "base-uri 'self'")
			// 'unsafe-inline' is allowed for styles only.
			assert.Contains(t, csp, "style-src 'self' 'unsafe-inline'")
			assert.NotContains(t, csp, "script-src 'self' 'unsafe-inline'")
			// frame-ancestors is per-route, appended by SecurityHeaders.
			assert.NotContains(t, csp, "frame-ancestors")
		})
	}
}

// The status page stores its logo and hero as data URLs; blocking data: images
// would break every personalized status page.
func TestContentSecurityPolicy_AllowsDataURLImages(t *testing.T) {
	assert.Contains(t, contentSecurityPolicy(nil), "img-src 'self' data: blob:")
}

func TestSecurityHeaders_SetOnEveryResponse(t *testing.T) {
	h := SecurityHeaders("default-src 'self'")(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil))

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
	assert.Equal(t, "default-src 'self'; frame-ancestors 'none'",
		rec.Header().Get("Content-Security-Policy"))
}

// The dashboard and the admin API are the clickjacking targets, so they refuse
// framing outright. /api/v1/status/ is admin despite the name and must not be
// mistaken for the public status page.
func TestSecurityHeaders_RefusesFramingOnAdminSurfaces(t *testing.T) {
	h := SecurityHeaders("default-src 'self'")(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {}))

	for _, path := range []string{"/", "/dashboard", "/api/v1/containers", "/api/v1/status/components", "/statuses"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
			assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
		})
	}
}

// The public status page is documented as embeddable; locking it to
// frame-ancestors 'none' would silently break anyone iframing it.
func TestSecurityHeaders_KeepsStatusPageEmbeddable(t *testing.T) {
	h := SecurityHeaders("default-src 'self'")(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {}))

	for _, path := range []string{"/status", "/status/", "/status/api", "/status/events"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			assert.Empty(t, rec.Header().Get("X-Frame-Options"))
			assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors *")
		})
	}
}

// HSTS is the reverse proxy's job — asserting its absence keeps someone from
// re-adding it here off a forgeable X-Forwarded-Proto.
func TestSecurityHeaders_NoHSTS(t *testing.T) {
	h := SecurityHeaders("default-src 'self'")(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	h.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Strict-Transport-Security"))
}

// The status page asset route sets a much stricter policy of its own. The
// middleware runs first, so the handler has to be able to overwrite it.
func TestSecurityHeaders_HandlerCanTightenPolicy(t *testing.T) {
	strict := "default-src 'none'; style-src 'unsafe-inline'"
	h := SecurityHeaders("default-src 'self'")(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Security-Policy", strict)
			w.WriteHeader(http.StatusOK)
		}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status/assets/logo", nil))

	assert.Equal(t, strict, rec.Header().Get("Content-Security-Policy"))
}
