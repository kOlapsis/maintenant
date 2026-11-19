// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPChannels_UpdatingTheHostDropsStoredHeaders(t *testing.T) {
	const headers = `{"Authorization":"Bearer s3cr3t"}`
	cases := []struct {
		name  string
		url   string
		heads *string
		want  string
	}{
		{"url on another host", "https://evil.example.net/hook", nil, ""},
		{"url on the same host", "https://hooks.example.com/other", nil, headers},
		{"url on another host with headers", "https://evil.example.net/hook", str(`{"X-New":"1"}`), `{"X-New":"1"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, cs := buildSecretChannelServices(t)
			id, _, _ := createOK(t, svc, createChannelInput{Name: "ops", URL: "https://hooks.example.com/x", Headers: headers})

			result, _, err := updateChannelHandler(svc)(context.Background(), nil, updateChannelInput{ID: id, URL: str(tc.url), Headers: tc.heads})

			require.NoError(t, err)
			require.False(t, result.IsError, textFromContent(t, result.Content))
			assert.Equal(t, tc.want, cs.channels[id].Headers)
		})
	}
}
