package app

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTTPServer_AcceptsAnyIPLiteralHost(t *testing.T) {
	a, _ := newTestApp(t, func(c *Config) {
		c.Addr = "0.0.0.0:0"
		c.BaseURL = "https://now.example.com"
	})

	for _, h := range []string{"192.168.1.10", "192.168.1.10:8080", "[fe80::1]:8080", "[fe80::1]", "[2001:db8::1]:8080"} {
		rec := hit(a, http.MethodGet, "/api/v1/health", host(h))
		assert.Equal(t, http.StatusOK, rec.Code, "%s: %s", h, rec.Body.String())
	}

	rec := hit(a, http.MethodGet, "/api/v1/health", host("nas.lan:8080"))
	assert.Equal(t, http.StatusMisdirectedRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "misdirected_request", errorCode(t, rec))
	assert.Contains(t, rec.Body.String(), "MAINTENANT_ALLOWED_HOSTS")
	assert.Contains(t, rec.Body.String(), "MAINTENANT_BASE_URL")

	rec = hit(a, http.MethodGet, "/api/v1/health", host("now.example.com"))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
