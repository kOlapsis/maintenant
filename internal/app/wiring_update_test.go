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

package app

import (
	"testing"

	"github.com/kolapsis/maintenant/internal/alert"
)

func detectedPayload(name, id string, risk int) map[string]any {
	return map[string]any{
		"container_id":   id,
		"container_name": name,
		"image":          name + ":1",
		"current_tag":    "1",
		"latest_tag":     "2",
		"update_type":    "minor",
		"risk_score":     risk,
		"update_command": "docker compose up -d " + name,
	}
}

// TestUpdateDetectedAlert_EntityIDPerContainer is the regression guard for the
// "keycloak message on bitwarden entity" bug: each container's update must carry
// its OWN entity id, so the alert engine never collides two containers onto one
// shared (empty) dedup key.
func TestUpdateDetectedAlert_EntityIDPerContainer(t *testing.T) {
	keycloak := updateDetectedAlert(detectedPayload("keycloak", "id-keycloak", 90), false)
	bitwarden := updateDetectedAlert(detectedPayload("bitwarden-postgres", "id-bw", 90), false)

	if keycloak.EntityID != "id-keycloak" {
		t.Errorf("keycloak EntityID = %q, want id-keycloak", keycloak.EntityID)
	}
	if bitwarden.EntityID != "id-bw" {
		t.Errorf("bitwarden EntityID = %q, want id-bw", bitwarden.EntityID)
	}
	if keycloak.EntityID == bitwarden.EntityID {
		t.Fatal("two containers must not share an entity id (would collide in the alert engine)")
	}
	// Message, entity name and entity id must all describe the SAME container.
	if keycloak.EntityName != "keycloak" || keycloak.Message != "Update available for keycloak: 2" {
		t.Errorf("keycloak alert fields mismatch: name=%q msg=%q", keycloak.EntityName, keycloak.Message)
	}
	if keycloak.EntityType != "container" {
		t.Errorf("EntityType = %q, want container", keycloak.EntityType)
	}
}

func TestUpdateDetectedAlert_SeverityFromRiskScore(t *testing.T) {
	cases := []struct {
		risk int
		want string
	}{
		{90, alert.SeverityCritical},
		{70, alert.SeverityWarning},
		{10, alert.SeverityInfo},
	}
	for _, tc := range cases {
		got := updateDetectedAlert(detectedPayload("svc", "id", tc.risk), false).Severity
		if got != tc.want {
			t.Errorf("risk %d → severity %q, want %q", tc.risk, got, tc.want)
		}
	}
}

func TestUpdateDetectedAlert_ProDetailsGated(t *testing.T) {
	ce := updateDetectedAlert(detectedPayload("svc", "id", 90), false)
	if _, ok := ce.Details["update_command"]; ok {
		t.Error("update_command must not leak into CE alert details")
	}
	pro := updateDetectedAlert(detectedPayload("svc", "id", 90), true)
	if _, ok := pro.Details["update_command"]; !ok {
		t.Error("update_command must be present in Pro alert details")
	}
	if pro.Details["latest_tag"] != "2" {
		t.Errorf("latest_tag detail = %v, want 2", pro.Details["latest_tag"])
	}
}

func TestUpdateResolvedAlert(t *testing.T) {
	evt := updateResolvedAlert(map[string]any{"container_id": "id-bw", "container_name": "bitwarden-postgres"})
	if !evt.IsRecover {
		t.Error("resolved alert must be a recovery event")
	}
	if evt.Severity != alert.SeverityInfo {
		t.Errorf("severity = %q, want info", evt.Severity)
	}
	if evt.EntityID != "id-bw" || evt.EntityType != "container" {
		t.Errorf("entity mismatch: id=%q type=%q", evt.EntityID, evt.EntityType)
	}
	if evt.Message != "Update no longer required for bitwarden-postgres" {
		t.Errorf("message = %q", evt.Message)
	}
}
