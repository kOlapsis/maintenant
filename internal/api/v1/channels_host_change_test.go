// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannels_UpdatingTheHostDropsStoredHeaders(t *testing.T) {
	const headers = `{"Authorization":"Bearer s3cr3t","X-Trace":"1"}`
	cases := []struct {
		name string
		body string
		want string
	}{
		{"url on another host", `{"url":"https://evil.example.net/hook"}`, ""},
		{"url on the same host", `{"url":"https://hooks.example.com/other"}`, headers},
		{"url on another host with headers", `{"url":"https://evil.example.net/hook","headers":"{\"X-New\":\"1\"}"}`, `{"X-New":"1"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newChannelHarness(t)
			id, _ := c.create(t, webhookWithSecret)

			rec := c.do(http.MethodPut, "/api/v1/channels/"+id, tc.body)

			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, tc.want, c.store.row(t, id).Headers)
		})
	}
}
