// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Atom feed types for encoding/xml.

type AtomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	XMLNS   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Link    AtomLink    `xml:"link"`
	Updated string      `xml:"updated"`
	Entries []AtomEntry `xml:"entry"`
}

type AtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type AtomEntry struct {
	Title   string   `xml:"title"`
	ID      string   `xml:"id"`
	Link    AtomLink `xml:"link"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary"`
}

// PageURL is the public address of the status page: statusURL when set, otherwise baseURL followed by /status.
func PageURL(statusURL, baseURL string) string {
	if statusURL != "" {
		return strings.TrimRight(statusURL, "/")
	}
	return strings.TrimRight(baseURL, "/") + "/status"
}

// HandleAtomFeed serves the Atom feed of the ongoing incidents and those resolved in the last 30 days, latest change first.
func (h *Handler) HandleAtomFeed(w http.ResponseWriter, r *http.Request) {
	active, err := h.service.incidents.ListActiveIncidents(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	recent, err := h.service.incidents.ListRecentIncidents(r.Context(), 30)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	incidents := append(active, recent...)
	sort.SliceStable(incidents, func(i, j int) bool {
		return incidents[i].UpdatedAt.After(incidents[j].UpdatedAt)
	})

	feed := AtomFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		Title:   "Status Updates",
		ID:      h.pageURL + "/feed.atom",
		Link:    AtomLink{Href: h.pageURL, Rel: "alternate", Type: "text/html"},
		Updated: time.Now().UTC().Format(time.RFC3339),
	}

	for _, inc := range incidents {
		summary := fmt.Sprintf("[%s] %s - %s", inc.Severity, inc.Status, inc.Title)
		if len(inc.Updates) > 0 {
			summary = inc.Updates[0].Message
		}

		feed.Entries = append(feed.Entries, AtomEntry{
			Title:   fmt.Sprintf("[%s] %s", inc.Severity, inc.Title),
			ID:      h.pageURL + "/incidents/" + inc.ID,
			Link:    AtomLink{Href: h.pageURL, Rel: "alternate"},
			Updated: inc.UpdatedAt.UTC().Format(time.RFC3339),
			Summary: summary,
		})
	}

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)

	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_, _ = w.Write([]byte(xml.Header))
	_ = enc.Encode(feed)
}
