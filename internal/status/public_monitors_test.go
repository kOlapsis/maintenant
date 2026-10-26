// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
)

// Acceptance tests of 043-public-surfaces-hardening, US1 (C1).

var publicMonitorKeys = map[string]bool{"type": true, "status": true, "label": true}

func monitorsOf(t *testing.T, raw json.RawMessage) []map[string]any {
	t.Helper()
	var monitors []map[string]any
	require.NoError(t, json.Unmarshal(raw, &monitors))
	return monitors
}

func assertPublicShape(t *testing.T, monitors []map[string]any) {
	t.Helper()
	for i, m := range monitors {
		for key := range m {
			assert.True(t, publicMonitorKeys[key], "monitor %d carries the key %q, only type, status and label are public", i, key)
		}
		assert.NotEmpty(t, m["type"], "monitor %d has no type", i)
	}
}

// FR-001 / SC-001: the public form of a monitor is its type, its status and its label.
func TestPublicMonitors_KeepTypeStatusAndLabelOnly(t *testing.T) {
	refs := []MonitorRef{
		{Type: "endpoint", ID: "0195-endpoint", Name: "http://api.internal:3000/health", Label: "API publique", Status: StatusDegraded},
		{Type: "heartbeat", ID: "4f1e-ping-token", Name: "nightly backup", Status: StatusOperational},
		{Type: "certificate", ID: "0195-cert", Name: "db.internal:5432"},
	}

	got := PublicMonitors(refs)

	require.Equal(t, []PublicMonitor{
		{Type: "endpoint", Status: StatusDegraded, Label: "API publique"},
		{Type: "heartbeat", Status: StatusOperational},
		{Type: "certificate"},
	}, got)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, `[{"type":"endpoint","status":"degraded","label":"API publique"},{"type":"heartbeat","status":"operational"},{"type":"certificate"}]`, string(raw))
	for _, secret := range []string{"0195-endpoint", "api.internal", "4f1e-ping-token", "nightly backup", "0195-cert", "db.internal"} {
		assert.NotContains(t, string(raw), secret)
	}
}

// FR-002: a public label is plain text, trimmed, 80 runes at most, without a control character.
func TestValidateMonitorLabel(t *testing.T) {
	eighty := strings.Repeat("a", 80)
	accents := strings.Repeat("é", 80)
	accepted := map[string]string{
		"":                 "",
		"   ":              "",
		" API publique ":   "API publique",
		"Base de données":  "Base de données",
		eighty:             eighty,
		" " + eighty + " ": eighty,
		accents:            accents,
	}
	for in, want := range accepted {
		got, err := ValidateMonitorLabel(in)
		require.NoError(t, err, "%q", in)
		assert.Equal(t, want, got, "%q", in)
	}

	refused := []string{
		strings.Repeat("a", 81),
		strings.Repeat("é", 81),
		"API\npublique",
		"API\rpublique",
		"API\tpublique",
		"API\x07publique",
		"API\x00publique",
		"API\u0085publique",
	}
	for _, in := range refused {
		_, err := ValidateMonitorLabel(in)
		assert.Error(t, err, "%q must be refused", in)
	}
}

func publicSurfaceService(t *testing.T, components []Component, public, admin func(string, any)) *Service {
	t.Helper()
	return NewService(Deps{
		Components: &mockComponentStore{visibleComponents: components},
		Logger:     discardLogger(),
		MonitorStatus: func(_ context.Context, monitorType, _ string) string {
			if monitorType == "heartbeat" {
				return StatusMajorOutage
			}
			return StatusOperational
		},
		MonitorPopulation: func(_ context.Context, monitorType string) []MonitorRef {
			switch monitorType {
			case "endpoint":
				return []MonitorRef{
					{Type: "endpoint", ID: "0195-endpoint", Name: "http://api.internal:3000/health"},
					{Type: "endpoint", ID: "0195-other", Name: "https://db.internal:5432/ready"},
				}
			case "certificate":
				return []MonitorRef{{Type: "certificate", ID: "0195-cert", Name: "db.internal:5432"}}
			}
			return nil
		},
		MonitorName: func(_ context.Context, _, _ string) string {
			return "http://api.internal:3000/health"
		},
		PublicBroadcaster: public,
		AdminBroadcaster:  admin,
	})
}

var explicitComponent = Component{
	ID: "c-explicit", DisplayName: "API", CompositionMode: CompositionExplicit, Visible: true,
	Monitors: []MonitorRef{
		{Type: "endpoint", ID: "0195-endpoint", Label: "API publique"},
		{Type: "heartbeat", ID: "4f1e-ping-token"},
		{Type: "certificate", ID: "0195-cert"},
	},
}

var matchAllComponent = Component{
	ID: "c-all", DisplayName: "Every endpoint", CompositionMode: CompositionMatchAll, MatchAllType: "endpoint", Visible: true,
}

var secrets = []string{"0195-endpoint", "0195-other", "0195-cert", "4f1e-ping-token", "api.internal", "db.internal", `"id"`, `"name"`}

