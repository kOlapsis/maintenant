// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, C7: what a label may ask of a monitor.

func parseOne(t *testing.T, labels map[string]string) (*ParsedEndpoint, string) {
	t.Helper()
	var logs strings.Builder
	parsed, errs := ParseEndpointLabels(labels, slog.New(slog.NewTextHandler(&logs, nil)))
	require.Empty(t, errs)
	require.Len(t, parsed, 1)
	return parsed[0], logs.String()
}

// US2 scenario 1, FR-010: only GET and HEAD come from a label; anything else is ignored with a warning naming the value.
func TestParseEndpointLabels_OnlyGetAndHeadAreAccepted(t *testing.T) {
	for _, method := range []string{"DELETE", "POST", "PUT", "PATCH", "OPTIONS", "delete", "TRACE"} {
		t.Run(method, func(t *testing.T) {
			ep, logs := parseOne(t, map[string]string{
				"maintenant.endpoint.http":        "http://web:8080/health",
				"maintenant.endpoint.http.method": method,
			})
			assert.Equal(t, "GET", ep.Config.Method)
			assert.Contains(t, logs, "WARN")
			assert.Contains(t, logs, strings.ToUpper(method), "the warning names the ignored method")
		})
	}

	for label, want := range map[string]string{"HEAD": "HEAD", "head": "HEAD", "GET": "GET", "get": "GET"} {
		ep, _ := parseOne(t, map[string]string{
			"maintenant.endpoint.http":        "http://web:8080/health",
			"maintenant.endpoint.http.method": label,
		})
		assert.Equal(t, want, ep.Config.Method, label)
	}
}

// FR-010 on an indexed endpoint: the rule holds whatever the label form.
func TestParseEndpointLabels_IndexedMethodIsBoundedToo(t *testing.T) {
	ep, logs := parseOne(t, map[string]string{
		"maintenant.endpoint.0.http":        "http://api:3000/ready",
		"maintenant.endpoint.0.http.method": "POST",
	})
	assert.Equal(t, "GET", ep.Config.Method)
	assert.Contains(t, logs, "POST")
}

// US2 scenario 2, FR-011: Authorization, Cookie and Proxy-Authorization are dropped whatever their case; the rest stays.
func TestParseEndpointLabels_SensitiveHeadersAreIgnored(t *testing.T) {
	cases := map[string]struct {
		raw     string
		kept    map[string]string
		dropped []string
	}{
		"json authorization": {
			raw:     `{"Authorization":"Bearer x","X-Trace":"1"}`,
			kept:    map[string]string{"X-Trace": "1"},
			dropped: []string{"Authorization"},
		},
		"json lower-case authorization": {
			raw:     `{"authorization":"Bearer x","X-Request-Id":"abc"}`,
			kept:    map[string]string{"X-Request-Id": "abc"},
			dropped: []string{"authorization"},
		},
		"json cookie and proxy": {
			raw:     `{"Cookie":"session=1","PROXY-AUTHORIZATION":"Basic x","Accept":"application/json"}`,
			kept:    map[string]string{"Accept": "application/json"},
			dropped: []string{"Cookie", "PROXY-AUTHORIZATION"},
		},
		"key=value form": {
			raw:     "cookie=session=1,X-Trace=2",
			kept:    map[string]string{"X-Trace": "2"},
			dropped: []string{"cookie"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ep, logs := parseOne(t, map[string]string{
				"maintenant.endpoint.http":         "http://web:8080/health",
				"maintenant.endpoint.http.headers": tc.raw,
			})
			assert.Equal(t, tc.kept, ep.Config.Headers)
			for _, h := range tc.dropped {
				assert.NotContains(t, ep.Config.Headers, h)
				assert.Contains(t, logs, h, "the warning names the ignored header")
			}
			assert.Contains(t, logs, "WARN")
		})
	}
}

// FR-011: a label made only of sensitive headers leaves no header at all.
func TestParseEndpointLabels_OnlySensitiveHeadersLeavesNone(t *testing.T) {
	ep, _ := parseOne(t, map[string]string{
		"maintenant.endpoint.http":         "http://web:8080/health",
		"maintenant.endpoint.http.headers": `{"Authorization":"Bearer x"}`,
	})
	assert.Empty(t, ep.Config.Headers)
}

// US2 scenario 1 and 2, FR-010, FR-011: the warning written by the server names the container.
func TestService_SyncEndpoints_WarningsNameTheContainer(t *testing.T) {
	var logs strings.Builder
	store := newMemStore()
	svc := NewService(Deps{
		Store:  store,
		Engine: noopEngine(),
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	ctx := context.Background()

	svc.SyncEndpoints(ctx, "shop-web", "ctr-1", map[string]string{
		"maintenant.endpoint.http":         "http://web:8080/health",
		"maintenant.endpoint.http.method":  "DELETE",
		"maintenant.endpoint.http.headers": `{"Authorization":"Bearer x","X-Trace":"1"}`,
	})

	eps, err := store.ListEndpointsByExternalID(ctx, uid.LocalAgent, "ctr-1")
	require.NoError(t, err)
	require.Len(t, eps, 1)
	assert.Equal(t, "GET", eps[0].Config.Method)
	assert.Equal(t, map[string]string{"X-Trace": "1"}, eps[0].Config.Headers)

	var methodLine, headerLine string
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "WARN") && strings.Contains(line, "DELETE") {
			methodLine = line
		}
		if strings.Contains(line, "WARN") && strings.Contains(line, "Authorization") {
			headerLine = line
		}
	}
	require.NotEmpty(t, methodLine, "a warning names the ignored method: %s", logs.String())
	assert.Contains(t, methodLine, "shop-web", "the method warning names the container")
	require.NotEmpty(t, headerLine, "a warning names the ignored header: %s", logs.String())
	assert.Contains(t, headerLine, "shop-web", "the header warning names the container")
}

// FR-014: an agent's container goes through the same parser, with the same bounds and the same naming.
func TestService_SyncAgentEndpoints_AppliesTheSameBounds(t *testing.T) {
	var logs strings.Builder
	store := newMemStore()
	svc := NewService(Deps{
		Store:  store,
		Engine: noopEngine(),
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	ctx := context.Background()

	svc.SyncAgentEndpoints(ctx, "agent-1", "remote-api", "ctr-9", map[string]string{
		"maintenant.endpoint.http":         "http://api:3000/ready",
		"maintenant.endpoint.http.method":  "PUT",
		"maintenant.endpoint.http.headers": `{"Cookie":"s=1","X-Env":"prod"}`,
	})

	eps, err := store.ListEndpointsByExternalID(ctx, "agent-1", "ctr-9")
	require.NoError(t, err)
	require.Len(t, eps, 1)
	assert.Equal(t, "GET", eps[0].Config.Method)
	assert.Equal(t, map[string]string{"X-Env": "prod"}, eps[0].Config.Headers)
	assert.Equal(t, SourceLabel, eps[0].Source)
	assert.Contains(t, logs.String(), "remote-api")
	assert.Contains(t, logs.String(), "PUT")
	assert.Contains(t, logs.String(), "Cookie")
}
