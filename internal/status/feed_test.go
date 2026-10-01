// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"encoding/xml"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type feedIncidentStore struct {
	IncidentStore
	active, recent []Incident
}

func (s feedIncidentStore) ListActiveIncidents(context.Context) ([]Incident, error) {
	return s.active, nil
}

func (s feedIncidentStore) ListRecentIncidents(context.Context, int) ([]Incident, error) {
	return s.recent, nil
}

func TestAtomFeedCarriesActiveAndRecentIncidents(t *testing.T) {
	now := time.Now().UTC()
	resolvedAt := now.Add(-2 * time.Hour)
	incidents := feedIncidentStore{
		active: []Incident{
			{ID: "ongoing", Title: "Database down", Severity: SeverityMajor, Status: IncidentInvestigating, UpdatedAt: now.Add(-time.Minute)},
		},
		recent: []Incident{
			{ID: "fixed", Title: "DNS hiccup", Severity: SeverityMinor, Status: IncidentResolved, ResolvedAt: &resolvedAt, UpdatedAt: resolvedAt},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(Deps{Components: emptyComponentStore{}, Logger: logger, Incidents: incidents})
	h := NewHandler(svc, nil, logger, nil, "https://status.example.com")

	rec := httptest.NewRecorder()
	h.HandleAtomFeed(rec, httptest.NewRequest(http.MethodGet, "http://10.0.0.5:8080/status/feed.atom", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}

	var feed AtomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if feed.ID != "https://status.example.com/feed.atom" || feed.Link.Href != "https://status.example.com" {
		t.Fatalf("feed id %q and link %q, want them under the configured page, not the request host", feed.ID, feed.Link.Href)
	}
	var ids []string
	for _, e := range feed.Entries {
		ids = append(ids, e.ID)
		if e.Link.Href != "https://status.example.com" {
			t.Fatalf("entry link %q, want the configured page", e.Link.Href)
		}
	}
	want := []string{
		"https://status.example.com/incidents/ongoing",
		"https://status.example.com/incidents/fixed",
	}
	if len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("entries %v, want the ongoing incident first then the resolved one: %v", ids, want)
	}
}

func TestPageURLPrefersTheStatusURL(t *testing.T) {
	cases := []struct{ statusURL, baseURL, want string }{
		{"https://status.example.com/", "https://maintenant.example.com", "https://status.example.com"},
		{"", "https://maintenant.example.com/", "https://maintenant.example.com/status"},
		{"", "https://example.com/monitoring", "https://example.com/monitoring/status"},
	}
	for _, tc := range cases {
		if got := PageURL(tc.statusURL, tc.baseURL); got != tc.want {
			t.Errorf("PageURL(%q, %q) = %q, want %q", tc.statusURL, tc.baseURL, got, tc.want)
		}
	}
}
