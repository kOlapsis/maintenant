// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelPatchApply_HostChangeDropsStoredHeaders(t *testing.T) {
	const stored = `{"Authorization":"Bearer s3cr3t"}`
	const fresh = `{"X-New":"1"}`
	cases := []struct {
		name    string
		url     *string
		headers *string
		want    string
	}{
		{"other host without headers", new("https://evil.example.net/hook"), nil, ""},
		{"other port without headers", new("https://hooks.example.com:8443/hook"), nil, ""},
		{"other path on the same host", new("https://hooks.example.com/other?x=1"), nil, stored},
		{"same host, other case", new("https://HOOKS.example.com/other"), nil, stored},
		{"other host with new headers", new("https://evil.example.net/hook"), new(fresh), fresh},
		{"no url in the patch", nil, nil, stored},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := &NotificationChannel{Type: "webhook", URL: "https://hooks.example.com/hook", Headers: stored}
			require.NoError(t, (&ChannelPatch{URL: tc.url, Headers: tc.headers}).Apply(ch))
			assert.Equal(t, tc.want, ch.Headers)
		})
	}
}
