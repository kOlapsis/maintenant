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

// TestAgentLifecycleEvent pins the severity/recovery semantics of the agent
// lifecycle alert: a genuine outage raises a Warning (severity owned by the
// alert engine), while reconnection and intentional removal emit a non-paging
// recovery.
func TestAgentLifecycleEvent(t *testing.T) {
	cases := []struct {
		name      string
		reason    string
		connected bool
		wantSev   string
		wantRec   bool
	}{
		{"stream drop is warning", "stream_ended", false, alert.SeverityWarning, false},
		{"stale liveness is warning", "stale", false, alert.SeverityWarning, false},
		{"reconnect recovers", "", true, alert.SeverityInfo, true},
		{"revoked recovers, never pages", "revoked", false, alert.SeverityInfo, true},
		{"deleted recovers, never pages", "deleted", false, alert.SeverityInfo, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evt := agentLifecycleEvent("agent-uuid", "web-01", tc.reason, tc.connected)
			if evt.Severity != tc.wantSev {
				t.Errorf("severity = %q, want %q", evt.Severity, tc.wantSev)
			}
			if evt.IsRecover != tc.wantRec {
				t.Errorf("isRecover = %v, want %v", evt.IsRecover, tc.wantRec)
			}
			if evt.Source != "agent" || evt.AlertType != "disconnected" || evt.EntityType != "agent" {
				t.Errorf("unexpected envelope: source=%q type=%q entity=%q", evt.Source, evt.AlertType, evt.EntityType)
			}
			if evt.EntityID != "agent-uuid" {
				t.Errorf("entityID = %q, want agent-uuid", evt.EntityID)
			}
			if evt.Message == "" {
				t.Error("message must not be empty")
			}
			// A genuine outage carries diagnostic details; recoveries do not.
			if !tc.connected && tc.reason != "revoked" && tc.reason != "deleted" {
				if evt.Details["reason"] != tc.reason {
					t.Errorf("details.reason = %v, want %q", evt.Details["reason"], tc.reason)
				}
			}
		})
	}
}
