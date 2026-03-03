// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package status

import (
	"encoding/xml"
	"fmt"
	"net/http"
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

// HandleAtomFeed serves the Atom feed of recent incident updates.
func (h *Handler) HandleAtomFeed(w http.ResponseWriter, r *http.Request) {
	incidents, err := h.service.incidents.ListRecentIncidents(r.Context(), 30)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	baseURL := fmt.Sprintf("https://%s", r.Host)
	if r.TLS == nil {
		baseURL = fmt.Sprintf("http://%s", r.Host)
	}

	feed := AtomFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		Title:   "Status Updates",
		ID:      baseURL + "/status/feed.atom",
		Link:    AtomLink{Href: baseURL + "/status", Rel: "alternate", Type: "text/html"},
		Updated: time.Now().UTC().Format(time.RFC3339),
	}

	for _, inc := range incidents {
		summary := fmt.Sprintf("[%s] %s - %s", inc.Severity, inc.Status, inc.Title)
		if len(inc.Updates) > 0 {
			summary = inc.Updates[0].Message
		}

		feed.Entries = append(feed.Entries, AtomEntry{
			Title:   fmt.Sprintf("[%s] %s", inc.Severity, inc.Title),
			ID:      fmt.Sprintf("%s/status/incidents/%d", baseURL, inc.ID),
			Link:    AtomLink{Href: baseURL + "/status", Rel: "alternate"},
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
