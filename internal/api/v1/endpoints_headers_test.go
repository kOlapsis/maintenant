// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpointList_StillReturnsProbeHeaders(t *testing.T) {
	h := newEndpointHandler(t)

	rec := sendEndpoint(h.HandleCreateEndpoint, http.MethodPost, "/api/v1/endpoints", "",
		`{"name":"api","target":"https://example.com/health","endpoint_type":"http","headers":{"Authorization":"Bearer s3cr3t-value"}}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	rec = sendEndpoint(h.HandleListEndpoints, http.MethodGet, "/api/v1/endpoints", "", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "Bearer s3cr3t-value")
}
