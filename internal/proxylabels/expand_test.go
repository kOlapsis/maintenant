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
			name: "traefik multiple hosts and || combination",
			labels: map[string]string{
				"traefik.http.routers.app.rule": "Host(`b.example.com`, `a.example.com`) || Host('c.example.com')",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://a.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.endpoint.1.http":                 "http://b.example.com",
				"maintenant.endpoint.1.http.expected-status": "2xx,3xx",
				"maintenant.endpoint.2.http":                 "http://c.example.com",
				"maintenant.endpoint.2.http.expected-status": "2xx,3xx",
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
				"caddy":   "a.example.com, b.example.com",
				"caddy_1": "c.example.com d.example.com:8443",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://a.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.endpoint.1.http":                 "https://b.example.com",
				"maintenant.endpoint.1.http.expected-status": "2xx,3xx",
				"maintenant.endpoint.2.http":                 "https://c.example.com",
				"maintenant.endpoint.2.http.expected-status": "2xx,3xx",
				"maintenant.endpoint.3.http":                 "https://d.example.com:8443",
				"maintenant.endpoint.3.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "a.example.com,b.example.com,c.example.com,d.example.com:8443",
			},
		},
		{
			name: "caddy http scheme, port 80, explicit 443",
			labels: map[string]string{
				"caddy":   "http://plain.example.com",
				"caddy_0": "eighty.example.com:80",
				"caddy_1": "secure.example.com:443",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "http://eighty.example.com",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.endpoint.1.http":                 "http://plain.example.com",
				"maintenant.endpoint.1.http.expected-status": "2xx,3xx",
				"maintenant.endpoint.2.http":                 "https://secure.example.com",
				"maintenant.endpoint.2.http.expected-status": "2xx,3xx",
				"maintenant.tls.certificates":                "secure.example.com",
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
				"caddy_1":   "public.example.com",
			},
			want: map[string]string{
				"maintenant.endpoint.0.http":                 "https://internal.lan",
				"maintenant.endpoint.0.http.expected-status": "2xx,3xx",
				"maintenant.endpoint.0.http.tls-verify":      "false",
				"maintenant.endpoint.1.http":                 "https://public.example.com",
				"maintenant.endpoint.1.http.expected-status": "2xx,3xx",
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