// US1 scenarios 1 and 3, FR-001, SC-001: /status/api describes a monitor by type, status and label,
// in explicit mode as in match-all mode.
func TestStatusAPI_DescribesMonitorsByTypeStatusAndLabelOnly(t *testing.T) {
	svc := publicSurfaceService(t, []Component{explicitComponent, matchAllComponent}, nil, nil)
	h := NewHandler(svc, nil, discardLogger(), nil, "https://status.example.com")

	rec := httptest.NewRecorder()
	h.HandleStatusAPI(rec, httptest.NewRequest(http.MethodGet, "/status/api", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Components []struct {
			ID       string          `json:"id"`
			Monitors json.RawMessage `json:"monitors"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Components, 2)

	byID := map[string][]map[string]any{}
	for _, c := range resp.Components {
		byID[c.ID] = monitorsOf(t, c.Monitors)
		assertPublicShape(t, byID[c.ID])
	}

	explicit := byID["c-explicit"]
	require.Len(t, explicit, 3)
	assert.Equal(t, map[string]any{"type": "endpoint", "status": StatusOperational, "label": "API publique"}, explicit[0])
	assert.Equal(t, map[string]any{"type": "heartbeat", "status": StatusMajorOutage}, explicit[1], "no label means no label key, never an id")
	assert.Equal(t, map[string]any{"type": "certificate", "status": StatusOperational}, explicit[2])

	all := byID["c-all"]
	require.Len(t, all, 2)
	for _, m := range all {
		assert.Equal(t, map[string]any{"type": "endpoint", "status": StatusOperational}, m, "a match-all monitor is its type and its status only")
	}

	assert.Equal(t, 1, strings.Count(rec.Body.String(), "API publique"))
	assert.Contains(t, rec.Body.String(), `"id":"c-explicit"`, "the component id itself stays, only the monitors are reduced")
	body := strings.ReplaceAll(strings.ReplaceAll(rec.Body.String(), `"id":"c-explicit"`, ""), `"id":"c-all"`, "")
	body = strings.ReplaceAll(strings.ReplaceAll(body, `"name":"API"`, ""), `"name":"Every endpoint"`, "")
	for _, secret := range secrets {
		assert.NotContains(t, body, secret)
	}
}

// US1 scenario 2, FR-001, FR-003: status.component_changed is reduced on the public bus and complete on the admin bus.
func TestBroadcastComponentChange_PublicPayloadHidesIDsAdminKeepsThem(t *testing.T) {
	for name, comp := range map[string]Component{"explicit": explicitComponent, "match-all": matchAllComponent} {
		t.Run(name, func(t *testing.T) {
			var public, admin []json.RawMessage
			capture := func(sink *[]json.RawMessage) func(string, any) {
				return func(eventType string, data any) {
					if eventType != event.StatusComponentChanged {
						return
					}
					raw, err := json.Marshal(data)
					require.NoError(t, err)
					*sink = append(*sink, raw)
				}
			}
			svc := publicSurfaceService(t, nil, capture(&public), capture(&admin))

			comp := comp
			svc.BroadcastComponentChange(context.Background(), &comp)

			require.Len(t, public, 1, "one public status.component_changed")
			require.Len(t, admin, 1, "one admin status.component_changed")

			var pub, adm struct {
				ComponentID string          `json:"component_id"`
				Name        string          `json:"name"`
				Status      string          `json:"status"`
				Monitors    json.RawMessage `json:"monitors"`
			}
			require.NoError(t, json.Unmarshal(public[0], &pub))
			require.NoError(t, json.Unmarshal(admin[0], &adm))
			assert.Equal(t, comp.ID, pub.ComponentID)
			assert.Equal(t, comp.DisplayName, pub.Name)
			assert.NotEmpty(t, pub.Status)

			pubMonitors := monitorsOf(t, pub.Monitors)
			require.NotEmpty(t, pubMonitors)
			assertPublicShape(t, pubMonitors)
			for _, secret := range secrets {
				assert.NotContains(t, string(pub.Monitors), secret)
			}
			if name == "explicit" {
				assert.Equal(t, "API publique", pubMonitors[0]["label"])
				assert.Equal(t, StatusMajorOutage, pubMonitors[1]["status"])
			}

			admMonitors := monitorsOf(t, adm.Monitors)
			require.Len(t, admMonitors, len(pubMonitors))
			for i, m := range admMonitors {
				assert.NotEmpty(t, m["id"], "admin monitor %d keeps its id", i)
				assert.Equal(t, pubMonitors[i]["type"], m["type"])
				assert.Equal(t, pubMonitors[i]["status"], m["status"])
			}
			if name == "explicit" {
				assert.Equal(t, "API publique", admMonitors[0]["label"], "the admin surface carries the label too")
				assert.Equal(t, "4f1e-ping-token", admMonitors[1]["id"])
			} else {
				assert.Equal(t, "http://api.internal:3000/health", admMonitors[0]["name"], "the admin surface keeps the target as the name")
			}
		})
	}
}
