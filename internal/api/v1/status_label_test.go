// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Acceptance tests of 043-public-surfaces-hardening, US1 scenarios 5 and 6, FR-002, FR-003.

func explicitComponentBody(t *testing.T, name string, monitors []status.MonitorRef) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"display_name":     name,
		"composition_mode": "explicit",
		"monitors":         monitors,
	})
	require.NoError(t, err)
	return string(raw)
}

func monitorsBody(t *testing.T, monitors []status.MonitorRef) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"monitors": monitors})
	require.NoError(t, err)
	return string(raw)
}

func adminMonitors(t *testing.T, admin http.Handler, componentID string) map[string]status.MonitorRef {
	t.Helper()
	rec := serve(t, admin, http.MethodGet, "/api/v1/status/components", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var comps []status.Component
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comps))
	for _, c := range comps {
		if c.ID != componentID {
			continue
		}
		byID := map[string]status.MonitorRef{}
		for _, m := range c.Monitors {
			require.NotEmpty(t, m.ID, "the admin surface keeps the monitor id")
			byID[m.ID] = m
		}
		return byID
	}
	t.Fatalf("component %s not listed in %s", componentID, rec.Body.String())
	return nil
}

func TestStatusAdmin_StoresAndReturnsMonitorLabels(t *testing.T) {
	admin := statusAdminRouter(t)
	endpointID, heartbeatID := uid.New(), uid.New()

	rec := serve(t, admin, http.MethodPost, "/api/v1/status/components", explicitComponentBody(t, "API", []status.MonitorRef{
		{Type: "endpoint", ID: endpointID, Label: "  API publique  "},
		{Type: "heartbeat", ID: heartbeatID},
	}))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var comp status.Component
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comp))

	got := adminMonitors(t, admin, comp.ID)
	require.Len(t, got, 2)
	assert.Equal(t, "API publique", got[endpointID].Label, "the label is stored trimmed")
	assert.Equal(t, "endpoint", got[endpointID].Type)
	assert.Empty(t, got[heartbeatID].Label)

	rec = serve(t, admin, http.MethodPut, "/api/v1/status/components/"+comp.ID, monitorsBody(t, []status.MonitorRef{
		{Type: "endpoint", ID: endpointID, Label: "Base de données"},
		{Type: "heartbeat", ID: heartbeatID, Label: "Cron nocturne"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got = adminMonitors(t, admin, comp.ID)
	assert.Equal(t, "Base de données", got[endpointID].Label, "a kept monitor takes its new label")
	assert.Equal(t, "Cron nocturne", got[heartbeatID].Label, "a kept monitor can gain a label")

	rec = serve(t, admin, http.MethodPut, "/api/v1/status/components/"+comp.ID, monitorsBody(t, []status.MonitorRef{
		{Type: "endpoint", ID: endpointID, Label: "   "},
		{Type: "heartbeat", ID: heartbeatID, Label: "Cron nocturne"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got = adminMonitors(t, admin, comp.ID)
	assert.Empty(t, got[endpointID].Label, "blank clears the label")
	assert.Equal(t, "Cron nocturne", got[heartbeatID].Label)

	rec = serve(t, admin, http.MethodGet, "/api/v1/status/components", "")
	assert.Contains(t, rec.Body.String(), endpointID, "the admin list keeps the ids")
	assert.Contains(t, rec.Body.String(), heartbeatID)
}

func TestStatusAdmin_RefusesLabelsOutsideTheRule(t *testing.T) {
	admin := statusAdminRouter(t)
	monitorID := uid.New()
	bad := map[string]string{
		"81 characters":   strings.Repeat("a", 81),
		"81 runes":        strings.Repeat("é", 81),
		"newline":         "API\npublique",
		"carriage return": "API\rpublique",
		"tab":             "API\tpublique",
		"control":         "API\x07publique",
	}

	for name, label := range bad {
		t.Run("create "+name, func(t *testing.T) {
			rec := serve(t, admin, http.MethodPost, "/api/v1/status/components", explicitComponentBody(t, "API", []status.MonitorRef{
				{Type: "endpoint", ID: monitorID, Label: label},
			}))
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "validation", body.Error.Code)
		})
	}
	rec := serve(t, admin, http.MethodGet, "/api/v1/status/components", "")
	var listed []status.Component
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	assert.Empty(t, listed, "a refused component is not written")

	rec = serve(t, admin, http.MethodPost, "/api/v1/status/components", explicitComponentBody(t, "API", []status.MonitorRef{
		{Type: "endpoint", ID: monitorID, Label: strings.Repeat("é", 80)},
	}))
	require.Equal(t, http.StatusCreated, rec.Code, "80 runes are the limit, not 80 bytes: %s", rec.Body.String())
	var comp status.Component
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comp))
	rec = serve(t, admin, http.MethodPut, "/api/v1/status/components/"+comp.ID, monitorsBody(t, []status.MonitorRef{
		{Type: "endpoint", ID: monitorID, Label: "ok"},
	}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	for name, label := range bad {
		t.Run("update "+name, func(t *testing.T) {
			rec := serve(t, admin, http.MethodPut, "/api/v1/status/components/"+comp.ID, monitorsBody(t, []status.MonitorRef{
				{Type: "endpoint", ID: monitorID, Label: label},
			}))
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "validation", body.Error.Code)
			assert.Equal(t, "ok", adminMonitors(t, admin, comp.ID)[monitorID].Label, "a refused update leaves the label in place")
		})
	}
}
