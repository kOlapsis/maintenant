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

package agentserver

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolvePublicURL_Explicit(t *testing.T) {
	url, warnings := ResolvePublicURL(nil, PublicURLConfig{
		Explicit: "grpcs://server.example.com:8443",
	})
	assert.Equal(t, "grpcs://server.example.com:8443", url)
	assert.Empty(t, warnings)
}

func TestResolvePublicURL_ExplicitLocalWarning(t *testing.T) {
	url, warnings := ResolvePublicURL(nil, PublicURLConfig{
		Explicit: "grpcs://127.0.0.1:8443",
	})
	assert.Equal(t, "grpcs://127.0.0.1:8443", url)
	assert.Contains(t, warnings, "public_url_appears_local")
}

func TestResolvePublicURL_ExplicitLocalhost(t *testing.T) {
	_, warnings := ResolvePublicURL(nil, PublicURLConfig{
		Explicit: "localhost:8443",
	})
	assert.Contains(t, warnings, "public_url_appears_local")
}

func TestResolvePublicURL_ExplicitHTTPS(t *testing.T) {
	url, _ := ResolvePublicURL(nil, PublicURLConfig{
		Explicit: "https://server.example.com",
	})
	assert.Equal(t, "grpcs://server.example.com", url)
}

func TestResolvePublicURL_ForwardedHeaders(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://internal:8080/api/v1/agents/enrollment-tokens", nil)
	req.Header.Set("X-Forwarded-Host", "server.example.com:8443")
	req.Header.Set("X-Forwarded-Proto", "grpcs")

	url, warnings := ResolvePublicURL(req, PublicURLConfig{})
	assert.Equal(t, "grpcs://server.example.com:8443", url)
	assert.Empty(t, warnings)
}

func TestResolvePublicURL_ForwardedHeadersStandardPort(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://internal:8080/api/v1/agents/enrollment-tokens", nil)
	req.Header.Set("X-Forwarded-Host", "server.example.com:443")
	req.Header.Set("X-Forwarded-Proto", "grpcs")

	url, warnings := ResolvePublicURL(req, PublicURLConfig{})
	assert.Equal(t, "grpcs://server.example.com", url)
	assert.Empty(t, warnings)
}

func TestResolvePublicURL_ForwardedLocalWarning(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://internal:8080/api/v1/agents/enrollment-tokens", nil)
	req.Header.Set("X-Forwarded-Host", "192.168.1.10:8443")

	_, warnings := ResolvePublicURL(req, PublicURLConfig{})
	assert.Contains(t, warnings, "public_url_appears_local")
}

func TestResolvePublicURL_HostHeader(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://server.example.com:8443/api/v1/agents/enrollment-tokens", nil)
	req.Host = "server.example.com:8443"

	url, warnings := ResolvePublicURL(req, PublicURLConfig{})
	assert.Equal(t, "grpcs://server.example.com:8443", url)
	assert.Empty(t, warnings)
}

func TestResolvePublicURL_FallbackListen(t *testing.T) {
	url, warnings := ResolvePublicURL(nil, PublicURLConfig{
		ListenAddr: "127.0.0.1:8443",
	})
	assert.Equal(t, "grpcs://127.0.0.1:8443", url)
	assert.Contains(t, warnings, "public_url_appears_local")
}

func TestResolvePublicURL_RFC1918(t *testing.T) {
	for _, ip := range []string{"10.0.0.1", "172.16.0.1", "192.168.1.1"} {
		_, warnings := ResolvePublicURL(nil, PublicURLConfig{Explicit: ip + ":8443"})
		assert.Contains(t, warnings, "public_url_appears_local", "expected local warning for %s", ip)
	}
}

func TestResolvePublicURL_ExplicitPriorityOverHeaders(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("X-Forwarded-Host", "header-host.example.com")

	url, _ := ResolvePublicURL(req, PublicURLConfig{
		Explicit: "grpcs://explicit.example.com:8443",
	})
	assert.Equal(t, "grpcs://explicit.example.com:8443", url)
}
