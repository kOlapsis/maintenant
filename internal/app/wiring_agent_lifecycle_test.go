// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

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
			if evt.AgentID != "agent-uuid" {
				t.Errorf("agentID = %q, want agent-uuid", evt.AgentID)
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

func TestAgentAlertEvents(t *testing.T) {
	type want struct {
		alertType string
		severity  string
		recover   bool
	}
	cases := []struct {
		name      string
		reason    string
		connected bool
		want      []want
	}{
		{"runtime stops answering", "runtime_unreachable", true, []want{{"runtime_unreachable", alert.SeverityWarning, false}}},
		{"runtime answers again", "runtime_reachable", true, []want{{"runtime_unreachable", alert.SeverityInfo, true}}},
		{"connect clears the disconnection only", "", true, []want{{"disconnected", alert.SeverityInfo, true}}},
		{"stream drop leaves the runtime alert alone", "stream_ended", false, []want{{"disconnected", alert.SeverityWarning, false}}},
		{"revoked clears both", "revoked", false, []want{{"disconnected", alert.SeverityInfo, true}, {"runtime_unreachable", alert.SeverityInfo, true}}},
		{"deleted clears both", "deleted", false, []want{{"disconnected", alert.SeverityInfo, true}, {"runtime_unreachable", alert.SeverityInfo, true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := agentAlertEvents("agent-uuid", "web-01", tc.reason, tc.connected)
			if len(events) != len(tc.want) {
				t.Fatalf("got %d events, want %d", len(events), len(tc.want))
			}
			for i, evt := range events {
				got := want{evt.AlertType, evt.Severity, evt.IsRecover}
				if got != tc.want[i] {
					t.Errorf("event %d = %+v, want %+v", i, got, tc.want[i])
				}
				if evt.Source != alert.SourceAgent || evt.EntityType != "agent" || evt.EntityID != "agent-uuid" || evt.AgentID != "agent-uuid" {
					t.Errorf("event %d: unexpected envelope %+v", i, evt)
				}
				if evt.EntityName != "web-01" || evt.Message == "" {
					t.Errorf("event %d: name %q, message %q", i, evt.EntityName, evt.Message)
				}
			}
		})
	}
}
