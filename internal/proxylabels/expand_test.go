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

package proxylabels

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func synthesized(in, out map[string]string) map[string]string {
	added := make(map[string]string)
	for k, v := range out {
		if orig, ok := in[k]; !ok || orig != v {
			added[k] = v
		}
	}
	return added
}

func TestExpand(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   map[string]string
	}{
		{
			name:   "no proxy labels",
			labels: map[string]string{"com.docker.compose.service": "api"},
			want:   map[string]string{},
		},
		{
			name: "traefik host with tls certresolver",
			labels: map[string]string{
				"traefik.enable":                                     "true",
				"traefik.http.routers.app.rule":                      "Host(`app.example.com`)",
				"traefik.http.routers.app.tls.certresolver":          "le",
				"traefik.http.services.app.loadbalancer.server.port": "8080",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "app.example.com",
			},
		},
		{
			name: "traefik tls=true",
			labels: map[string]string{
				"traefik.http.routers.app.rule": "Host(`app.example.com`)",
				"traefik.http.routers.app.tls":  "true",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "app.example.com",
			},
		},
		{
			name: "traefik tls=false stays http",
			labels: map[string]string{
				"traefik.http.routers.app.rule": "Host(`app.example.com`)",
				"traefik.http.routers.app.tls":  "false",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik entrypoints websecure",
			labels: map[string]string{
				"traefik.http.routers.app.rule":        "Host(\"app.example.com\")",
				"traefik.http.routers.app.entrypoints": "web, websecure",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "app.example.com",
			},
		},
		{
			name: "traefik entrypoint web only is http",
			labels: map[string]string{
				"traefik.http.routers.app.rule":        "Host(`app.example.com`)",
				"traefik.http.routers.app.entrypoints": "web",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik multiple hosts keep the first in order",
			labels: map[string]string{
				"traefik.http.routers.app.rule": "Host(`b.example.com`, `a.example.com`) || Host('c.example.com')",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://a.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik root route wins over a path route",
			labels: map[string]string{
				"traefik.http.routers.app.rule":                       "Host(`app.example.com`)",
				"traefik.http.routers.app.tls":                        "true",
				"traefik.http.routers.app-ws.rule":                    "Host(`app.example.com`) && PathPrefix(`/ws`)",
				"traefik.http.routers.app-ws.tls":                     "true",
				"traefik.http.middlewares.strip.stripprefix.prefixes": "/ws",
				"traefik.http.routers.app-ws.middlewares":             "strip",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "app.example.com",
			},
		},
		{
			name: "traefik shortest path wins",
			labels: map[string]string{
				"traefik.http.routers.api.rule": "Host(`example.com`) && PathPrefix(`/api/v1/deep`)",
				"traefik.http.routers.app.rule": "Host(`example.com`) && PathPrefix(`/api`)",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://example.com/api",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik https wins over http",
			labels: map[string]string{
				"traefik.http.routers.plain.rule":  "Host(`z.example.com`)",
				"traefik.http.routers.secure.rule": "Host(`a.example.com`)",
				"traefik.http.routers.secure.tls":  "true",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://a.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "a.example.com",
			},
		},
		{
			name: "traefik basicauth middleware declared on the container is skipped",
			labels: map[string]string{
				"traefik.http.routers.private.rule":              "Host(`app.example.com`)",
				"traefik.http.routers.private.middlewares":       "guard@docker",
				"traefik.http.middlewares.guard.basicauth.users": "admin:$2y$05$abc",
				"traefik.http.routers.public.rule":               "Host(`app.example.com`) && PathPrefix(`/status`)",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://app.example.com/status",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik middleware named auth is skipped",
			labels: map[string]string{
				"traefik.http.routers.private.rule":        "Host(`app.example.com`)",
				"traefik.http.routers.private.middlewares": "authelia@file, compress",
				"traefik.http.routers.public.rule":         "Host(`app.example.com`) && PathPrefix(`/health`)",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://app.example.com/health",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik forwardauth on every route keeps the best one anyway",
			labels: map[string]string{
				"traefik.http.routers.app.rule":                    "Host(`app.example.com`)",
				"traefik.http.routers.app.middlewares":             "sso",
				"traefik.http.middlewares.sso.forwardauth.address": "http://authelia:9091/api/verify",
				"traefik.http.routers.app-api.rule":                "Host(`app.example.com`) && PathPrefix(`/api`)",
				"traefik.http.routers.app-api.middlewares":         "sso",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik non auth middleware does not disqualify a route",
			labels: map[string]string{
				"traefik.http.routers.app.rule":        "Host(`app.example.com`)",
				"traefik.http.routers.app.middlewares": "compress,secure-headers@file",
				"traefik.http.routers.api.rule":        "Host(`app.example.com`) && PathPrefix(`/api`)",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik host with single path prefix",
			labels: map[string]string{
				"traefik.http.routers.api.rule": "Host(`example.com`) && PathPrefix(`/api`)",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://example.com/api",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik host with single path",
			labels: map[string]string{
				"traefik.http.routers.api.rule": "Host(`example.com`) && Path(`/health`)",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://example.com/health",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik two paths are not appended",
			labels: map[string]string{
				"traefik.http.routers.api.rule": "Host(`example.com`) && (PathPrefix(`/a`) || PathPrefix(`/b`))",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "traefik HostRegexp and HostSNI ignored",
			labels: map[string]string{
				"traefik.http.routers.a.rule": "HostRegexp(`^.+\\.example\\.com$`)",
				"traefik.tcp.routers.b.rule":  "HostSNI(`db.example.com`)",
			},
			want: map[string]string{},
		},
		{
			name: "traefik tcp and udp routers ignored",
			labels: map[string]string{
				"traefik.tcp.routers.db.rule":  "Host(`db.example.com`)",
				"traefik.udp.routers.dns.rule": "Host(`dns.example.com`)",
			},
			want: map[string]string{},
		},
		{
			name: "traefik.enable=false",
			labels: map[string]string{
				"traefik.enable":                "false",
				"traefik.http.routers.app.rule": "Host(`app.example.com`)",
			},
			want: map[string]string{},
		},
		{
			name: "caddy single site",
			labels: map[string]string{
				"caddy":               "app.example.com",
				"caddy.reverse_proxy": "{{upstreams 8080}}",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "app.example.com",
			},
		},
		{
			name: "caddy multiple sites, spaces and commas, numbered key",
			labels: map[string]string{
				"caddy":   "b.example.com, a.example.com",
				"caddy_1": "c.example.com d.example.com:8443",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://a.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "a.example.com",
			},
		},
		{
			name: "caddy basicauth site is skipped",
			labels: map[string]string{
				"caddy":               "a.example.com",
				"caddy.basicauth.bob": "$2a$14$hash",
				"caddy_1":             "z.example.com",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://z.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "z.example.com",
			},
		},
		{
			name: "caddy forward_auth site is skipped",
			labels: map[string]string{
				"caddy":                  "a.example.com",
				"caddy.forward_auth":     "authelia:9091",
				"caddy.forward_auth.uri": "/api/verify?rd=https://auth.example.com",
				"caddy_1":                "z.example.com",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://z.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "z.example.com",
			},
		},
		{
			name: "caddy http scheme, port 80, explicit 443: https wins",
			labels: map[string]string{
				"caddy":   "http://plain.example.com",
				"caddy_0": "eighty.example.com:80",
				"caddy_1": "secure.example.com:443",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://secure.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "secure.example.com",
			},
		},
		{
			name: "caddy http only keeps the first in order",
			labels: map[string]string{
				"caddy":   "http://z.example.com",
				"caddy_1": "http://a.example.com",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://a.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
			},
		},
		{
			name: "caddy wildcards, placeholders and bare ports skipped",
			labels: map[string]string{
				"caddy":   "*.example.com :80 {$DOMAIN}",
				"caddy_1": ":443",
			},
			want: map[string]string{},
		},
		{
			name: "caddy subkeys are not sites",
			labels: map[string]string{
				"caddy.reverse_proxy": "app.example.com",
				"caddy_1.tls":         "internal",
			},
			want: map[string]string{},
		},
		{
			name: "caddy tls internal disables verification and certificate",
			labels: map[string]string{
				"caddy":     "internal.lan",
				"caddy.tls": "internal",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://internal.lan",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.endpoint.0.http.tls-verify":      "false",
			},
		},
		{
			name: "caddy tls internal not selected leaves the public site alone",
			labels: map[string]string{
				"caddy":     "z-internal.lan",
				"caddy.tls": "internal",
				"caddy_1":   "public.example.com",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://public.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "public.example.com",
			},
		},
		{
			name: "traefik and caddy dedupe",
			labels: map[string]string{
				"traefik.http.routers.app.rule": "Host(`app.example.com`)",
				"traefik.http.routers.app.tls":  "true",
				"caddy":                         "app.example.com",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "app.example.com",
			},
		},
		{
			name: "global expected-status wins",
			labels: map[string]string{
				"caddy": "app.example.com",
				"maintenant.endpoint.http.expected-status": "200",
				"maintenant.endpoint.interval":             "1m",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":  "https://app.example.com",
				"maintenant.tls.certificates": "app.example.com",
			},
		},
		{
			name: "existing certificates label is merged",
			labels: map[string]string{
				"caddy":                       "app.example.com",
				"maintenant.tls.certificates": "mail.example.com, app.example.com",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://app.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "mail.example.com,app.example.com",
			},
		},
		{
			name: "explicit simple http target wins",
			labels: map[string]string{
				"caddy":                    "app.example.com",
				"maintenant.endpoint.http": "http://app:8080/health",
			},
			want: map[string]string{},
		},
		{
			name: "explicit simple tcp target wins",
			labels: map[string]string{
				"caddy":                   "app.example.com",
				"maintenant.endpoint.tcp": "app:5432",
			},
			want: map[string]string{},
		},
		{
			name: "explicit indexed target wins",
			labels: map[string]string{
				"traefik.http.routers.app.rule": "Host(`app.example.com`)",
				"maintenant.endpoint.3.tcp":     "app:6379",
			},
			want: map[string]string{},
		},
		{
			name: "ignored container",
			labels: map[string]string{
				"caddy":             "app.example.com",
				"maintenant.ignore": "true",
			},
			want: map[string]string{},
		},
		{
			name: "per-container opt-out",
			labels: map[string]string{
				"caddy":                   "app.example.com",
				"maintenant.proxy-labels": "false",
			},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := Expand(tt.labels)
			assert.Equal(t, tt.want, synthesized(tt.labels, out))
			for k, v := range tt.labels {
				if !strings.HasPrefix(k, "maintenant.tls.certificates") {
					assert.Equal(t, v, out[k], "original label %s must be kept", k)
				}
			}
		})
	}
}

func TestExpand_DoesNotMutateInput(t *testing.T) {
	in := map[string]string{"caddy": "app.example.com"}
	_ = Expand(in)
	assert.Equal(t, map[string]string{"caddy": "app.example.com"}, in)
}
