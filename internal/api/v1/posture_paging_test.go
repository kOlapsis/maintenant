// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
)

func TestHandleListContainerPostures_CapsTheLimit(t *testing.T) {
	store := &agentEnrichStore{containers: []*container.Container{
		{ID: "1", Name: "a"}, {ID: "2", Name: "b"}, {ID: "3", Name: "c"},
	}}
	svc := container.NewService(container.Deps{Store: store, Logger: slog.Default()})
	h := NewPostureHandler(stubPostureScorer{}, svc, nil, nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/security/posture/containers?limit=9223372036854775807&offset=1", nil)
	require.NotPanics(t, func() { h.HandleListContainerPostures(w, req) })

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Containers []json.RawMessage `json:"containers"`
		Total      int               `json:"total"`
		Limit      int               `json:"limit"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body.Containers, 2)
	assert.Equal(t, 3, body.Total)
	assert.Equal(t, 500, body.Limit)
}
